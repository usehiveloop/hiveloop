#!/usr/bin/env python3
"""
Index a codebase into sqlite-vec using OpenAI text-embedding-3-large.

Extracts functions/methods/classes from Go, TypeScript, and Python files
using tree-sitter, embeds them via OpenAI API in batch, and stores
everything in a SQLite database with vector similarity search.
"""

import os
import sys
import time
import sqlite3
import struct
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor, as_completed

import sqlite_vec
import tree_sitter_go as tsgo
import tree_sitter_typescript as tsts
import tree_sitter_python as tspy
from tree_sitter import Language, Parser
from openai import OpenAI

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

EMBEDDING_MODEL = "text-embedding-3-large"
EMBEDDING_DIMS = 3072
BATCH_SIZE = 500  # Keep under OpenAI's 300K token-per-request limit

# ---------------------------------------------------------------------------
# Tree-sitter setup
# ---------------------------------------------------------------------------

GO_LANG = Language(tsgo.language())
TS_LANG = Language(tsts.language_typescript())
TSX_LANG = Language(tsts.language_tsx())
PY_LANG = Language(tspy.language())

LANG_MAP = {
    ".go": GO_LANG,
    ".ts": TS_LANG,
    ".tsx": TSX_LANG,
    ".py": PY_LANG,
}

EXTRACT_TYPES = {
    "function_declaration", "method_declaration",
    "type_declaration",
    "function_definition", "method_definition",
    "class_declaration", "interface_declaration",
    "type_alias_declaration", "class_definition",
}

SKIP_DIRS = {
    ".git", "node_modules", "vendor", "dist", "build", ".next",
    "__pycache__", ".ignored", "zig-cache", "zig-out", ".turbo",
    ".vercel", "coverage", ".nyc_output", "target",
}


def get_symbol_name(node, source_bytes):
    name_node = node.child_by_field_name("name")
    if name_node:
        return source_bytes[name_node.start_byte:name_node.end_byte].decode("utf-8", errors="replace")
    if node.type == "lexical_declaration":
        for child in node.children:
            if child.type == "variable_declarator":
                name_child = child.child_by_field_name("name")
                if name_child:
                    return source_bytes[name_child.start_byte:name_child.end_byte].decode("utf-8", errors="replace")
    return None


def extract_symbols(file_path, lang):
    try:
        source = Path(file_path).read_bytes()
    except (OSError, UnicodeDecodeError):
        return []

    parser = Parser(lang)
    tree = parser.parse(source)
    symbols = []

    def walk(node):
        if node.type in EXTRACT_TYPES:
            name = get_symbol_name(node, source)
            if not name or len(name) < 2:
                return
            body = source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")
            if len(body.strip()) < 10:
                return
            # Truncate for embedding (OpenAI handles 8191 tokens, but keep reasonable)
            embed_body = body[:3000] if len(body) > 3000 else body
            symbols.append({
                "name": name,
                "file_path": str(file_path),
                "start_line": node.start_point[0] + 1,
                "end_line": node.end_point[0] + 1,
                "node_type": node.type,
                "body": body,
                "embed_text": embed_body,
                "language": {".go": "go", ".ts": "typescript", ".tsx": "tsx", ".py": "python"}.get(
                    Path(file_path).suffix, "unknown"
                ),
            })
            return
        for child in node.children:
            walk(child)

    walk(tree.root_node)
    return symbols


def collect_files(repo_path):
    files = []
    for root, dirs, filenames in os.walk(repo_path):
        dirs[:] = [directory for directory in dirs if directory not in SKIP_DIRS]
        for filename in filenames:
            ext = Path(filename).suffix
            if ext in LANG_MAP:
                files.append((os.path.join(root, filename), ext))
    return files


def serialize_f32(vector):
    return struct.pack(f"{len(vector)}f", *vector)


def embed_batch(client, texts):
    """Embed a batch of texts via OpenAI API. Returns list of embedding vectors."""
    response = client.embeddings.create(
        model=EMBEDDING_MODEL,
        input=texts,
    )
    # Sort by index to maintain order
    sorted_data = sorted(response.data, key=lambda item: item.index)
    return [item.embedding for item in sorted_data]


def main():
    repo_path = sys.argv[1] if len(sys.argv) > 1 else "/workspace/repo"
    db_path = sys.argv[2] if len(sys.argv) > 2 else "/workspace/vectors.db"

    api_key = os.environ.get("OPENAI_API_KEY") or os.environ.get("OPENAPI_KEY")
    if not api_key:
        print("ERROR: Set OPENAI_API_KEY or OPENAPI_KEY environment variable")
        sys.exit(1)

    client = OpenAI(api_key=api_key)
    total_start = time.time()

    print(f"[1/4] Scanning {repo_path} for source files...")
    scan_start = time.time()
    files = collect_files(repo_path)
    scan_time = time.time() - scan_start
    print(f"       Found {len(files)} source files in {scan_time:.2f}s")

    print(f"[2/4] Extracting symbols with tree-sitter...")
    extract_start = time.time()
    all_symbols = []
    file_count_by_lang = {}
    for file_path, ext in files:
        lang = LANG_MAP[ext]
        symbols = extract_symbols(file_path, lang)
        all_symbols.extend(symbols)
        lang_name = {".go": "go", ".ts": "ts", ".tsx": "tsx", ".py": "py"}[ext]
        file_count_by_lang[lang_name] = file_count_by_lang.get(lang_name, 0) + 1

    for symbol in all_symbols:
        symbol["file_path"] = os.path.relpath(symbol["file_path"], repo_path)
    extract_time = time.time() - extract_start
    print(f"       Extracted {len(all_symbols)} symbols in {extract_time:.2f}s")
    print(f"       Files by language: {file_count_by_lang}")

    print(f"[3/4] Generating embeddings via OpenAI API ({EMBEDDING_MODEL})...")
    embed_start = time.time()
    texts = [symbol["embed_text"] for symbol in all_symbols]

    # Build batches
    batches = []
    for batch_idx in range(0, len(texts), BATCH_SIZE):
        batches.append((batch_idx, texts[batch_idx:batch_idx + BATCH_SIZE]))
    print(f"       {len(batches)} batches, sending in parallel...")

    # Send all batches in parallel, track each round-trip
    all_embeddings = [None] * len(texts)
    batch_timings = []

    def embed_one_batch(args):
        batch_idx, start_idx, batch_texts = args
        batch_start = time.time()
        response = client.embeddings.create(model=EMBEDDING_MODEL, input=batch_texts)
        batch_elapsed = time.time() - batch_start
        sorted_data = sorted(response.data, key=lambda item: item.index)
        tokens_used = response.usage.total_tokens if response.usage else 0
        return batch_idx, start_idx, [item.embedding for item in sorted_data], batch_elapsed, tokens_used, len(batch_texts)

    with ThreadPoolExecutor(max_workers=len(batches)) as executor:
        futures = []
        for batch_idx, (start_idx, batch_texts) in enumerate(batches):
            futures.append(executor.submit(embed_one_batch, (batch_idx, start_idx, batch_texts)))
        for future in as_completed(futures):
            batch_idx, start_idx, embeddings, elapsed, tokens, count = future.result()
            batch_timings.append({"batch": batch_idx + 1, "symbols": count, "tokens": tokens, "api_ms": round(elapsed * 1000, 1)})
            for local_idx, embedding in enumerate(embeddings):
                all_embeddings[start_idx + local_idx] = embedding

    embed_time = time.time() - embed_start
    valid_count = sum(1 for embedding in all_embeddings if embedding is not None)
    total_tokens = sum(bt["tokens"] for bt in batch_timings)

    print(f"       Generated {valid_count} embeddings in {embed_time:.1f}s")
    if valid_count:
        print(f"       Speed: {valid_count/embed_time:.0f} embeddings/sec")
    print(f"       Total tokens: {total_tokens:,}")
    print(f"       Estimated cost: ${total_tokens * 0.00000013:.4f}")
    print(f"       Per-batch timings:")
    for bt in sorted(batch_timings, key=lambda x: x["batch"]):
        print(f"         Batch {bt['batch']}: {bt['symbols']} symbols, {bt['tokens']:,} tokens, {bt['api_ms']}ms round-trip")

    print(f"[4/4] Storing in SQLite + sqlite-vec...")
    store_start = time.time()

    db_open_start = time.time()
    db = sqlite3.connect(db_path)
    db.enable_load_extension(True)
    sqlite_vec.load(db)
    db.enable_load_extension(False)
    db_open_ms = (time.time() - db_open_start) * 1000

    schema_start = time.time()
    db.execute("DROP TABLE IF EXISTS symbols")
    db.execute("DROP TABLE IF EXISTS vec_symbols")
    db.execute("DROP TABLE IF EXISTS index_meta")
    db.execute("""
        CREATE TABLE symbols (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            file_path TEXT NOT NULL,
            start_line INTEGER NOT NULL,
            end_line INTEGER NOT NULL,
            node_type TEXT NOT NULL,
            language TEXT NOT NULL,
            body TEXT NOT NULL
        )
    """)
    db.execute("CREATE INDEX idx_symbols_name ON symbols(name)")
    db.execute("CREATE INDEX idx_symbols_file ON symbols(file_path)")
    db.execute("CREATE INDEX idx_symbols_lang ON symbols(language)")
    db.execute(f"CREATE VIRTUAL TABLE vec_symbols USING vec0(embedding float[{EMBEDDING_DIMS}])")
    db.execute("""
        CREATE TABLE index_meta (
            key TEXT PRIMARY KEY,
            value TEXT
        )
    """)
    schema_ms = (time.time() - schema_start) * 1000

    insert_start = time.time()
    for symbol, embedding in zip(all_symbols, all_embeddings):
        cursor = db.execute(
            "INSERT INTO symbols (name, file_path, start_line, end_line, node_type, language, body) VALUES (?, ?, ?, ?, ?, ?, ?)",
            (symbol["name"], symbol["file_path"], symbol["start_line"], symbol["end_line"],
             symbol["node_type"], symbol["language"], symbol["body"])
        )
        row_id = cursor.lastrowid
        db.execute(
            "INSERT INTO vec_symbols (rowid, embedding) VALUES (?, ?)",
            (row_id, serialize_f32(embedding))
        )
    insert_ms = (time.time() - insert_start) * 1000

    import subprocess
    try:
        git_head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repo_path, text=True).strip()
    except Exception:
        git_head = "unknown"

    db.execute("INSERT INTO index_meta VALUES ('last_commit', ?)", (git_head,))
    db.execute("INSERT INTO index_meta VALUES ('model', ?)", (EMBEDDING_MODEL,))
    db.execute("INSERT INTO index_meta VALUES ('dimensions', ?)", (str(EMBEDDING_DIMS),))
    db.execute("INSERT INTO index_meta VALUES ('symbol_count', ?)", (str(len(all_symbols)),))
    db.execute("INSERT INTO index_meta VALUES ('total_tokens', ?)", (str(total_tokens),))
    db.execute("INSERT INTO index_meta VALUES ('indexed_at', ?)", (time.strftime("%Y-%m-%dT%H:%M:%SZ"),))

    commit_start = time.time()
    db.commit()
    commit_ms = (time.time() - commit_start) * 1000

    store_time = time.time() - store_start
    print(f"       DB open + vec load:  {db_open_ms:.1f}ms")
    print(f"       Schema creation:     {schema_ms:.1f}ms")
    print(f"       Insert {len(all_symbols)} rows:    {insert_ms:.1f}ms ({insert_ms/len(all_symbols):.2f}ms/row)")
    print(f"       Commit to disk:      {commit_ms:.1f}ms")
    total_time = time.time() - total_start
    db_size = os.path.getsize(db_path) / (1024 * 1024)

    print(f"\n{'='*60}")
    print(f"INDEXING COMPLETE")
    print(f"{'='*60}")
    print(f"  Repository:     {repo_path}")
    print(f"  Database:       {db_path} ({db_size:.1f} MB)")
    print(f"  Source files:   {len(files)}")
    print(f"  Symbols:        {len(all_symbols)}")
    print(f"  Embeddings:     {len(all_embeddings)} ({EMBEDDING_MODEL})")
    print(f"  Git HEAD:       {git_head}")
    print(f"  File scan:      {scan_time:.2f}s")
    print(f"  Extraction:     {extract_time:.2f}s")
    print(f"  Embedding:      {embed_time:.1f}s")
    print(f"  Storage:        {store_time:.2f}s")
    print(f"  TOTAL:          {total_time:.1f}s")
    print(f"{'='*60}")


if __name__ == "__main__":
    main()
