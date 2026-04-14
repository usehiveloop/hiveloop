# Codebase Memory MCP — Tool Index

Complete index of all tools provided by the codebase-memory-mcp server. This MCP server provides graph-based code intelligence using tree-sitter AST parsing, SQLite-backed knowledge graphs, and BFS traversal across 66 supported languages.

---

## Indexing & Project Management

| Name | Description |
|------|-------------|
| `index_repository` | Index a repository into the knowledge graph. Runs a multi-pass pipeline (structure, definitions, calls, HTTP links, config, tests). Supports `full`, `moderate`, and `fast` indexing modes. |
| `list_projects` | List all indexed projects with graph statistics (node/edge counts, size, last modified, status). |
| `delete_project` | Delete a project and all its graph data from the index. |
| `index_status` | Get the indexing status of a project (node/edge counts, ready/empty/no_project). |

## Searching & Querying

| Name | Description |
|------|-------------|
| `search_graph` | Search the knowledge graph for functions, classes, routes, and variables. Uses BM25 full-text search, regex pattern matching, or vector cosine semantic search. Replaces grep/glob for code discovery. |
| `query_graph` | Execute Cypher-like graph queries for complex multi-hop analysis, aggregations, and cross-service patterns. Read-only. |
| `trace_call_path` | Trace function call paths via BFS. Find callers, callees, data flow, or cross-service HTTP calls. Supports depth 1-5 and risk classification. |
| `detect_changes` | Map git diff to affected symbols and compute blast radius. Identifies functions/classes impacted by uncommitted changes with risk classification. |
| `search_code` | Graph-augmented text search. Finds patterns via grep, enriches with graph metadata, deduplicates into containing functions, ranks by structural importance. |

## Code Retrieval

| Name | Description |
|------|-------------|
| `get_code_snippet` | Retrieve source code for a function, class, or symbol by qualified name. Falls back to suffix/short name matching. Returns suggestions if ambiguous. |

## Architecture & Insights

| Name | Description |
|------|-------------|
| `get_architecture` | Get high-level architecture overview: languages, packages, entry points, routes, hotspots, layers, and community clusters. |
| `get_graph_schema` | Get the schema of the knowledge graph — all node labels and edge types with counts. |
| `manage_adr` | Create, read, or update Architecture Decision Records. Persists architectural decisions in `.codebase-memory/adr.md` across sessions. |

## Data Integration

| Name | Description |
|------|-------------|
| `ingest_traces` | Ingest runtime execution traces to validate and enhance HTTP_CALLS edges. Currently a placeholder — runtime edge creation not yet implemented. |

---

## Tool Details

### `index_repository`

Index a repository into the knowledge graph.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo_path` | string | Yes | Absolute path to the repository root |
| `mode` | string | No (default: `"full"`) | `"full"` (all passes + semantic edges), `"moderate"` (fast + SIMILAR_TO + SEMANTICALLY_RELATED), `"fast"` (structure only) |

**Returns:** `{project, status, nodes, edges, adr_present, adr_hint?}`

**Notes:** Serializes pipeline runs to prevent concurrent writes. Suggests ADR creation if `.codebase-memory/adr.md` not found.

---

### `list_projects`

List all indexed projects with graph statistics.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| *(none)* | — | — | — |

**Returns:** `{projects: [{name, nodes, edges, size_mb, modified, status}]}`

---

### `delete_project`

Delete a project and all its graph data.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | Yes | Project name to delete |

**Returns:** `{project, status}` — status is `"deleted"`, `"not_found"`, or `"error"`

---

### `index_status`

Get indexing status of a project.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | Yes | Project name |

**Returns:** `{project, nodes, edges, status}` — status is `"ready"`, `"empty"`, or `"no_project"`

---

### `search_graph`

Search the knowledge graph for symbols. Three independent search modes can be combined in one call.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | Yes | Project name |
| `query` | string | No | BM25 full-text search (tokenized, camelCase-split, structurally boosted). When provided, `name_pattern` is ignored. |
| `label` | string | No | Node type filter: `Function`, `Class`, `Route`, `Interface`, `Method`, `Enum`, `Type`, etc. |
| `name_pattern` | string | No | Regex for exact name matching (e.g. `.*Handler.*`) |
| `qn_pattern` | string | No | Regex for qualified name matching |
| `file_pattern` | string | No | Glob pattern to filter by file |
| `relationship` | string | No | Filter by edge type (e.g. `CALLS`, `IMPORTS`) |
| `min_degree` | integer | No | Minimum node connections (in + out edges) |
| `max_degree` | integer | No | Maximum node connections |
| `exclude_entry_points` | boolean | No (default: false) | Exclude main/init functions |
| `include_connected` | boolean | No (default: false) | Include all nodes connected to results |
| `semantic_query` | array of strings | No | Vector cosine search keywords. Must be an array, not a single string. Requires `moderate` or `full` index. |
| `limit` | integer | No | Max results |
| `offset` | integer | No (default: 0) | Pagination offset |

**Returns:** `{results: [{name, qualified_name, label, file_path, start_line, end_line, properties}], semantic_results?, total, took_ms}`

**BM25 Ranking Boosts:** Functions/Methods +10, Routes +8, Classes/Interfaces +5. Noise labels (File/Folder/Module/Variable) filtered out.

---

### `query_graph`

Execute Cypher-like graph queries.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Cypher query |
| `project` | string | Yes | Project name |
| `max_rows` | integer | No (default: 100k) | Row limit |

**Returns:** `{columns, rows, total}`

**Supported Cypher:** `MATCH` with labels and relationships, variable-length paths (`-[:CALLS*1..3]->`), `WHERE` with comparisons and regex (`=~`), `RETURN` with `COUNT()` and `DISTINCT`, `ORDER BY`, `LIMIT`. Not supported: `WITH`, `COLLECT`, `OPTIONAL MATCH`, mutations.

---

### `trace_call_path`

Trace function call paths via BFS.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `function_name` | string | Yes | Function to trace (short name or qualified) |
| `project` | string | Yes | Project name |
| `direction` | string | No (default: `"both"`) | `"inbound"` (callers), `"outbound"` (callees), `"both"` |
| `depth` | integer | No (default: 3) | BFS depth, 1-5 |
| `mode` | string | No (default: `"calls"`) | `"calls"` (CALLS edges only), `"data_flow"` (+ DATA_FLOWS with arg propagation), `"cross_service"` (+ HTTP_CALLS + ASYNC_CALLS through Route nodes) |
| `parameter_name` | string | No | For `data_flow` mode: scope trace to specific parameter |
| `edge_types` | array of strings | No | Explicit edge types to follow |
| `risk_labels` | boolean | No (default: false) | Add risk classification (CRITICAL/HIGH/MEDIUM/LOW) by hop distance |
| `include_tests` | boolean | No (default: false) | Include test files in results |

**Returns:** `{function, direction, mode?, callers?, callees?}` — each caller/callee has `name, qualified_name, label, file_path, start_line, end_line, is_test?, risk?`

---

### `detect_changes`

Map git diff to affected symbols and compute blast radius.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | Yes | Project name |
| `base_branch` | string | No (default: `"main"`) | Git ref to compare against |
| `scope` | string | No (default: `"symbols"`) | `"files"` (changed files only), `"symbols"` (+ affected functions/classes), `"impact"` (same as symbols) |
| `depth` | integer | No (default: 2) | BFS depth for impact calculation |
| `since` | string | No | Git ref or date to compare from (e.g. `"HEAD~5"`, `"v0.5.0"`, `"2026-01-01"`) |

**Returns:** `{changed_files, changed_count, impacted_symbols: [{name, qualified_name, label, file_path, risk_class}], depth}`

**Risk Classification:** CRITICAL = direct change, HIGH = 1 hop, MEDIUM = 2 hops, LOW = >2 hops

---

### `get_code_snippet`

Retrieve source code for a symbol.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `qualified_name` | string | Yes | Full qualified name or short function name |
| `project` | string | Yes | Project name |
| `include_neighbors` | boolean | No (default: false) | Include calling/called functions in output |

**Returns:** `{status, qualified_name, name, label, file_path, start_line, end_line, source, language, suggestions?}`

**Resolution:** Tier 1: exact qualified name match. Tier 2: suffix match. Returns `suggestions` array if ambiguous.

---

### `search_code`

Graph-augmented text search.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `pattern` | string | Yes | Text or regex pattern |
| `project` | string | Yes | Project name |
| `file_pattern` | string | No | Glob for file filter (e.g. `"*.go"`) |
| `path_filter` | string | No | Regex on file paths (e.g. `^src/`) |
| `regex` | boolean | No (default: false) | Interpret pattern as regex |
| `mode` | string | No (default: `"compact"`) | `"compact"` (signatures + metadata), `"full"` (with source), `"files"` (paths only) |
| `context` | integer | No | Lines of context around match (compact mode only) |
| `limit` | integer | No (default: 10) | Max results |

**Returns:** `{results: [{file_path, line, content?, containing_node?, node_type}], total, search_time_ms}`

**Ranking:** Definitions > popular functions > non-test code > test code > raw matches

---

### `get_architecture`

Get high-level architecture overview.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | Yes | Project name |
| `aspects` | array of strings | No (default: all) | `"all"`, `"structure"`, `"dependencies"`, `"routes"` |

**Returns:** `{project, total_nodes, total_edges, node_labels?, edge_types?, relationship_patterns?}`

---

### `get_graph_schema`

Get knowledge graph schema.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | Yes | Project name |

**Returns:** `{node_labels: [{label, count}], edge_types: [{type, count}], adr_present}`

---

### `manage_adr`

Manage Architecture Decision Records.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | Yes | Project name |
| `mode` | string | No (default: `"get"`) | `"get"` (read), `"update"` (write), `"sections"` (list headers) |
| `content` | string | Required for `"update"` | ADR markdown content |

**Returns:** `{status, content?, sections?, hint?}`

**Storage:** `.codebase-memory/adr.md` in project root

---

### `ingest_traces`

Ingest runtime execution traces (placeholder — not yet implemented).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `traces` | array of objects | Yes | Runtime trace events |
| `project` | string | Yes | Project name |

**Returns:** `{status: "accepted", traces_received, note}`

---

## Graph Data Model

### Node Labels

`Project`, `Package`, `Folder`, `File`, `Module`, `Class`, `Function`, `Method`, `Interface`, `Enum`, `Type`, `Route`, `Resource`

### Edge Types

`CONTAINS_PACKAGE`, `CONTAINS_FOLDER`, `CONTAINS_FILE`, `DEFINES`, `DEFINES_METHOD`, `IMPORTS`, `CALLS`, `HTTP_CALLS`, `ASYNC_CALLS`, `IMPLEMENTS`, `HANDLES`, `USAGE`, `CONFIGURES`, `WRITES`, `MEMBER_OF`, `TESTS`, `USES_TYPE`, `FILE_CHANGES_WITH`

### Qualified Name Format

`<project>.<path_parts>.<name>` (e.g. `myapp.handlers.main.ProcessOrder`)

---

## Common Usage Patterns

1. **Initial exploration:** `index_repository` → `get_architecture` → `search_graph`
2. **Find implementation:** `search_graph` (name_pattern) → `get_code_snippet` (qualified_name from results)
3. **Impact analysis:** `detect_changes` → `trace_call_path` (both directions, risk_labels=true)
4. **Dead code detection:** `search_graph` with `max_degree=0` and `exclude_entry_points=true`
5. **Cross-service mapping:** `query_graph("MATCH (r:Route)<-[:HTTP_CALLS]-(f:Function) RETURN r.name, f.name")`
6. **Call chain tracing:** `trace_call_path` with `mode="calls"` and `depth=5`

---

## Performance

| Operation | Time | Scale |
|-----------|------|-------|
| Full index (Linux kernel, 28M LOC) | ~3 min | 2.1M nodes, 4.9M edges |
| Full index (Django, 49K LOC) | ~6 sec | 49K nodes, 196K edges |
| Cypher query | <1 ms | Relationship traversal |
| Name search (regex) | <10 ms | SQL LIKE pre-filtering |
| Dead code detection | ~150 ms | Full graph scan |
| Call path trace (depth=5) | <10 ms | BFS traversal |
| Incremental re-index (no changes) | <1 ms | No-op detection |
