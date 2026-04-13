# Sandbox Reference

Bridge agents run inside isolated sandbox environments powered by Daytona. Each sandbox is created from a **snapshot** — a pre-built machine image with the agent runtime and developer tools pre-installed.

## Flavors

| Flavor | Snapshot name | Description |
|--------|--------------|-------------|
| `bridge` | `ziraloop-bridge-<version>-<size>` | Minimal runtime: Bridge binary, CodeDB, base system packages. |
| `dev-box` | `zira-dev-box-<size>-v<version>` | Full developer environment: everything in `bridge` plus Node.js, Go, Rust, Python, Chrome, databases, and CLI tools. |

## Sizes

All flavors are available in four resource tiers:

| Size | CPU | Memory | Disk |
|------|-----|--------|------|
| `small` | 1 | 2 GB | 10 GB |
| `medium` | 2 | 4 GB | 20 GB |
| `large` | 4 | 8 GB | 40 GB |
| `xlarge` | 8 | 16 GB | 80 GB |

## Dev-Box Toolchain

The `dev-box` flavor ships the following tools pre-installed. Nothing starts automatically at boot except Bridge and the chrome-devtools-axi daemon. All databases and services are dormant until the agent explicitly starts them.

### Base Runtime

| Tool | Description |
|------|-------------|
| Bridge | Agent runtime (port 8080) |
| CodeDB | Code intelligence for agents |
| curl, wget, git, jq, unzip | Standard system utilities |

### Browser Automation

| Tool | Version | Notes |
|------|---------|-------|
| chrome-devtools-axi | 0.1.15 | AXI CLI for browser automation. Daemon pre-warmed at boot on port 9224. |
| gh-axi | 0.1.11 | AXI CLI for GitHub operations. Replaces `gh` CLI with lower token cost. |
| chrome-headless-shell | Latest stable | Lean headless Chromium (~170 MB). Symlinked to `/opt/google/chrome/chrome`. |

The chrome-devtools-axi daemon starts automatically in the entrypoint. Agents use it via Bash:

```sh
chrome-devtools-axi open https://example.com
chrome-devtools-axi snapshot
chrome-devtools-axi click @1_3
chrome-devtools-axi screenshot /tmp/page.png
chrome-devtools-axi eval "document.title"
```

Run `chrome-devtools-axi --help` for the full command list (34 commands).

**Environment variables:**

| Variable | Value | Purpose |
|----------|-------|---------|
| `CHROME_DEVTOOLS_AXI_DISABLE_HOOKS` | `1` | Disables axi-sdk-js session hook auto-install (not needed under Bridge). |
| `CHROME_DEVTOOLS_AXI_CHROME_ARGS` | `--no-sandbox --disable-dev-shm-usage --disable-gpu` | Required Chrome flags for running as root in a container. Set via entrypoint and `/etc/profile.d/chrome-args.sh`. |

### Node.js

| Tool | Version | Notes |
|------|---------|-------|
| Node.js | LTS (via nvm) | Symlinked to `/usr/local/bin/node`. |
| npm | Bundled with Node | Symlinked to `/usr/local/bin/npm`. |
| npx | Bundled with Node | Symlinked to `/usr/local/bin/npx`. |
| nvm | 0.40.4 | Installed at `/usr/local/nvm`. |

**Version switching:**

```sh
source /usr/local/nvm/nvm.sh
nvm install 22
nvm use 22
```

### Go

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.2 | Installed at `/usr/local/go`. `go` and `gofmt` symlinked to `/usr/local/bin`. |

```sh
go version
go run main.go
```

### Rust

| Tool | Version | Notes |
|------|---------|-------|
| rustc | Stable (latest at build time) | Symlinked to `/usr/local/bin/rustc`. |
| cargo | Stable (latest at build time) | Symlinked to `/usr/local/bin/cargo`. |
| rustup | Latest | Symlinked to `/usr/local/bin/rustup`. Version manager for Rust toolchains. |

**Environment variables:**

| Variable | Value |
|----------|-------|
| `RUSTUP_HOME` | `/usr/local/rustup` |
| `CARGO_HOME` | `/usr/local/cargo` |

**Version switching:**

```sh
rustup install nightly
rustup run nightly rustc --version
rustup default nightly
```

### Python

| Tool | Version | Notes |
|------|---------|-------|
| python3 | 3.12 | System Python from Ubuntu 24.04. |
| pip3 | 24.0 | Python package manager. |
| venv | Bundled | `python3 -m venv /path/to/env` |

### Build Tools

| Tool | Version |
|------|---------|
| gcc | 13.3.0 |
| g++ | 13.3.0 |
| make | 4.3 |
| build-essential | Meta-package (includes gcc, g++, make, libc-dev) |

### Databases

All databases are **dormant at boot**. Start them on demand:

**PostgreSQL 16:**

```sh
pg_ctlcluster 16 main start
psql -U postgres -c 'SELECT version();'
pg_ctlcluster 16 main stop
```

**Redis 7:**

```sh
redis-server --daemonize yes
redis-cli ping
redis-cli shutdown
```

**SQLite 3:**

```sh
sqlite3 mydb.db "CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT);"
```

### Media

| Tool | Version | Notes |
|------|---------|-------|
| ffmpeg | 6.1.1 | Video/audio encoding, format conversion, screen recording. |

### Archive and Compression

| Tool | Notes |
|------|-------|
| zip / unzip | ZIP archives |
| tar | Tape archives |
| gzip | GNU zip compression |
| xz | XZ/LZMA compression |
| zstd | Zstandard compression |
| bzip2 | Bzip2 compression |

### Network and Diagnostics

| Tool | Notes |
|------|-------|
| curl | HTTP client (pre-installed in base) |
| httpie | User-friendly HTTP client (`http` command) |
| dig / nslookup | DNS lookups (from `dnsutils`) |
| netstat / ifconfig | Network diagnostics (from `net-tools`) |
| openssl | TLS/SSL toolkit |

### Data Processing

| Tool | Notes |
|------|-------|
| jq | JSON processor (pre-installed in base) |
| yq | YAML, JSON, and TOML processor |
| xmllint | XML validation and formatting (from `libxml2-utils`) |
| xmlstarlet | XML querying and transformation |

### Code Intelligence

The dev-box ships three tools that together enable Greptile-level code review: a structural code graph, AST parsing for function-granularity chunking, and vector storage for semantic search over LLM-generated docstrings.

| Tool | Notes |
|------|-------|
| codebase-memory-mcp | Builds a code graph (symbols, calls, refs, deps) from tree-sitter ASTs. Queryable via MCP tools. Single static binary. |
| tree-sitter CLI | AST parsing across 100+ languages. Agents use it for function-granularity chunking before generating docstring embeddings. |
| pgvector | PostgreSQL extension for vector similarity search. Stores docstring embeddings for semantic code retrieval. |

**Typical agent workflow for code review:**

```sh
# 1. Index the repo (structural graph — runs once after clone)
codebase-memory-mcp index /path/to/repo

# 2. Query the graph
codebase-memory-mcp query "trace_call_path(function_name='handleRequest', direction='inbound')"
codebase-memory-mcp query "search_graph(query='authentication')"

# 3. Parse AST for function-granularity chunks
tree-sitter parse src/auth/service.ts

# 4. Store docstring embeddings in pgvector (agent generates docstrings via LLM)
pg_ctlcluster 16 main start
psql -U postgres -c "CREATE EXTENSION IF NOT EXISTS vector;"
# Agent inserts embeddings and queries with vector similarity search
```

### Terminal and Editors

| Tool | Notes |
|------|-------|
| tmux | Terminal multiplexer |
| screen | Terminal multiplexer (alternative) |
| nano | Text editor |

## Disk Budget

Approximate disk usage of the dev-box toolchain (medium template, 20 GB total):

| Component | Size |
|-----------|------|
| Rust (stable toolchain) | ~2.7 GB |
| Node.js + nvm | ~490 MB |
| Go | ~286 MB |
| chrome-headless-shell | ~257 MB |
| Ubuntu base + apt packages | ~1.8 GB |
| **Total** | **~5.5 GB** |
| **Free for workspace** | **~14.5 GB** |

## Building Templates

```sh
# Build all dev-box sizes
make build-templates VERSION=0.18.0 FLAVOR=dev-box

# Build a specific size
make build-templates VERSION=0.18.0 FLAVOR=dev-box SIZE=medium

# Build the minimal bridge flavor
make build-templates VERSION=0.18.0
```
