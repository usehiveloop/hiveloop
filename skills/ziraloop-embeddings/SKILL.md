---
name: ziraloop-embeddings
description: Semantic code search via vector embeddings. Use when you need to find functions by meaning, discover code patterns, locate similar implementations, check for duplicates, find related tests, or understand what conventions exist in a codebase. Triggers on "find similar functions", "what patterns exist", "find code that does X", "are there functions like this", "search the codebase for", "find related code", "what other handlers exist", "check for duplicate logic", "find tests for this area".
---

# ziraloop-embeddings — Semantic Code Search

A CLI tool that indexes codebases into vector embeddings for semantic similarity search. Use it to find functions by meaning, discover patterns, locate related code, and understand codebase structure.

**Prerequisite**: Run `ziraloop-embeddings init` before any queries. If you get an error about missing environment variables, the org has no OpenAI credential configured.

---

## Quick Reference

| I want to... | Command |
|---|---|
| Index a codebase | `ziraloop-embeddings init --repos=/workspace/repo` |
| Index multiple repos | `ziraloop-embeddings init --repos=/workspace/repo1,/workspace/repo2` |
| Find functions by description | `ziraloop-embeddings search --query="rate limiting middleware"` |
| Find functions similar to one | `ziraloop-embeddings similar --file=path/to/file.go --function=Create` |
| Check what's indexed | `ziraloop-embeddings status` |
| Query the database directly | `ziraloop-embeddings sql --query="SELECT name, file_path FROM symbols WHERE language='go'"` |

---

## Step 1: Index the Codebase

Always run init first. It extracts all functions, classes, types, and interfaces from the codebase, embeds them, and stores the vectors locally. Subsequent runs detect git drift and only re-index changed files.

```bash
ziraloop-embeddings init --repos=/workspace/repo
```

Output:
```
=== Indexing repo ===
[extract] 4749 symbols from map[go:380 typescript:121 tsx:274] in 794ms
[embed] 4749 embeddings in 16.4s (855069 tokens)
[store] 4749 rows in 4.1s
[drive] Upload complete.
Total time: 21.3s
```

On subsequent runs with no changes:
```
[drift] No changes since d5e6516. Skipping.
```

---

## Step 2: Search by Meaning

Find functions semantically related to a natural language description. This is the primary use case — you describe what you're looking for in plain English and get back the most relevant code.

```bash
ziraloop-embeddings search --query="resolve system prompt for provider" --limit=5
```

```
[1] ResolveProviderConfig (go) — similarity: -0.0007
    internal/model/provider_prompts.go:21-52

[2] ProviderPromptConfig (go) — similarity: -0.0145
    internal/model/provider_prompts.go:9-12

[3] TestResolveProviderConfig_EmptyProviderPrompts (go) — similarity: -0.0249
    internal/model/provider_prompts_test.go:9-22
```

**Useful queries for code review:**
```bash
# Find all middleware patterns
ziraloop-embeddings search --query="HTTP middleware" --limit=10

# Find error handling patterns
ziraloop-embeddings search --query="error handling and recovery" --limit=10

# Find authentication logic
ziraloop-embeddings search --query="authentication and authorization check" --limit=10

# Find database operations
ziraloop-embeddings search --query="database query and transaction" --limit=10

# Find test helpers
ziraloop-embeddings search --query="test helper setup function" --limit=10
```

**Filter by language:**
```bash
ziraloop-embeddings search --query="form validation" --language=typescript --limit=5
ziraloop-embeddings search --query="HTTP handler" --language=go --limit=5
```

**Filter by repo** (when multiple repos are indexed):
```bash
ziraloop-embeddings search --query="user model" --repo=myapp --limit=5
```

---

## Step 3: Find Similar Functions (No API Call)

When you already have a function and want to find others like it, use `similar`. This uses the stored embedding vector — no API call, runs in under 30ms.

```bash
ziraloop-embeddings similar --file=internal/sub-agents/seeder.go --function=Seed --limit=5
```

```
Similar to Seed in internal/sub-agents/seeder.go (search: 25ms, no API call)

[1] Seed (go) — similarity: 0.5995
    internal/system-agents/seeder.go:29-60

[2] seedGroup (go) — similarity: 0.4013
    internal/sub-agents/seeder.go:124-169

[3] seedAgent (go) — similarity: 0.3319
    internal/system-agents/seeder.go:62-100
```

**Use cases for code review:**
```bash
# "This new handler looks different from the others — find similar handlers to compare patterns"
ziraloop-embeddings similar --file=internal/handler/agents.go --function=Create

# "Is there duplicate logic? Find functions similar to this one"
ziraloop-embeddings similar --file=internal/trigger/executor.go --function=Execute

# "What test patterns exist for functions like this?"
ziraloop-embeddings similar --file=e2e/agents_api_test.go --function=TestAgentAPI_CRUD
```

---

## Step 4: Query the Database Directly

For structured queries, use SQL against the `symbols` table. No embedding needed — instant results.

```bash
# Count symbols by language
ziraloop-embeddings sql --query="SELECT language, COUNT(*) as count FROM symbols GROUP BY language ORDER BY count DESC"

# Find all functions in a specific file
ziraloop-embeddings sql --query="SELECT name, start_line, end_line FROM symbols WHERE file_path='internal/handler/agents.go' ORDER BY start_line"

# Find all type declarations
ziraloop-embeddings sql --query="SELECT name, file_path FROM symbols WHERE node_type='type_declaration' ORDER BY name"

# Find functions by name pattern
ziraloop-embeddings sql --query="SELECT name, file_path FROM symbols WHERE name LIKE '%Handler%'"

# Find the largest functions (most lines)
ziraloop-embeddings sql --query="SELECT name, file_path, (end_line - start_line) as lines FROM symbols ORDER BY lines DESC LIMIT 10"

# Find all interfaces
ziraloop-embeddings sql --query="SELECT name, file_path FROM symbols WHERE node_type='interface_declaration'"

# Count functions per file (find complex files)
ziraloop-embeddings sql --query="SELECT file_path, COUNT(*) as funcs FROM symbols GROUP BY file_path ORDER BY funcs DESC LIMIT 10"
```

---

## Check Index Status

```bash
ziraloop-embeddings status
```

```
Database: /tmp/ziraloop-vectors.db (54.8 MB)

  myapp
    Path:       /workspace/repo
    Commit:     d5e6516f
    Symbols:    4749
    Model:      text-embedding-3-large
    Indexed at: 2026-04-14T09:49:25Z

By language:
  go              3357
  tsx             979
  typescript      395
  python          18

By type:
  function_declaration    2599
  method_declaration      946
  type_declaration        694
  type_alias_declaration  188
  interface_declaration   169
```

---

## Database Schema

The `symbols` table stores all extracted code:

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER | Auto-increment primary key |
| `repo_name` | TEXT | Which repo this symbol belongs to |
| `name` | TEXT | Symbol name (e.g. `Create`, `AgentHandler`) |
| `file_path` | TEXT | Relative path (e.g. `internal/handler/agents.go`) |
| `start_line` | INTEGER | First line of the symbol |
| `end_line` | INTEGER | Last line of the symbol |
| `node_type` | TEXT | Tree-sitter node type (e.g. `function_declaration`) |
| `language` | TEXT | Language: `go`, `typescript`, `tsx`, `python`, etc. |
| `body` | TEXT | Full source code of the symbol |

Indexes on: `name`, `file_path`, `language`, `repo_name`.

---

## Supported Languages

Go, TypeScript, TSX, JavaScript, Python, Rust, Java, C, C++, C#, Ruby, PHP.

Extracts: functions, methods, classes, interfaces, structs, enums, type declarations, type aliases.

---

## Code Review Workflow

When reviewing a PR or implementing changes, use this workflow:

```bash
# 1. Index the codebase (first time or after pulling changes)
ziraloop-embeddings init --repos=/workspace/repo

# 2. For each changed function in the diff, find similar functions to check patterns
ziraloop-embeddings similar --file=internal/handler/agents.go --function=Create --limit=5

# 3. Search for related patterns that might need updating
ziraloop-embeddings search --query="agent creation validation" --limit=5

# 4. Check if tests exist for this area
ziraloop-embeddings search --query="test for agent creation" --limit=5

# 5. Find all middleware to verify the new endpoint follows conventions
ziraloop-embeddings search --query="middleware chain setup" --language=go --limit=10
```

---

## Performance

| Operation | Time |
|---|---|
| Full index (4,749 symbols, 218K LOC) | ~21s |
| Incremental re-index (changed files only) | ~2-3s |
| Semantic search (with API call for query embedding) | ~500-800ms |
| Similar function search (no API call, uses stored vector) | ~25ms |
| SQL query | <1ms |
| Drive upload/download | ~1-2s |

---

## Troubleshooting

**"ziraloop-embeddings is disabled: missing required environment variables"**
The org has no OpenAI credential. Add an OpenAI API key to the org's credentials to enable code embeddings.

**"error opening database: create vec table: no such module: vec0"**
The sqlite-vec extension is not loaded. This should not happen with the distributed binary — report as a bug.

**Search returns unexpected results**
The embedding model finds semantic similarity, not exact text matches. For exact text search, use `ziraloop-embeddings sql` with `LIKE` patterns or use grep/codedb instead.

**"drift detection failed, re-indexing fully"**
The stored git commit is not reachable (shallow clone, force push, branch switch). The tool falls back to a full re-index automatically.
