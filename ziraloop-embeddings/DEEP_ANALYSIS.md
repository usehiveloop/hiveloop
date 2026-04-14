# ziraloop-embeddings: Deep Competitive Analysis & Improvement Roadmap

> **Date:** 2026-04-14
> **Scope:** RAG indexing pipeline comparison against Greptile, CodeRabbit, Sourcegraph Cody, Cursor, Qodo, Bloop, and GitHub Copilot
> **Purpose:** Identify gaps in our code understanding pipeline and prioritize improvements that directly improve code review quality

---

## Table of Contents

1. [Current Architecture Summary](#1-current-architecture-summary)
2. [Industry Landscape](#2-industry-landscape)
3. [Gap Analysis](#3-gap-analysis)
   - [Gap 1: Raw Code Embedding vs. Semantic Description Embedding](#gap-1-raw-code-embedding-vs-semantic-description-embedding)
   - [Gap 2: No Code Graph or Relationship Tracking](#gap-2-no-code-graph-or-relationship-tracking)
   - [Gap 3: Single Retrieval Modality (No Keyword Search)](#gap-3-single-retrieval-modality-no-keyword-search)
   - [Gap 4: No Re-Ranking Stage](#gap-4-no-re-ranking-stage)
   - [Gap 5: Isolated Symbol Results (No Surrounding Context)](#gap-5-isolated-symbol-results-no-surrounding-context)
   - [Gap 6: File-Level Incremental Indexing (No Symbol-Level Caching)](#gap-6-file-level-incremental-indexing-no-symbol-level-caching)
   - [Gap 7: No Non-Symbol Content Indexing](#gap-7-no-non-symbol-content-indexing)
   - [Gap 8: No Feedback or Learning Loop](#gap-8-no-feedback-or-learning-loop)
4. [Priority Matrix](#4-priority-matrix)
5. [What We Do Well](#5-what-we-do-well)
6. [References](#6-references)

---

## 1. Current Architecture Summary

### Pipeline

```
Repository Scan → Tree-sitter AST Parse → Symbol Extraction (functions/classes)
  → Raw Body Truncation (3000 chars) → OpenAI Embedding → SQLite+sqlite-vec Storage
  → Cosine Distance Search → Return Results
```

### Key Implementation Details

| Component | Implementation | Source File |
|-----------|---------------|-------------|
| **AST Parsing** | Tree-sitter via `go-tree-sitter`, 13 languages | `internal/extractor/walker.go` |
| **Chunking** | Top-level functions + classes only. Methods inside classes via recursion. Min 10 chars body, min 2 chars name | `internal/extractor/walker.go:36-71` |
| **Embed Text** | Raw code body, truncated to 3000 chars | `internal/extractor/walker.go:49-51` |
| **Embedding Model** | `text-embedding-3-large` (3072 dims) or `text-embedding-3-small` (1536 dims) via OpenAI-compatible proxy | `internal/embedder/embedder.go`, `internal/config/config.go:36-43` |
| **Batch Processing** | Max 500 texts per batch, parallel goroutines | `internal/embedder/embedder.go:14`, `embedder.go:98-128` |
| **Storage** | SQLite + sqlite-vec (`vec0` virtual table), WAL mode | `internal/store/store.go:53-69`, `internal/store/schema.go` |
| **Vector Search** | `embedding MATCH ?` with L2 distance, over-fetch 5x for post-filtering | `internal/store/store.go:232-238` |
| **Filtering** | Post-query filtering by repo and language | `internal/store/store.go:259-268` |
| **Incremental Indexing** | `git diff --name-only` between stored commit and HEAD, re-index changed files entirely | `internal/drift/drift.go:38-105` |
| **Result Format** | Symbol name, file path, line range, similarity score, 200-char body preview | `internal/store/store.go:270-285` |
| **Persistence** | Local SQLite file with optional drive sync (pull-if-exists, index, push) | `main.go:109-146`, `internal/drive/client.go` |

### Schema

```sql
-- Symbols table (flat, no relationships)
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_name TEXT NOT NULL,
    name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    node_type TEXT NOT NULL,
    language TEXT NOT NULL,
    body TEXT NOT NULL
);

-- Vector index
CREATE VIRTUAL TABLE vec_symbols USING vec0(embedding float[3072]);

-- Repo metadata
CREATE TABLE repo_meta (
    repo_name TEXT PRIMARY KEY,
    repo_path TEXT NOT NULL,
    last_commit TEXT NOT NULL,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    symbol_count INTEGER NOT NULL,
    total_tokens INTEGER NOT NULL,
    indexed_at TEXT NOT NULL
);
```

### What Is NOT Stored

- No natural-language descriptions or summaries
- No import/dependency edges
- No call graph relationships
- No full-text search index
- No content hashes for symbol-level deduplication
- No configuration files, migrations, or non-code content
- No review feedback or learning data

---

## 2. Industry Landscape

### Greptile

**Company:** YC W24, $25M Series A. AI code review for enterprise.

**Indexing Architecture:**

Greptile's core thesis is that codebases are graphs, not lists of files. Their indexing has four stages:

1. **Tree-sitter AST parsing** across all supported languages
2. **Recursive docstring generation**: An LLM generates natural-language summaries for every function, starting from leaf functions (no callees) and working upward. Each function's summary incorporates the summaries of its callees, so a high-level function's description captures the entire call chain beneath it. This is conceptually similar to HyDE (Hypothetical Document Embeddings) but applied at index time rather than query time.
3. **Docstring embedding**: The NL summaries are embedded instead of raw code. Their testing shows NL queries score **0.815 similarity** against NL descriptions vs **0.728** against raw code — a 12% improvement.
4. **Code graph construction**: Nodes represent files, functions, classes, variables, and external calls. Edges represent function calls, imports, variable usage, and library references.

**Retrieval:** Three modalities combined — (1) vector similarity on docstring embeddings, (2) keyword/full-text search for exact identifiers, (3) agentic graph traversal where an LLM agent follows call chains and import paths to discover indirect relationships.

**Storage:** PostgreSQL with pgvector (single database for relational + vector data).

**Review Architecture (v3):** Built on the Anthropic Claude Agent SDK. The agent runs in a loop with tools for codebase search, git history, semantic similarity, learned rules lookup, and PR context examination. Sub-agents handle memory retrieval (coding standards, `.cursorrules`, `CLAUDE.md`, learned codebase patterns).

**Feedback System:** Embeds upvoted/downvoted review comments. If a new comment has high cosine similarity with 3+ unique downvoted comments, it gets blocked. Improved their "address rate" (% of comments developers actually fix) from **19% to over 55%** in two weeks.

**Sources:**
- [Greptile: Codebases are uniquely hard to search semantically](https://www.greptile.com/blog/semantic-codebase-search)
- [Greptile v3: Agentic Code Review](https://www.greptile.com/blog/greptile-v3-agentic-code-review)
- [Greptile: Graph-based Codebase Context](https://www.greptile.com/docs/how-greptile-works/graph-based-codebase-context)
- [Hatchet x Greptile Case Study](https://hatchet.run/customers/greptile)
- [ZenML: Improving AI Code Review Comment Quality via Vector Embeddings](https://www.zenml.io/llmops-database/improving-ai-code-review-bot-comment-quality-through-vector-embeddings)
- [Claude Customer Story: Greptile](https://claude.com/customers/greptile)
- [HN Launch Thread (founder comments)](https://news.ycombinator.com/item?id=39604961)

---

### CodeRabbit

**Company:** AI-native code review, 10K+ daily PRs at peak.

**Indexing Architecture:**

CodeRabbit does NOT build a persistent pre-indexed embedding store the way most tools do. Instead, it constructs context on-the-fly per review using a 5-layer context engineering pipeline:

1. **PR and repository metadata**: PR title, description, commit range, incremental changes since last review
2. **Knowledge integration**: Past review learnings from LanceDB, issue tracker intent (Jira, Linear, GitHub Issues), PR history, coding guidelines (`.cursorrules`, `CLAUDE.md`), custom path-based instructions
3. **Code graph + semantic retrieval**: At review time, builds a lightweight dependency graph by parsing changed files and their references using ast-grep (tree-sitter based). Adds co-change analysis from commit history (files that frequently change together). Performs semantic similarity retrieval from LanceDB.
4. **Static analysis signals**: 45+ linters and SAST tools (ESLint, Ruff, Clippy, RuboCop, Semgrep, ShellCheck, Trivy, TruffleHog, Checkov, Brakeman, etc.)
5. **Verification and grounding**: LLM-generated shell/Python scripts that confirm assumptions against actual code before comments are posted

**Design Principle:** 1:1 code-to-context ratio — for every line of code under review, an equal weight of surrounding context is included.

**Vector Storage:** LanceDB — stores embeddings of functions, classes, tests, prior changes, issue tracker content, and past review learnings. Manages tens of thousands of tables. Sub-second P99 at 50K+ daily PRs.

**AST Analysis:** Uses [ast-grep](https://github.com/ast-grep/ast-grep), a Rust-based structural code search tool built on tree-sitter. Serves three roles: (1) detecting structural code patterns/smells, (2) extracting deterministic context to ground LLM analysis, (3) discovering architectural patterns like database queries and error handling.

**Prompt Architecture:** Model-agnostic core prompt + model-specific subunits. Claude models get strong directive language; GPT models get top-to-bottom instruction alignment matching attention decay patterns.

**Agentic Validation:** For each review comment, the AI generates shell scripts (`cat`, `grep`, `ast-grep`) and Python code to verify its claims against actual code. Runs in a three-layer sandbox: Cloud Run microVM + Jailkit + cgroups.

**Learnings System:** Stores developer feedback as vector-embedded NL in LanceDB. Scoped per-repo or per-org. When a developer interacts with `@coderabbitai`, the system evaluates whether feedback is systemic and stores it as a learning. Usage count tracked.

**Sources:**
- [Google Cloud: How CodeRabbit Built Its AI Code Review Agent](https://cloud.google.com/blog/products/ai-machine-learning/how-coderabbit-built-its-ai-code-review-agent-with-google-cloud-run)
- [CodeRabbit: Accurate AI Code Reviews on Massive Codebases](https://www.coderabbit.ai/blog/how-coderabbit-delivers-accurate-ai-code-reviews-on-massive-codebases)
- [CodeRabbit: The Art and Science of Context Engineering](https://www.coderabbit.ai/blog/the-art-and-science-of-context-engineering)
- [LanceDB Case Study: CodeRabbit](https://lancedb.com/blog/case-study-coderabbit/)
- [CodeRabbit: Agentic Code Validation](https://www.coderabbit.ai/blog/how-coderabbits-agentic-code-validation-helps-with-code-reviews)
- [CodeRabbit: AI-Native Universal Linter (ast-grep + LLM)](https://www.coderabbit.ai/blog/ai-native-universal-linter-ast-grep-llm)
- [CodeRabbit: End of One-Size-Fits-All Prompts](https://www.coderabbit.ai/blog/the-end-of-one-sized-fits-all-prompts-why-llm-models-are-no-longer-interchangeable)

---

### Sourcegraph Cody (now Amp)

**Company:** Code intelligence platform, renamed Cody to "Amp" mid-2025, spun off as Amp Inc.

**Indexing Architecture:**

Sourcegraph's differentiator is SCIP (SCIP Code Intelligence Protocol), a Protobuf-based semantic indexing format that replaced LSIF. Language-specific indexers (scip-typescript, scip-java, scip-clang, scip-python, scip-ruby, etc. covering 14+ languages) produce structured data about definitions, references, symbols, and doc comments. This builds a true semantic code graph — not just text similarity, but actual symbol relationships.

**Code Graph:**
- **Repo-level Semantic Graph (RSG)**: Encapsulates global context elements and their dependencies
- **SCIP indexers**: Produce cross-file and cross-repo symbol definitions, references, and call chains
- **Fallback**: Tree-sitter and ctags for languages without SCIP indexers

**Retrieval:**
- Hybrid dense-sparse vector retrieval: sparse vectors for keyword search (with LLM-based term expansion), dense vectors for semantic similarity
- Graph-based "Expand and Refine" ranking: uses link prediction on the RSG to rerank results
- Agentic context fetching: a mini-agent proactively uses tools (code search, file access, terminal, web browser, OpenCtx) to gather context

**Scale:** 300,000+ repositories, 90GB+ monorepos. Self-hostable and air-gapped deployments.

**Sources:**
- [Sourcegraph: SCIP — A Better Code Indexing Format](https://sourcegraph.com/blog/announcing-scip)
- [Sourcegraph: How Cody Provides Remote Repository Context](https://sourcegraph.com/blog/how-cody-provides-remote-repository-context)
- [Sourcegraph: Agentic Context Fetching](https://sourcegraph.com/docs/cody/capabilities/agentic-context-fetching)
- [arXiv: AI-Assisted Coding with Cody](https://arxiv.org/html/2408.05344v1)
- [Sourcegraph: How Cody Understands Your Codebase](https://sourcegraph.com/blog/how-cody-understands-your-codebase)

---

### Cursor

**Indexing Architecture:**

Cursor uses AST-aware chunking via tree-sitter. Files are parsed into ASTs, then split at meaningful boundaries (function definitions, class bodies). Sibling AST nodes are merged into larger chunks as long as they stay under the token limit.

**Key Innovation — Merkle Tree Change Detection:** A cryptographic hash tree is computed over all files. During re-indexing (every ~10 minutes), only changed subtrees are reprocessed. Embeddings are cached by chunk hash, so unchanged chunks are never re-embedded.

**Vector Storage:** Turbopuffer — a serverless vector database optimized for fast nearest-neighbor search.

**Privacy:** File paths are obfuscated client-side before transmission (each path component is masked using a secret key and nonce). Actual code content is retrieved locally after the server returns obfuscated paths + line ranges.

**Retrieval:** Embedding nearest-neighbor search followed by a **cross-encoder reranker** that reorders results by relevance before injection into LLM context.

**Sources:**
- [Towards Data Science: How Cursor Actually Indexes Your Codebase](https://towardsdatascience.com/how-cursor-actually-indexes-your-codebase/)
- [Engineer's Codex: How Cursor Indexes Codebases Fast](https://read.engineerscodex.com/p/how-cursor-indexes-codebases-fast)
- [Cursor Blog: Securely Indexing Large Codebases](https://cursor.com/blog/secure-codebase-indexing)
- [Cursor Docs: Codebase Indexing](https://cursor.com/docs/context/codebase-indexing)

---

### Qodo (formerly CodiumAI)

**Indexing Architecture:**

Qodo's Context Engine indexes repositories and creates a "structured, multi-layered understanding." Chunks include natural-language descriptions. Indexes past PR diffs, comments, discussions, and resolved issues alongside code.

**Key Innovation — Qodo-Embed-1:** A purpose-built code embedding model. The 1.5B parameter variant scores 68.53 on the CoIR (Code Information Retrieval) benchmark, surpassing larger 7B models from other vendors. The 7B variant scores 71.5. Trained with contrastive learning using high-quality synthetic data and real-world code samples.

**Multi-Agent Review:** Specialized expert agents each focus on one review dimension (security, performance, code quality). Each agent operates within its own context loaded with domain-specific knowledge (vulnerability taxonomies, complexity heuristics).

**Cross-Repo:** The Context Engine maps dependencies and shared modules across repositories. Detects cross-service breaking changes.

**Sources:**
- [Qodo Blog: Qodo-Embed-1](https://www.qodo.ai/blog/qodo-embed-1-code-embedding-code-retrieval/)
- [Qodo Docs: Context Engine Overview](https://docs.qodo.ai/qodo-documentation/qodo-gen/code-intelligence/context-engine)
- [Qodo Blog: Next Generation AI Code Review](https://www.qodo.ai/blog/the-next-generation-of-ai-code-review-from-isolated-to-system-intelligence/)

---

### Bloop

**Company:** YC S21, open-source (Rust). Fully local code search.

**Architecture:** On-device MiniLM embeddings (no cloud dependency for indexing). Qdrant for vector storage. Tantivy (Rust-based, Lucene-equivalent) for full-text/trigram search. Tree-sitter for code navigation. Cold index of 1.3M-line monorepo: ~4 min 20s. Incremental sync after 200-commit pull: <15 seconds.

**Retrieval:** GPT-4 rewrites NL questions into keyword queries, which are then embedded and matched against Qdrant. Retrieved snippets are ranked, trimmed, and injected into a second GPT-4 prompt.

**Sources:**
- [Qdrant: Powering Bloop Semantic Code Search](https://qdrant.tech/blog/case-study-bloop/)
- [GitHub: BloopAI/bloop](https://github.com/BloopAI/bloop)

---

### GitHub Copilot

**Embedding Model:** Proprietary transformer trained with contrastive learning (InfoNCE loss) and Matryoshka Representation Learning (MRL), which supports multiple embedding granularities from small fragments to entire files. Hard negatives strategy: trained on code that looks correct but isn't, teaching the model to distinguish "almost right" from "actually right." 2025 model: 37.6% retrieval quality improvement, 2x throughput, 8x smaller index size.

**Code Review:** Uses GitHub Actions for agentic context gathering. Integrates CodeQL (semantic code analysis), ESLint, PMD as deterministic static analysis tools separate from the AI pipeline.

**Sources:**
- [GitHub Blog: Copilot New Embedding Model](https://github.blog/news-insights/product-news/copilot-new-embedding-model-vs-code/)
- [GitHub Docs: Indexing Repositories for Copilot](https://docs.github.com/copilot/concepts/indexing-repositories-for-copilot-chat)
- [GitHub Docs: About Copilot Code Review](https://docs.github.com/en/copilot/concepts/agents/code-review)

---

## 3. Gap Analysis

### Gap 1: Raw Code Embedding vs. Semantic Description Embedding

#### Current State

In `internal/extractor/walker.go:49-51`, we truncate the raw function body to 3000 characters and embed it directly:

```go
embedText := bodyStr
if len(embedText) > 3000 {
    embedText = embedText[:3000]
}
```

The `EmbedText` field is then passed straight to the OpenAI embedding API in `main.go:197-199`:

```go
texts := make([]string, len(symbols))
for idx, sym := range symbols {
    texts[idx] = sym.EmbedText
}
```

No transformation, summarization, or enrichment happens between extraction and embedding.

#### What Industry Leaders Do

**Greptile** generates natural-language docstrings for every function using an LLM before embedding. Their published benchmarks show:
- NL query vs. NL description: **0.815 similarity**
- NL query vs. raw code: **0.728 similarity**
- Delta: **+12% retrieval quality**

The recursive nature is critical: leaf functions get summaries first, then their callers get summaries that reference their callees' summaries. A function like `handleCheckout()` gets a description like "Processes a customer checkout by validating the cart via validateCart(), calculating totals including tax via calculateOrderTotal(), charging the payment method via processPayment(), and sending a confirmation email via notifyCustomer()." This captures the entire call chain in a single embedding.

**Qodo** adds "natural-language descriptions to each chunk" before embedding.

**CodeRabbit** enriches its LanceDB entries with semantic descriptions alongside code.

#### Why This Matters for Code Review

A code review agent asks: "Where is the authentication logic?" Our system embeds that query and compares it against raw code like:

```go
func ValidateToken(tokenStr string) (*Claims, error) {
    token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
```

The semantic overlap between the NL query and the code tokens is weak. An NL description like "Validates a JWT authentication token by parsing it, checking the signing method, verifying expiration, and returning the decoded claims" would match far better.

#### Recommended Solution

**Phase 1 — Signature + context enrichment (no LLM required):**

Construct `EmbedText` as a structured string instead of raw body:

```
[language] [node_type]
Name: calculateOrderTotal
File: internal/billing/calculator.go
Signature: func calculateOrderTotal(items []LineItem, taxRate float64) (Money, error)
Body: <first 2000 chars of body>
```

This adds language context and separates the signature (high semantic value) from the body.

**Phase 2 — LLM summary generation during indexing:**

After extraction, batch-process symbols through the LLM proxy to generate one-line summaries. Embed the summary concatenated with the signature.

Ordering matters: process leaf functions first (those that don't call other indexed functions), then work upward so caller summaries can reference callee summaries. This requires the relationship graph from Gap 2.

**Phase 3 — Recursive docstring propagation (Greptile approach):**

Once the call graph exists, generate summaries bottom-up through the tree. Each function's summary includes what its callees do. This is the gold standard.

#### Impact Assessment

| Metric | Current | After Phase 1 | After Phase 2 | After Phase 3 |
|--------|---------|---------------|---------------|---------------|
| Retrieval similarity (NL query) | ~0.72 (estimated) | ~0.74 | ~0.81 | ~0.83 |
| Indexing cost | Embedding only | Embedding only | +LLM calls | +LLM calls (more) |
| Indexing time | Seconds | Seconds | +Minutes | +Minutes |
| Implementation effort | — | Low (string formatting) | Medium (batch LLM) | High (requires graph) |

---

### Gap 2: No Code Graph or Relationship Tracking

#### Current State

Our `symbols` table is completely flat. No edges, no relationships, no import tracking. Each symbol is an island. The schema in `internal/store/schema.go` contains only `symbols`, `vec_symbols`, and `repo_meta`.

The AST walker in `internal/extractor/walker.go:33-71` extracts function/class nodes but discards all information about what those functions call, import, or depend on.

#### What Industry Leaders Do

**Sourcegraph (SCIP):** The most sophisticated approach. Language-specific SCIP indexers produce a full semantic graph: every definition, every reference, cross-file and cross-repo. Powers go-to-definition, find-references, and call chain traversal. This is a separate binary per language (`scip-typescript`, `scip-java`, etc.).

**Greptile:** Builds a code graph during indexing with explicit node types (files, functions, classes, variables, external calls) and edge types (function calls, imports, variable usage, library references). Three-stage process: repository scanning → relationship mapping → graph storage.

**CodeRabbit:** Builds a lightweight dependency graph on-the-fly at review time using ast-grep. Maps definitions and references across changed files and their dependencies. Also performs co-change analysis from commit history.

**Qodo:** Maps dependencies and shared modules across repositories. Detects cross-service breaking changes (e.g., a shared API change breaking downstream consumers).

#### Why This Matters for Code Review

Without a graph, when a PR changes `calculateDiscount()`, our system can only find functions with similar names or body content. It cannot tell the reviewer:

- `generateInvoice()` calls `applyPricing()` which calls `calculateDiscount()` — this is a critical path
- `generateMonthlyReport()` also calls `applyPricing()` — it will be affected
- `calculateDiscount()` is imported by 3 other packages — they all need testing
- The function was changed 4 times in the last month alongside `taxCalculator.go` — these files are coupled

This is the difference between "here are similar functions" and "here is the blast radius of your change."

#### Recommended Solution

**Phase 1 — Import edge extraction:**

During AST walking, also extract import/require/use statements. These are easily identifiable tree-sitter node types:

| Language | Import Node Types |
|----------|------------------|
| Go | `import_declaration`, `import_spec` |
| TypeScript/JS | `import_statement`, `import_clause` |
| Python | `import_statement`, `import_from_statement` |
| Java | `import_declaration` |
| Rust | `use_declaration` |

Store in a new table:

```sql
CREATE TABLE edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_symbol_id INTEGER,        -- NULL for file-level imports
    source_file TEXT NOT NULL,
    target_name TEXT NOT NULL,        -- the imported symbol/module name
    target_file TEXT,                 -- resolved path if possible, NULL otherwise
    edge_type TEXT NOT NULL,          -- 'imports', 'calls', 'extends', 'implements'
    repo_name TEXT NOT NULL
);

CREATE INDEX idx_edges_source ON edges(source_file);
CREATE INDEX idx_edges_target ON edges(target_name);
CREATE INDEX idx_edges_repo ON edges(repo_name);
```

**Phase 2 — Approximate call edge extraction:**

Inside each extracted function body, scan for identifier references that match known symbol names in the same repo. This is approximate (no type resolution) but catches the majority of same-repo calls:

```go
// After extracting all symbols, build a name→symbol map
// For each symbol's body, scan for references to other known symbols
// Store as edges with edge_type = 'calls'
```

**Phase 3 — Co-change analysis:**

Use `git log` to identify files that frequently change together. Store as weighted edges:

```sql
CREATE TABLE co_changes (
    file_a TEXT NOT NULL,
    file_b TEXT NOT NULL,
    change_count INTEGER NOT NULL,
    repo_name TEXT NOT NULL,
    PRIMARY KEY (file_a, file_b, repo_name)
);
```

#### Impact Assessment

| Metric | Current | After Phase 1 | After Phase 2 | After Phase 3 |
|--------|---------|---------------|---------------|---------------|
| Blast radius detection | None | Import-level | Call-level (approx) | + Historical coupling |
| Cross-file context | None | Which files import what | Which functions call which | + Implicit coupling |
| Indexing time overhead | — | +Seconds (AST walk extension) | +Seconds (name matching) | +Seconds (git log parse) |
| Storage overhead | — | ~1 row per import | ~5-20 rows per function | ~1 row per file pair |
| Implementation effort | — | Low (extend walker) | Medium (name resolution) | Low (git log parsing) |

---

### Gap 3: Single Retrieval Modality (No Keyword Search)

#### Current State

We have exactly one retrieval path — vector similarity search in `internal/store/store.go:228-293`:

```go
rows, err := s.db.Query(`
    SELECT vec_symbols.rowid, vec_symbols.distance
    FROM vec_symbols
    WHERE embedding MATCH ?
    ORDER BY distance
    LIMIT ?
`, vecBytes, fetchLimit)
```

No keyword search, no full-text index, no BM25.

#### What Industry Leaders Do

**Greptile:** Triple-modality — vector similarity + keyword search + agentic graph traversal.

**Sourcegraph:** Hybrid dense-sparse vectors. Sparse vectors handle keyword search with LLM-based term expansion and ranking. Dense vectors handle semantic similarity.

**Bloop:** Qdrant (vector) + Tantivy (full-text trigram index). Dual modality.

**The industry consensus** is that hybrid retrieval (vector + keyword) significantly outperforms either modality alone for code search. The reason is straightforward: vector search excels at semantic intent ("where is the authentication logic?") but fails at exact identifiers ("find handleWebhookPayload"). Keyword search does the reverse.

#### Why This Matters for Code Review

A code review agent needs both capabilities:
- **Semantic:** "Find functions that validate user input" → vector search
- **Exact:** "Where is `processStripeWebhook` used?" → keyword search
- **Hybrid:** "Find the error handling for database timeouts" → benefits from both

Currently, an exact identifier search through our system embeds the query, which may return semantically similar but wrong functions.

#### Recommended Solution

SQLite natively supports FTS5 (Full-Text Search 5). No new dependency needed.

**Schema addition:**

```sql
CREATE VIRTUAL TABLE symbols_fts USING fts5(
    name,
    body,
    file_path,
    content=symbols,
    content_rowid=id
);

-- Triggers to keep FTS in sync
CREATE TRIGGER symbols_fts_insert AFTER INSERT ON symbols BEGIN
    INSERT INTO symbols_fts(rowid, name, body, file_path)
    VALUES (new.id, new.name, new.body, new.file_path);
END;

CREATE TRIGGER symbols_fts_delete AFTER DELETE ON symbols BEGIN
    INSERT INTO symbols_fts(symbols_fts, rowid, name, body, file_path)
    VALUES ('delete', old.id, old.name, old.body, old.file_path);
END;
```

**Hybrid search function:**

```go
func (s *Store) HybridSearch(queryVec []float32, queryText string, limit int, ...) ([]SearchResult, error) {
    // 1. Vector search (existing)
    vecResults := s.SearchSimilar(queryVec, limit*2, ...)

    // 2. FTS5 keyword search
    ftsResults := s.KeywordSearch(queryText, limit*2, ...)

    // 3. Reciprocal Rank Fusion (RRF) to merge
    // score(doc) = sum(1 / (k + rank_in_list)) for each list containing doc
    // k = 60 (standard RRF constant)
    merged := reciprocalRankFusion(vecResults, ftsResults, k=60)

    return merged[:limit], nil
}
```

Reciprocal Rank Fusion (RRF) is the standard merging strategy — simple, effective, no tuning required.

#### Impact Assessment

| Metric | Current | After FTS5 |
|--------|---------|-----------|
| Exact identifier search | Poor (embedding similarity) | Exact match |
| Semantic search | Good | Good (unchanged) |
| Hybrid queries | Poor | Strong (RRF fusion) |
| Storage overhead | — | ~30-50% increase (FTS index) |
| Query latency | ~ms | +~ms (two queries + merge) |
| Implementation effort | — | Low (~100 lines, SQLite native) |

---

### Gap 4: No Re-Ranking Stage

#### Current State

In `internal/store/store.go:232-238`, results are returned in raw cosine distance order. The over-fetch of 5x (`limit * 5`) exists only to accommodate post-filtering by repo/language — not for re-ranking:

```go
fetchLimit := limit * 5 // over-fetch for filtering
```

#### What Industry Leaders Do

**Cursor:** Cross-encoder reranker reorders initial retrieval results. Cross-encoders see the query and candidate together (unlike bi-encoders which embed them separately), catching nuances that independent embeddings miss.

**Sourcegraph:** Graph-based link prediction on the Repo-level Semantic Graph (RSG) for re-ranking.

**Greptile:** LLM agent reviews and re-ranks results for relevance.

**The research consensus** is that bi-encoder retrieval (what we use) is fast but coarse. A two-stage pipeline — fast retrieval then precise re-ranking — is standard practice in production search systems.

#### Why This Matters for Code Review

When searching for "database connection pooling logic," our top results might include:
1. `func NewPool(...)` — correct, highly relevant
2. `func ParseDatabaseURL(...)` — related but not pooling logic
3. `func TestPoolTimeout(...)` — a test, less useful for understanding production code
4. `func poolMetrics(...)` — monitoring, tangentially related

A re-ranker would promote #1 and demote #3 and #4 based on understanding what "connection pooling logic" actually means in context.

#### Recommended Solution

**Option A — Heuristic re-ranking (zero cost, immediate):**

Apply boosting heuristics after vector retrieval:
- Boost exact name matches (+0.1 similarity)
- Boost query terms appearing in file path (+0.05)
- Penalize test files (-0.05)
- Boost functions over types when query implies behavior ("how does X work")

**Option B — LLM-based re-ranking (higher quality, higher cost):**

Pass top-20 candidates to the LLM proxy with a scoring prompt:

```
Given query: "database connection pooling logic"
Rate each candidate 0-10 for relevance:
1. func NewPool(...) in db/pool.go — [body preview]
2. func ParseDatabaseURL(...) in db/config.go — [body preview]
...
```

This is expensive per-query but very effective. Suitable when the downstream consumer (code review agent) is already making LLM calls anyway.

**Option C — Cross-encoder model (best quality/cost ratio):**

Use a lightweight cross-encoder like `ms-marco-MiniLM-L-6-v2` to score (query, candidate) pairs. Runs in ~5ms per pair, so 20 candidates = ~100ms. Requires hosting the model or using an API.

#### Impact Assessment

| Metric | Current | Heuristic (A) | LLM (B) | Cross-Encoder (C) |
|--------|---------|--------------|---------|-------------------|
| Ranking quality | Coarse | Better | Best | Very good |
| Latency overhead | — | ~0ms | +500-2000ms | +100ms |
| Cost per query | — | Free | ~$0.001-0.005 | Hosting cost |
| Implementation effort | — | Low | Low | Medium (model hosting) |

---

### Gap 5: Isolated Symbol Results (No Surrounding Context)

#### Current State

Search results return a single symbol with a 200-character body preview (`internal/store/store.go:270-273`):

```go
preview := body
if len(preview) > 200 {
    preview = preview[:200] + "..."
}
```

The result contains: name, file path, line range, node type, language, distance, similarity, and body preview. Nothing about what surrounds the symbol, what calls it, what it calls, or what file it lives in.

#### What Industry Leaders Do

**CodeRabbit:** 1:1 code-to-context ratio. For every line of code under review, includes equal context: callers, callees, related tests, import context, issue tracker links.

**Greptile:** Returns the function plus its recursive docstring (summarizing the entire call chain), plus graph-connected related code.

**Sourcegraph:** Returns the symbol plus its SCIP context: all definitions, references, and cross-file relationships.

#### Why This Matters for Code Review

When a review agent retrieves `func calculateTax(...)`, it needs to know:
- What struct/class is this a method of?
- What does this file import? (reveals dependencies)
- What other functions are in the same file? (reveals the module's purpose)
- What calls this function? (reveals impact)
- Is there a test file for this? (reveals test coverage)

Returning isolated symbols forces the consuming agent to make N additional retrieval calls to build context, increasing latency and reducing review quality.

#### Recommended Solution

Enrich `SearchResult` to include contextual data:

```go
type SearchResult struct {
    // Existing fields
    Name        string
    FilePath    string
    StartLine   int
    EndLine     int
    NodeType    string
    Language    string
    Distance    float32
    Similarity  float32
    BodyPreview string

    // New context fields
    FileImports    []string      // import statements from the file
    ContainingType string        // parent class/struct if this is a method
    SiblingSymbols []string      // other symbol names in the same file
    Callers        []string      // symbols that call this (from edges table)
    Callees        []string      // symbols this calls (from edges table)
}
```

Populate these with JOIN queries against the `edges` table (Gap 2) and the `symbols` table:

```sql
-- Sibling symbols in the same file
SELECT name, node_type FROM symbols
WHERE file_path = ? AND repo_name = ? AND id != ?
ORDER BY start_line;

-- Callers (requires edges table from Gap 2)
SELECT source_file, source_symbol_id FROM edges
WHERE target_name = ? AND edge_type = 'calls';
```

#### Impact Assessment

| Metric | Current | After Enrichment |
|--------|---------|-----------------|
| Context per result | 200-char preview | Full context (imports, siblings, callers, callees) |
| Queries per search result | 1 | 3-4 JOINs (still fast in SQLite) |
| Downstream round-trips needed | Many | Fewer (context already included) |
| Review agent quality | Must self-gather context | Context pre-gathered |
| Implementation effort | — | Low-Medium (depends on Gap 2 being done first) |

---

### Gap 6: File-Level Incremental Indexing (No Symbol-Level Caching)

#### Current State

In `main.go:246-312`, incremental indexing works at file granularity. When a file changes, ALL symbols in that file are deleted and re-extracted/re-embedded:

```go
// Delete old symbols for changed files
if err := db.DeleteSymbolsByFiles(repoName, relPaths); err != nil {
    return fmt.Errorf("deleting changed symbols: %w", err)
}

// Extract from changed files only
symbols, err := extractor.ExtractFiles(repoPath, driftResult.ChangedFiles)
```

If a file has 20 functions and only 1 changed, all 20 are re-embedded. At $0.13 per 1M tokens (`text-embedding-3-large`), this waste is proportional to file size.

#### What Industry Leaders Do

**Cursor:** Merkle tree hashing over all files. Embeddings are cached by chunk hash, so re-indexing the same codebase is much faster. Only changed subtrees are reprocessed. Runs every ~10 minutes.

**Bloop:** Incremental sync after a 200-commit pull: <15 seconds for a 1.3M-line monorepo.

#### Recommended Solution

Add a content hash to each symbol and skip re-embedding for unchanged symbols:

**Schema change:**

```sql
ALTER TABLE symbols ADD COLUMN body_hash TEXT;
CREATE INDEX idx_symbols_hash ON symbols(body_hash);
```

**Logic change in incremental indexing:**

```go
// For each extracted symbol from changed files:
hash := sha256Hex(symbol.Body)

// Check if a symbol with this exact hash already exists at this location
existing := db.GetSymbolByHash(repoName, symbol.FilePath, symbol.Name, hash)
if existing != nil {
    // Symbol unchanged — skip embedding, keep existing vector
    continue
}
// Symbol is new or changed — embed and insert
```

**Cost savings estimate:** If a typical incremental index touches 5 files with an average of 15 symbols each, and only 20% of symbols actually changed, this saves 80% of embedding API calls on incremental updates.

#### Impact Assessment

| Metric | Current | After Hash Caching |
|--------|---------|-------------------|
| Embedding calls (incremental) | All symbols in changed files | Only actually changed symbols |
| Estimated cost reduction | — | 60-80% on incremental updates |
| Indexing time (incremental) | Proportional to file count × symbols | Proportional to changed symbols only |
| Storage overhead | — | +32 bytes per symbol (SHA-256 hex) |
| Implementation effort | — | Low (~30 lines + schema migration) |

---

### Gap 7: No Non-Symbol Content Indexing

#### Current State

The extractor in `internal/extractor/walker.go` only captures AST nodes that match `spec.FunctionTypes` or `spec.ClassTypes`. Everything else is discarded:

```go
if spec.FunctionTypes[nodeType] || spec.ClassTypes[nodeType] {
    // extract
}
```

**What gets skipped:**
- Configuration files (YAML, JSON, TOML, Dockerfile, docker-compose, Makefile)
- SQL migrations
- Environment variable definitions
- Constants, type aliases, global variables, enums (in some languages)
- README and documentation
- CI/CD pipeline definitions (.github/workflows, Jenkinsfile)
- Package manifests (package.json, go.mod, Cargo.toml)

#### What Industry Leaders Do

**CodeRabbit:** Indexes PR diffs, issue tracker content, past review comments, and coding guidelines (`.cursorrules`, `CLAUDE.md`, `.github/copilot-instructions.md`) — not just source code.

**Qodo:** Indexes past PR diffs, comments, discussions, and resolved issues alongside code.

**Greptile:** Indexes directory structure and file metadata as part of its code graph.

#### Why This Matters for Code Review

When reviewing a PR that adds a new database migration, the review agent should be able to retrieve:
- The migration SQL file itself
- The ORM model definition it relates to
- The docker-compose service configuration for the database
- Previous migration files that modified the same table

Currently none of these are retrievable because they're not indexed.

#### Recommended Solution

**Phase 1 — Add a fallback text chunker for non-AST files:**

For files that don't have a tree-sitter language spec, split by logical blocks:
- YAML/TOML: top-level keys
- SQL: statement boundaries (`;`)
- Markdown: heading-delimited sections
- JSON: top-level object keys
- Generic: blank-line-separated paragraphs, max 3000 chars per chunk

Store with `node_type = 'text_chunk'` and appropriate language metadata.

**Phase 2 — Index type definitions and constants:**

Extend the AST walker to capture additional node types:
- Go: `const_declaration`, `var_declaration`, `type_alias`
- TypeScript: `type_alias_declaration`, `enum_declaration`, `interface_declaration`
- Python: top-level assignments

#### Impact Assessment

| Metric | Current | After Phase 1 | After Phase 2 |
|--------|---------|---------------|---------------|
| File types indexed | 13 languages (code only) | + Config, SQL, docs, CI | + Constants, types, enums |
| Symbol coverage (code files) | Functions + classes | Functions + classes | + All top-level declarations |
| Embedding cost increase | — | +10-30% (config files are small) | +5-15% |
| Retrieval breadth | Code only | Code + config + docs | Comprehensive |
| Implementation effort | — | Medium (new chunker) | Low (extend walker specs) |

---

### Gap 8: No Feedback or Learning Loop

#### Current State

The system is static. Once indexed, the quality of retrieval never improves based on usage. There is no mechanism to learn which results were useful, which were noise, or what the team's preferences are.

#### What Industry Leaders Do

**Greptile:** Embeds upvoted/downvoted review comments in a vector database partitioned by team. New comments are compared against history: if a comment matches 3+ downvoted comments, it gets blocked. Improved address rate from 19% to 55%.

**CodeRabbit:** Stores developer feedback as vector-embedded natural language in LanceDB. Scoped per-repo or per-org. When `@coderabbitai` receives feedback, it evaluates whether it's systemic and stores it as a "learning." Usage count tracked per learning. Learnings are retrieved at review time and included in the prompt.

**Qodo:** Learns from past PRs, accepted suggestions, and review comments. Adapts to team standards over time.

#### Why This Matters for Code Review

Without learning:
- The review agent repeats the same low-value comments that developers ignore
- Team-specific conventions (e.g., "we don't enforce trailing commas in this repo") are never captured
- The quality ceiling is fixed — no amount of usage improves it

#### Recommended Solution

This is the most complex gap and depends on the broader review system architecture, not just the embeddings service. The embeddings layer can prepare by:

**Phase 1 — Schema preparation:**

```sql
CREATE TABLE learnings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_name TEXT NOT NULL,
    org_id TEXT,
    content TEXT NOT NULL,        -- NL description of the learning
    source_type TEXT NOT NULL,    -- 'upvote', 'downvote', 'explicit', 'implicit'
    source_ref TEXT,              -- PR number, comment ID, etc.
    usage_count INTEGER DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE vec_learnings USING vec0(embedding float[3072]);
```

**Phase 2 — Learning ingestion endpoint:**

Add a CLI command or API endpoint to ingest learnings:

```
ziraloop-embeddings learn --repo=myrepo --content="Don't flag unused imports in test files" --source=downvote --ref=PR#42
```

**Phase 3 — Learning retrieval during search:**

When searching, also query `vec_learnings` for relevant learnings and include them in results. The consuming agent uses these to filter or adjust its review comments.

#### Impact Assessment

| Metric | Current | After Full Implementation |
|--------|---------|--------------------------|
| Review quality over time | Static | Continuously improving |
| Team preference capture | None | Automatic from feedback |
| Noise reduction | None | Suppress repeated low-value patterns |
| Address rate improvement | Baseline | Potentially 2-3x (based on Greptile data) |
| Implementation effort | — | High (requires full feedback loop in the review system) |

---

## 4. Priority Matrix

Ordered by **impact on code review quality** relative to **implementation effort**:

| Priority | Gap | Review Quality Impact | Implementation Effort | Dependencies |
|----------|-----|----------------------|----------------------|-------------|
| **P0** | **Gap 3: Add FTS5 hybrid search** | High | Low (~100 LOC, SQLite native) | None |
| **P0** | **Gap 1 Phase 1: Structured embed text** | High | Low (~20 LOC change in walker) | None |
| **P1** | **Gap 6: Symbol-level hash caching** | Medium (cost savings) | Low (~30 LOC + migration) | None |
| **P1** | **Gap 4 Option A: Heuristic re-ranking** | Medium-High | Low (~50 LOC) | None |
| **P2** | **Gap 2 Phase 1: Import edge extraction** | High | Medium (extend walker per language) | None |
| **P2** | **Gap 5: Enriched search results** | High | Low-Medium (query changes) | Gap 2 Phase 1 |
| **P2** | **Gap 2 Phase 2: Approximate call edges** | Very High | Medium (name matching) | Gap 2 Phase 1 |
| **P3** | **Gap 1 Phase 2: LLM summary generation** | Very High | Medium (batch LLM pipeline) | None |
| **P3** | **Gap 7 Phase 2: Constants and types** | Medium | Low (extend language specs) | None |
| **P3** | **Gap 7 Phase 1: Non-code file indexing** | Medium | Medium (new chunker) | None |
| **P4** | **Gap 2 Phase 3: Co-change analysis** | Medium | Low (git log parsing) | None |
| **P4** | **Gap 1 Phase 3: Recursive docstrings** | Highest | High (requires graph) | Gap 2 Phase 2 |
| **P4** | **Gap 4 Option B/C: LLM/Cross-encoder reranking** | High | Medium (model hosting or API) | None |
| **P5** | **Gap 8: Feedback learning loop** | High (long-term) | High (system-wide) | Broader review system |

**Recommended execution order for maximum impact with minimum risk:**

1. **Sprint 1 (P0):** Structured embed text + FTS5 hybrid search. These are independent, low-effort, and immediately improve retrieval quality.
2. **Sprint 2 (P1):** Symbol hash caching + heuristic re-ranking. Reduces cost and improves result ordering.
3. **Sprint 3 (P2):** Import edges + enriched results + approximate call edges. This is the big structural upgrade.
4. **Sprint 4 (P3):** LLM summaries + extended type indexing. Requires the most new infrastructure.
5. **Later:** Recursive docstrings, advanced re-ranking, feedback loop. These are the long-tail improvements.

---

## 5. What We Do Well

Our system gets several things right that are validated by industry practice:

| Strength | Validation |
|----------|-----------|
| **AST-level chunking via Tree-sitter** | Same approach as Greptile, CodeRabbit (via ast-grep), and Cursor. Greptile's published research confirms function-level chunks outperform file-level by 7%. |
| **Function/class granularity** | Matches the industry standard. Greptile explicitly tested and confirmed this granularity. |
| **13-language support** | Competitive with most tools. Only Sourcegraph (14+ via SCIP) covers more. |
| **SQLite + sqlite-vec** | Operationally simpler than Qdrant/Pinecone/pgvector. Perfectly adequate for single-repo or small-multi-repo scale. Bloop also uses a single-file approach (Qdrant embedded). |
| **Incremental indexing via git diff** | Smart design. All tools do some version of this. Cursor's Merkle tree is more granular but also more complex. |
| **Drive sync pattern** | Pull-if-exists → index → push is clean for per-sandbox ephemeral environments. No equivalent in other tools (they're not designed for sandbox-per-agent architecture). |
| **Parallel batch embedding** | Goroutine-based parallel batching with timing tracking. Solid engineering. |
| **WAL mode** | Enables concurrent reads during writes. Correct choice for a system that may search while indexing. |

**The foundation is architecturally sound.** The gaps are all additive — layering new capabilities on top of a working system, not rearchitecting from scratch.

---

## 6. References

### Greptile
1. [Codebases are uniquely hard to search semantically](https://www.greptile.com/blog/semantic-codebase-search) — Benchmarks on chunking strategies and NL vs. code embedding
2. [Greptile v3: Agentic Code Review](https://www.greptile.com/blog/greptile-v3-agentic-code-review) — Claude Agent SDK loop architecture, sub-agents, tool use
3. [Graph-based Codebase Context](https://www.greptile.com/docs/how-greptile-works/graph-based-codebase-context) — Node types, edge types, 3-stage graph construction
4. [Hatchet x Greptile Case Study](https://hatchet.run/customers/greptile) — Workflow durability, recursive docstring bottlenecks, Linux kernel indexing
5. [Improving AI Code Review Comment Quality via Vector Embeddings (ZenML)](https://www.zenml.io/llmops-database/improving-ai-code-review-bot-comment-quality-through-vector-embeddings) — Feedback system, 19% to 55% address rate
6. [Claude Customer Story: Greptile](https://claude.com/customers/greptile) — ~90% prompt cache hit rates, Claude Opus 4.5 usage
7. [HN Launch Thread](https://news.ycombinator.com/item?id=39604961) — Founder comments on recursive docstrings, triple-modality retrieval
8. [Greptile Self-Hosting Docker Compose](https://www.greptile.com/docs/self-hosting/docker-compose) — pgvector in PostgreSQL, Redis, infrastructure
9. [Greptile Seed Announcement](https://www.greptile.com/blog/seed) — Agentic search description

### CodeRabbit
10. [How CodeRabbit Built Its AI Code Review Agent (Google Cloud Blog)](https://cloud.google.com/blog/products/ai-machine-learning/how-coderabbit-built-its-ai-code-review-agent-with-google-cloud-run) — Cloud Run architecture, 3-layer sandboxing, ephemeral environments
11. [Accurate AI Code Reviews on Massive Codebases](https://www.coderabbit.ai/blog/how-coderabbit-delivers-accurate-ai-code-reviews-on-massive-codebases) — Codegraph, ast-grep, co-change analysis
12. [The Art and Science of Context Engineering](https://www.coderabbit.ai/blog/the-art-and-science-of-context-engineering) — 5-layer context pipeline, 1:1 code-to-context ratio
13. [Context Engineering: Level Up AI Code Reviews](https://www.coderabbit.ai/blog/context-engineering-ai-code-reviews) — Knowledge integration, path-based instructions
14. [LanceDB Case Study: CodeRabbit](https://lancedb.com/blog/case-study-coderabbit/) — LanceDB as semantic engine, sub-second P99, continuous ingestion
15. [Agentic Code Validation](https://www.coderabbit.ai/blog/how-coderabbits-agentic-code-validation-helps-with-code-reviews) — Verification scripts, sandbox execution, hallucination reduction
16. [AI-Native Universal Linter: ast-grep + LLM](https://www.coderabbit.ai/blog/ai-native-universal-linter-ast-grep-llm) — ast-grep integration, custom YAML rules
17. [End of One-Size-Fits-All Prompts](https://www.coderabbit.ai/blog/the-end-of-one-sized-fits-all-prompts-why-llm-models-are-no-longer-interchangeable) — Model-specific prompt subunits, behavioral characterization
18. [CodeRabbit Learnings Docs](https://docs.coderabbit.ai/knowledge-base/learnings) — Scoping, usage tracking, bulk import
19. [CodeRabbit Multi-Repo Analysis](https://www.coderabbit.ai/blog/Coderabbit-multi-repo-analysis) — Cross-repo dependency detection, linked repos
20. [ast-grep-essentials (GitHub)](https://github.com/coderabbitai/ast-grep-essentials) — Public AST rule repository
21. [Software Engineering Daily: CodeRabbit and RAG for Code Review](https://softwareengineeringdaily.com/2025/06/24/coderabbit-and-rag-for-codereview-with-harjot-gill/) — Architecture discussion

### Sourcegraph Cody / Amp
22. [SCIP — A Better Code Indexing Format](https://sourcegraph.com/blog/announcing-scip) — Protobuf-based, language-specific indexers, 14+ languages
23. [How Cody Provides Remote Repository Context](https://sourcegraph.com/blog/how-cody-provides-remote-repository-context) — Hybrid dense-sparse retrieval, 100K+ lines context
24. [Agentic Context Fetching](https://sourcegraph.com/docs/cody/capabilities/agentic-context-fetching) — Mini-agent with code search, file access, terminal, web browser tools
25. [arXiv: AI-Assisted Coding with Cody](https://arxiv.org/html/2408.05344v1) — RSG (Repo-level Semantic Graph), Expand and Refine ranking
26. [How Cody Understands Your Codebase](https://sourcegraph.com/blog/how-cody-understands-your-codebase) — Context architecture overview
27. [Cody Context Docs](https://sourcegraph.com/docs/cody/core-concepts/context) — Context sources, keyword search, embeddings, agentic

### Cursor
28. [How Cursor Actually Indexes Your Codebase (Towards Data Science)](https://towardsdatascience.com/how-cursor-actually-indexes-your-codebase/) — Merkle tree hashing, AST-aware chunking, sibling node merging
29. [How Cursor Indexes Codebases Fast (Engineer's Codex)](https://read.engineerscodex.com/p/how-cursor-indexes-codebases-fast) — Turbopuffer, chunk hash caching, cross-encoder reranking
30. [Securely Indexing Large Codebases (Cursor Blog)](https://cursor.com/blog/secure-codebase-indexing) — Path obfuscation, privacy design
31. [Codebase Indexing (Cursor Docs)](https://cursor.com/docs/context/codebase-indexing) — Re-indexing cadence, auto-indexing behavior

### Qodo (formerly CodiumAI)
32. [Qodo-Embed-1](https://www.qodo.ai/blog/qodo-embed-1-code-embedding-code-retrieval/) — 1.5B model, 68.53 CoIR score, contrastive learning, hard negatives
33. [Context Engine Overview (Qodo Docs)](https://docs.qodo.ai/qodo-documentation/qodo-gen/code-intelligence/context-engine) — Multi-layered understanding, cross-repo dependency mapping
34. [Next Generation AI Code Review](https://www.qodo.ai/blog/the-next-generation-of-ai-code-review-from-isolated-to-system-intelligence/) — Multi-agent architecture, specialized expert agents
35. [Qodo-Embed-1 Press Release](https://www.prnewswire.com/news-releases/qodo-achieves-best-code-embedding-performance-with-small-1-5-billion-parameter-model-302387275.html) — Benchmark comparisons

### Bloop
36. [Powering Bloop Semantic Code Search (Qdrant)](https://qdrant.tech/blog/case-study-bloop/) — MiniLM, Qdrant, Tantivy, performance benchmarks
37. [BloopAI/bloop (GitHub)](https://github.com/BloopAI/bloop) — Open-source Rust codebase

### GitHub Copilot
38. [Copilot New Embedding Model (GitHub Blog)](https://github.blog/news-insights/product-news/copilot-new-embedding-model-vs-code/) — Matryoshka Representation Learning, hard negatives, 37.6% improvement
39. [Indexing Repositories for Copilot (GitHub Docs)](https://docs.github.com/copilot/concepts/indexing-repositories-for-copilot-chat) — Auto-indexing, 750-file threshold, shared per-repo index
40. [About Copilot Code Review (GitHub Docs)](https://docs.github.com/en/copilot/concepts/agents/code-review) — CodeQL integration, agentic tool calling

### General
41. [State of AI Code Review Tools 2025 (DevTools Academy)](https://www.devtoolsacademy.com/blog/state-of-ai-code-review-tools-2025/) — Market comparison
42. [Sacra: Greptile Analysis](https://sacra.com/c/greptile/) — Market positioning, competitive landscape
