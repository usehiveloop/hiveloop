---
name: codebase-memory
description: Code knowledge graph for structural queries via CLI. Use when you need to explore the codebase, understand architecture, find functions by name, trace call chains, find callers/callees, detect dead code, analyze impact of changes, find high fan-out functions, or run graph queries. Triggers on "who calls this function", "what does X call", "trace the call chain", "find callers of", "show dependencies", "impact analysis", "dead code", "unused functions", "explore the codebase", "understand the architecture", "what functions exist", "show me the structure".
---

# Codebase Memory — CLI Usage Guide

Use `codebase-memory-mcp cli <tool> '<json>'` to query the code knowledge graph. Graph queries return precise structural results in ~500 tokens vs ~80K for grep. Always prefer graph tools for code discovery.

Binary location: `~/.local/bin/codebase-memory-mcp`

---

## Quick Decision Matrix

| I want to... | Command |
|--------------|---------|
| Check if a project is indexed | `codebase-memory-mcp cli list_projects '{}'` |
| Index a new codebase | `codebase-memory-mcp cli index_repository '{"repo_path":"/absolute/path"}'` |
| Find a function by name | `codebase-memory-mcp cli search_graph '{"project":"PROJECT","name_pattern":".*FuncName.*","label":"Function"}'` |
| Search by keyword | `codebase-memory-mcp cli search_graph '{"project":"PROJECT","query":"authentication middleware"}'` |
| Read a function's source code | `codebase-memory-mcp cli get_code_snippet '{"project":"PROJECT","qualified_name":"project.path.FuncName"}'` |
| Who calls this function? | `codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"FuncName","direction":"inbound"}'` |
| What does this function call? | `codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"FuncName","direction":"outbound"}'` |
| Find all HTTP routes | `codebase-memory-mcp cli search_graph '{"project":"PROJECT","label":"Route"}'` |
| Find all classes | `codebase-memory-mcp cli search_graph '{"project":"PROJECT","label":"Class"}'` |
| Impact of my changes | `codebase-memory-mcp cli detect_changes '{"project":"PROJECT","scope":"symbols"}'` |
| Get architecture overview | `codebase-memory-mcp cli get_architecture '{"project":"PROJECT"}'` |
| Find dead code | `codebase-memory-mcp cli search_graph '{"project":"PROJECT","max_degree":0,"exclude_entry_points":true}'` |
| Custom graph query | `codebase-memory-mcp cli query_graph '{"project":"PROJECT","query":"MATCH (f:Function)-[:CALLS]->(g) WHERE f.name = '\''main'\'' RETURN g.name"}'` |
| Text search with graph context | `codebase-memory-mcp cli search_code '{"project":"PROJECT","pattern":"MapProviderToGroup","file_pattern":"*.go"}'` |

---

## Step 1: Check What's Indexed

```bash
codebase-memory-mcp cli list_projects '{}'
```

Returns all indexed projects with node/edge counts. The project name is derived from the repo path (e.g. `/Users/dev/code/myapp` becomes `Users-dev-code-myapp`).

**Real example:**
```bash
$ codebase-memory-mcp cli list_projects '{}'
# Returns: Users-bahdcoder-code-ziraloop.com — 40,200 nodes, 60,090 edges
```

---

## Step 2: Index a Repository (if needed)

```bash
codebase-memory-mcp cli index_repository '{"repo_path":"/absolute/path/to/repo","mode":"full"}'
```

Modes:
- `"full"` — all passes including semantic edges (recommended for first index)
- `"moderate"` — fast discovery + similarity edges
- `"fast"` — structure only (definitions and hierarchy)

**Real example:**
```bash
codebase-memory-mcp cli index_repository '{"repo_path":"/Users/bahdcoder/code/ziraloop.com","mode":"full"}'
```

---

## Step 3: Understand the Graph Structure

```bash
codebase-memory-mcp cli get_graph_schema '{"project":"PROJECT"}'
```

Shows all node labels (Function, Class, Route, Interface, etc.) and edge types (CALLS, IMPORTS, HTTP_CALLS, etc.) with counts. Run this first to understand what's available.

**Real example (ziraloop.com):**
```
Node labels: Function(2549), Class(677), Route(482), Method(1029), Interface(181), Type(196)
Edge types: CALLS(11376), IMPORTS(381), HTTP_CALLS(148), DEFINES(38127), TESTS(2556)
```

---

## Searching for Code

### BM25 keyword search
```bash
codebase-memory-mcp cli search_graph '{"project":"PROJECT","query":"agent system prompt","limit":5}'
```
Tokenized full-text search. Boosts Functions/Methods (+10), Routes (+8), Classes (+5). Best for natural language queries.

**Real example:**
```bash
codebase-memory-mcp cli search_graph '{"project":"Users-bahdcoder-code-ziraloop.com","query":"agent system prompt","limit":5}'
# Returns:
#   Function  StepSystemPrompt             apps/web/.../step-system-prompt.tsx
#   Function  NewSystemAgentSyncHandler    internal/tasks/system_agent.go
#   Function  seedAgent                    internal/system-agents/seeder.go
```

### Regex name pattern search
```bash
codebase-memory-mcp cli search_graph '{"project":"PROJECT","name_pattern":".*Handler.*","label":"Function","limit":10}'
```
Exact regex matching on symbol names. Combine with `label` to filter by type.

**Real example:**
```bash
codebase-memory-mcp cli search_graph '{"project":"Users-bahdcoder-code-ziraloop.com","name_pattern":".*Handler.*","label":"Function","limit":5}'
# Returns:
#   NewAPIKeyHandler      internal/handler/api_keys.go
#   NewActionsHandler     internal/handler/actions.go
#   NewAdminHandler       internal/handler/admin.go
#   NewAgentHandler       internal/handler/agents.go
```

### Find classes matching a pattern
```bash
codebase-memory-mcp cli search_graph '{"project":"PROJECT","name_pattern":".*Agent.*","label":"Class","limit":10}'
```

**Real example:**
```bash
codebase-memory-mcp cli search_graph '{"project":"Users-bahdcoder-code-ziraloop.com","name_pattern":".*Agent.*","label":"Class","limit":10}'
# Returns:
#   Agent                 internal/model/agent.go
#   EnrichmentAgent       internal/trigger/enrichment/agent.go
#   AgentHandler          internal/handler/agents.go
#   AgentTrigger          internal/model/agent_trigger.go
```

### Graph-augmented text search
```bash
codebase-memory-mcp cli search_code '{"project":"PROJECT","pattern":"MapProviderToGroup","file_pattern":"*.go","mode":"compact","limit":5}'
```
Runs grep then enriches with graph metadata. Deduplicates matches into containing functions. Ranks by structural importance.

**Real example:**
```bash
codebase-memory-mcp cli search_code '{"project":"Users-bahdcoder-code-ziraloop.com","pattern":"MapProviderToGroup","file_pattern":"*.go","mode":"compact","limit":5}'
# Returns:
#   MapProviderToGroup (Function, in_degree=6)  internal/system-agents/seeder.go:104-121
#   run (Method, in_degree=1, out_degree=18)    internal/forge/controller.go:522-833
#   SetupContextGathering (Method)               internal/forge/controller.go
```

---

## Reading Source Code

### Get a function's source
```bash
codebase-memory-mcp cli get_code_snippet '{"project":"PROJECT","qualified_name":"FunctionName"}'
```
Use short names for quick lookup. If ambiguous, returns suggestions with full qualified names.

**Real example:**
```bash
codebase-memory-mcp cli get_code_snippet '{"project":"Users-bahdcoder-code-ziraloop.com","qualified_name":"ResolveProviderConfig"}'
# Returns:
#   File: internal/model/provider_prompts.go
#   Lines: 21-52
#   Source: func (agent *Agent) ResolveProviderConfig(providerGroup string) ...
```

**Workflow: search then read:**
```bash
# 1. Find the function
codebase-memory-mcp cli search_graph '{"project":"PROJECT","name_pattern":".*Seed.*","label":"Function"}'
# 2. Use the qualified_name from results
codebase-memory-mcp cli get_code_snippet '{"project":"PROJECT","qualified_name":"Users-bahdcoder-code-ziraloop.com.internal.sub-agents.seeder.Seed"}'
```

---

## Tracing Call Chains

### Who calls this function? (inbound)
```bash
codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"seedSubagent","direction":"inbound","depth":2}'
```

**Real example:**
```bash
codebase-memory-mcp cli trace_path '{"project":"Users-bahdcoder-code-ziraloop.com","function_name":"seedSubagent","direction":"inbound","depth":2}'
# Returns:
#   callers: Seed
```

### What does this function call? (outbound)
```bash
codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"Seed","direction":"outbound","depth":2}'
```

**Real example:**
```bash
codebase-memory-mcp cli trace_path '{"project":"Users-bahdcoder-code-ziraloop.com","function_name":"Seed","direction":"outbound","depth":2}'
# Returns:
#   callees: seedAgent, Exec, Debug, ForgeAgentConfig
```

### Full call context (both directions)
```bash
codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"FuncName","direction":"both","depth":3}'
```

### Risk-labeled trace (for impact analysis)
```bash
codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"FuncName","direction":"both","depth":3,"risk_labels":true}'
```
Adds CRITICAL/HIGH/MEDIUM/LOW risk classification by hop distance.

---

## Impact Analysis

### What symbols are affected by my uncommitted changes?
```bash
codebase-memory-mcp cli detect_changes '{"project":"PROJECT","scope":"symbols","depth":2}'
```

**Real example:**
```bash
codebase-memory-mcp cli detect_changes '{"project":"Users-bahdcoder-code-ziraloop.com","scope":"symbols","depth":1}'
# Returns:
#   Changed files: 42
#   Impacted symbols: CreateAgentFormValues, CreateAgentProvider, StepRouter, ...
```

### Compare against a specific branch or tag
```bash
codebase-memory-mcp cli detect_changes '{"project":"PROJECT","base_branch":"main","scope":"symbols","depth":2}'
codebase-memory-mcp cli detect_changes '{"project":"PROJECT","since":"v0.5.0","scope":"symbols"}'
codebase-memory-mcp cli detect_changes '{"project":"PROJECT","since":"HEAD~10","scope":"symbols"}'
```

---

## Custom Cypher Queries

```bash
codebase-memory-mcp cli query_graph '{"project":"PROJECT","query":"CYPHER_QUERY"}'
```

### Find all classes with "Agent" in the name
```bash
codebase-memory-mcp cli query_graph '{"project":"Users-bahdcoder-code-ziraloop.com","query":"MATCH (c:Class) WHERE c.name =~ '\''.*Agent.*'\'' RETURN c.name, c.file_path LIMIT 10"}'
# Returns:
#   Agent                    internal/model/agent.go
#   EnrichmentAgent          internal/trigger/enrichment/agent.go
#   AgentHandler             internal/handler/agents.go
```

### Find functions that call a specific function
```bash
codebase-memory-mcp cli query_graph '{"project":"PROJECT","query":"MATCH (f:Function)-[:CALLS]->(g:Function) WHERE g.name = '\''Seed'\'' RETURN f.name, f.file_path LIMIT 10"}'
```

### Find HTTP route handlers
```bash
codebase-memory-mcp cli query_graph '{"project":"PROJECT","query":"MATCH (r:Route)<-[:HANDLES]-(h:Function) RETURN r.name, h.name, h.file_path LIMIT 20"}'
```

### Find cross-service HTTP calls
```bash
codebase-memory-mcp cli query_graph '{"project":"PROJECT","query":"MATCH (a)-[r:HTTP_CALLS]->(b) RETURN a.name, b.name LIMIT 20"}'
```

### Count functions per file
```bash
codebase-memory-mcp cli query_graph '{"project":"PROJECT","query":"MATCH (f:File)-[:DEFINES]->(fn:Function) RETURN f.name, COUNT(fn) ORDER BY COUNT(fn) DESC LIMIT 10"}'
```

**Supported Cypher:** `MATCH`, `WHERE` (comparisons, regex `=~`, `CONTAINS`), `RETURN` (`COUNT()`, `DISTINCT`), `ORDER BY`, `LIMIT`, variable-length paths (`-[:CALLS*1..3]->`). Not supported: `WITH`, `COLLECT`, `OPTIONAL MATCH`, mutations.

---

## Architecture Overview

```bash
codebase-memory-mcp cli get_architecture '{"project":"PROJECT"}'
```

Returns high-level overview: total nodes/edges, node label distribution, edge type distribution, REST route patterns. Use as the first exploration step on a new codebase.

---

## Architecture Decision Records

### Read existing ADR
```bash
codebase-memory-mcp cli manage_adr '{"project":"PROJECT","mode":"get"}'
```

### Create/update ADR
```bash
codebase-memory-mcp cli manage_adr '{"project":"PROJECT","mode":"update","content":"# Architecture Decisions\n\n## Decision 1: ..."}'
```

Stored in `.codebase-memory/adr.md` in the project root. Persists across sessions.

---

## Standard Workflows

### Workflow 1: Explore a new codebase
```bash
# 1. Index it
codebase-memory-mcp cli index_repository '{"repo_path":"/path/to/repo","mode":"full"}'
# 2. Get the project name
codebase-memory-mcp cli list_projects '{}'
# 3. Understand the structure
codebase-memory-mcp cli get_architecture '{"project":"PROJECT"}'
codebase-memory-mcp cli get_graph_schema '{"project":"PROJECT"}'
# 4. Find key entry points
codebase-memory-mcp cli search_graph '{"project":"PROJECT","label":"Route","limit":20}'
codebase-memory-mcp cli search_graph '{"project":"PROJECT","name_pattern":".*main.*","label":"Function"}'
```

### Workflow 2: Understand a feature
```bash
# 1. Find the relevant function
codebase-memory-mcp cli search_graph '{"project":"PROJECT","query":"user authentication","limit":5}'
# 2. Read its source
codebase-memory-mcp cli get_code_snippet '{"project":"PROJECT","qualified_name":"FunctionName"}'
# 3. Trace what it calls and who calls it
codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"FunctionName","direction":"both","depth":3}'
```

### Workflow 3: Before making changes (impact analysis)
```bash
# 1. Check what's affected by current changes
codebase-memory-mcp cli detect_changes '{"project":"PROJECT","scope":"symbols","depth":2}'
# 2. Trace the blast radius of a specific function
codebase-memory-mcp cli trace_path '{"project":"PROJECT","function_name":"FunctionToChange","direction":"inbound","depth":3,"risk_labels":true}'
```

### Workflow 4: Find dead code
```bash
codebase-memory-mcp cli search_graph '{"project":"PROJECT","max_degree":0,"exclude_entry_points":true,"label":"Function","limit":20}'
```

---

## Node Labels

`Project`, `Package`, `Folder`, `File`, `Module`, `Class`, `Function`, `Method`, `Interface`, `Enum`, `Type`, `Route`, `Resource`, `Section`, `Variable`

## Edge Types

`CALLS`, `HTTP_CALLS`, `ASYNC_CALLS`, `IMPORTS`, `DEFINES`, `DEFINES_METHOD`, `HANDLES`, `IMPLEMENTS`, `USAGE`, `CONFIGURES`, `WRITES`, `MEMBER_OF`, `TESTS`, `TESTS_FILE`, `USES_TYPE`, `FILE_CHANGES_WITH`, `CONTAINS_FILE`, `CONTAINS_FOLDER`, `CONTAINS_PACKAGE`, `SIMILAR_TO`, `SEMANTICALLY_RELATED`, `RAISES`

---

## Gotchas

1. **Project names use dashes, not slashes.** `/Users/dev/code/myapp` becomes `Users-dev-code-myapp`.
2. **Use `search_graph` before `get_code_snippet`.** You need the exact qualified name from search results to read source reliably.
3. **Use `trace_path` before `get_code_snippet`.** Trace call chains first, then read the specific functions you care about.
4. **The CLI tool is `trace_path`**, not `trace_call_path`. The MCP docs use a different name.
5. **Single quotes in Cypher need escaping.** Use `'\''` to escape single quotes inside bash single-quoted strings.
6. **Default limit is small.** Always pass `"limit":N` to get enough results.
7. **`search_graph` with `relationship` filters by node degree**, not by actual edge traversal. Use `query_graph` with Cypher for edge-level queries.
8. **Re-index after major changes.** The graph reflects the state at index time. Run `index_repository` again after large refactors.
