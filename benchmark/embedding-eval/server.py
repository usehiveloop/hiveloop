#!/usr/bin/env python3
"""
Query server for the embedding database.

Endpoints:
  POST /query    — run raw SQL against the symbols table
  POST /similar  — find symbols similar to a code snippet or query
  GET  /stats    — database statistics
  GET  /health   — health check
"""

import os
import sqlite3
import struct
import time

import sqlite_vec
from flask import Flask, request, jsonify
from openai import OpenAI

app = Flask(__name__)

DB_PATH = os.environ.get("DB_PATH", "/workspace/vectors.db")
EMBEDDING_MODEL = "text-embedding-3-large"

_client = None


def get_db():
    database = sqlite3.connect(DB_PATH)
    database.row_factory = sqlite3.Row
    database.enable_load_extension(True)
    sqlite_vec.load(database)
    database.enable_load_extension(False)
    return database


def get_client():
    global _client
    if _client is None:
        api_key = os.environ.get("OPENAI_API_KEY") or os.environ.get("OPENAPI_KEY")
        _client = OpenAI(api_key=api_key)
    return _client


def serialize_f32(vector):
    return struct.pack(f"{len(vector)}f", *vector)


@app.route("/health")
def health():
    return jsonify({"status": "ok"})


@app.route("/stats")
def stats():
    database = get_db()
    total = database.execute("SELECT COUNT(*) FROM symbols").fetchone()[0]
    by_lang = database.execute(
        "SELECT language, COUNT(*) as count FROM symbols GROUP BY language ORDER BY count DESC"
    ).fetchall()
    by_type = database.execute(
        "SELECT node_type, COUNT(*) as count FROM symbols GROUP BY node_type ORDER BY count DESC"
    ).fetchall()
    meta = database.execute("SELECT key, value FROM index_meta").fetchall()
    db_size = os.path.getsize(DB_PATH) / (1024 * 1024)

    return jsonify({
        "total_symbols": total,
        "db_size_mb": round(db_size, 1),
        "by_language": [{"language": row["language"], "count": row["count"]} for row in by_lang],
        "by_type": [{"node_type": row["node_type"], "count": row["count"]} for row in by_type],
        "meta": {row["key"]: row["value"] for row in meta},
    })


@app.route("/query", methods=["POST"])
def query():
    data = request.get_json()
    if not data or "sql" not in data:
        return jsonify({"error": "missing 'sql' field"}), 400

    database = get_db()
    start = time.time()
    try:
        cursor = database.execute(data["sql"], data.get("params", []))
        columns = [desc[0] for desc in cursor.description] if cursor.description else []
        rows = [dict(zip(columns, row)) for row in cursor.fetchall()]
        return jsonify({
            "columns": columns, "rows": rows,
            "count": len(rows), "elapsed_ms": round((time.time() - start) * 1000, 2),
        })
    except Exception as exc:
        return jsonify({"error": str(exc)}), 400


@app.route("/similar", methods=["POST"])
def similar():
    data = request.get_json()
    if not data or "query" not in data:
        return jsonify({"error": "missing 'query' field"}), 400

    query_text = data["query"]
    limit = data.get("limit", 10)
    language_filter = data.get("language")

    client = get_client()
    database = get_db()

    start = time.time()
    response = client.embeddings.create(model=EMBEDDING_MODEL, input=[query_text])
    query_embedding = response.data[0].embedding
    embed_time = time.time() - start

    search_start = time.time()
    vec_query = serialize_f32(query_embedding)
    fetch_limit = limit * 5 if language_filter else limit

    rows = database.execute("""
        SELECT vec_symbols.rowid, vec_symbols.distance
        FROM vec_symbols
        WHERE embedding MATCH ?
        ORDER BY distance
        LIMIT ?
    """, (vec_query, fetch_limit)).fetchall()

    results = []
    for row in rows:
        symbol = database.execute(
            "SELECT id, name, file_path, start_line, end_line, node_type, language, body FROM symbols WHERE id = ?",
            (row["rowid"],)
        ).fetchone()
        if symbol is None:
            continue
        if language_filter and symbol["language"] != language_filter:
            continue
        results.append({
            "name": symbol["name"],
            "file_path": symbol["file_path"],
            "start_line": symbol["start_line"],
            "end_line": symbol["end_line"],
            "node_type": symbol["node_type"],
            "language": symbol["language"],
            "distance": round(row["distance"], 4),
            "similarity": round(1 - row["distance"], 4),
            "body_preview": symbol["body"][:200] + "..." if len(symbol["body"]) > 200 else symbol["body"],
        })
        if len(results) >= limit:
            break

    search_time = time.time() - search_start

    return jsonify({
        "query": query_text,
        "results": results,
        "count": len(results),
        "embed_time_ms": round(embed_time * 1000, 2),
        "search_time_ms": round(search_time * 1000, 2),
        "total_time_ms": round((embed_time + search_time) * 1000, 2),
    })


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8080))
    get_db()
    print(f"Server ready on port {port}")
    app.run(host="0.0.0.0", port=port)
