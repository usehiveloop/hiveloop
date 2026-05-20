# Repository instructions

These instructions are for coding agents. Follow them strictly.

HiveLoop is a Go-first platform with a Go API/worker, Next.js web app, Rust sandbox runtimes, local integration simulators, and Docker/native infrastructure.

Use codebase-memory MCP graph tools before shell search when available: `search_graph`, `trace_path`, `get_code_snippet`, `query_graph`, `get_architecture`. Fall back to `rg`, `find`, and direct reads only when graph tools are unavailable or when searching docs, configs, logs, generated files, env values, or string literals.

## List of services and what they do

- `cmd/server`: main Go binary for API, worker, MCP, webhooks, proxying, streaming, and auth.
- `cmd/server serve`: HTTP API process for handlers, middleware, routes, MCP server, and API background subscribers.
- `cmd/server work`: Asynq worker for email, billing, audit/generation writes, sandbox lifecycle, RAG jobs, trigger dispatch, and cleanup.
- `cmd/server both`: combined API and worker mode for deployments that run both in one process.
- `apps/web`: primary Next.js customer app and docs surface. Calls the backend through `/api/proxy`.
- `cmd/fake-nango`: local Nango-compatible OAuth/integration simulator.
- Postgres: primary datastore. GORM models live in `internal/model`; migration is `model.AutoMigrate`.
- Redis: cache, rate-limit, streaming, and Asynq queue backend.
- Mailpit: local email capture for Docker-based flows.
- MinIO: local S3-compatible storage for RAG, drive, uploads, and tests.
- Qdrant: vector database for RAG ingestion/search.
- Hindsight: optional agent memory service.
- `sandboxes/runtime`: Rust bridge/runtime workspace.
- `sandboxes/employee`: Rust employee sandbox runtime workspace.
- `global-skills`: global skill definitions seeded during backend bootstrap.

Local ports: fake-nango `13004`, backend API `18080`, frontend `31112`, Redis `6379`, Postgres discovered at `/tmp/agent-test/pg.port` with many tests defaulting to `5433`.

## Set up and run local repository

The repository setup depends on your environment.

### Sandbox instructions

This environment gives you full control over the machine you are running on. 

You cannot run docker commands and containers in this environment as you are most likely already running inside a docker container.

Use only `make local-up`. Do not use `make dev`. It is not approved for agents.

`make local-up` is required because it starts the real local stack: Postgres, Redis, fake-nango, Go backend, `apps/web`, env/log/pid files under `/tmp/agent-test`, and seeded test data via `make seed-test`.

Setup is incomplete without seeded test data. `make local-up` runs `make seed-test`; if the database was reset manually, run `make seed-test`.

Approved associated commands:

- `make local-status`: checks fake-nango, backend, frontend, and supervisor PIDs.
- `make local-down`: stops the stack and clears port holders for `13004`, `18080`, and `31112`.
- `make local-reset`: stops and restarts the local stack.
- `make login-test`: creates a browser login session for `agent-test@example.com` using the OTP from `/tmp/agent-test/backend.log`.
- `tail -f /tmp/agent-test/*.log`: watches local stack logs.

### Developer's machine

This is the developer's local machine. You want to run the docker compose containers using `make up`
- Run local frontend application using `pnpm dev` in `apps/web` folder.

## Local development

## Run tests

Testing is mandatory. Every final response and every PR must list exact commands run, pass/fail results, and evidence. If a command cannot run, state the blocker and remaining risk.

Start with the narrowest meaningful test, then run broader coverage based on blast radius. Do not claim completion without test evidence.

Backend commands: `go test ./internal/... -count=1`, `go test ./internal/... -race -count=1`, `go test ./e2e/... -count=1 -timeout=5m`, `make test`, `make test-e2e`, `make check`.

Use targeted tests while iterating: `go test ./internal/handler -count=1 -run 'TestName'`, `go test ./internal/tasks -count=1 -run 'TestName'`, `go test ./internal/rag/... -count=1`.

Integration tests often require Postgres/Redis and commonly assume `postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable`.

Docker-backed test infrastructure: `make test-setup`.

Known backend suites: `make test-auth`, `make test-nango`, `make test-proxy`, `make test-connect`, `make test-connections`, `make test-integrations`.

Clean-slate suites: `make test-clean`, `make test-clean-auth`, `make test-clean-nango`, `make test-clean-proxy`, `make test-clean-connect`, `make test-clean-integrations`.

RAG infrastructure: `make test-services-up`, `make test-services-down`.

Frontend checks are mandatory for frontend changes: `cd apps/web && pnpm typecheck && pnpm build`.

Rust sandbox checks are mandatory when sandbox runtime files change: `make sandbox-runtime-test`, `make sandbox-runtime-fmt-check`, `make sandbox-runtime-clippy`, `make sandbox-employee-test`, `make sandbox-employee-fmt-check`.

## Linting and absolute code quality rules

These rules are absolute unless the task you are assigned explicitly overrides them.

- New hand-written Go files must stay under 300 lines. Run `./scripts/check-go-file-length.sh`.
- Added Go comments must be rare and high-signal. Run `./scripts/check-go-comment-density.sh` on PR branches.
- New log emit sites must stay within budget. Run `./scripts/check-log-budget.sh` before adding logs broadly.
- Do not use stdlib `log` or `fmt.Print*` for application logging. Use `log/slog`, `logging.FromContext`, or `logging.Capture`.
- Do not add real credentials, tokens, API keys, private keys, or production DSNs.
- Do not hand-edit generated files. Fix the source and regenerate.
- Do not commit generated binaries, local test binaries, logs, `.env` files, or `/tmp/agent-test` artifacts.
- Keep `context.Context` threaded through request, I/O, database, Redis, worker, and outbound HTTP paths.
- Preserve org scoping and auth checks on every customer/admin data path.
- Keep handlers thin. Put reusable domain behavior in the relevant `internal/*` package.
- Use typed request/response structs and existing helpers instead of ad hoc maps or string parsing.
- For encrypted credentials, use `internal/credentials`, `internal/crypto`, KMS, and cache invalidation paths.
- For background work, define payload constructors, set queue/retry/timeout intentionally, and register handlers in `internal/tasks/registry.go`.

When public backend API contracts change, all regeneration is required: `make openapi`, `cd apps/web && pnpm generate`, `cd packages/sdk && npm run generate`.

When bridge OpenAPI contracts change, run the relevant command: `make generate-bridge-client` or `make generate-employee-bridge-client`.

## Creating pull requests

PRs must include complete testing evidence. A PR without evidence is not ready.

Every PR summary must include: files/areas changed, exact test commands, pass/fail result for each command, manual verification evidence, untested risk, and generated artifacts updated.

Do not write vague claims like “tested locally” or “should work”. Include commands, payloads, screenshots, videos, or logs that prove the change.

### Frontend focused pull requests guide

Frontend PRs require automated and visual evidence.

Required automated evidence: `cd apps/web && pnpm typecheck && pnpm build`. Run only the app-specific command if the PR truly touches one app, and state that scope.

Required visual evidence:

- Use `agent-browser` to open the affected local page or flow.
- Capture screenshots or videos of the changed UI in the running app.
- Include desktop and mobile/responsive evidence when layout can change by viewport.
- Exercise loading, empty, error, and success states when the PR changes those states.

Frontend agents must reuse existing UI components, API hooks, auth/session helpers, and app shells. Do not create parallel component systems or bypass `/api/proxy` for authenticated app calls.

### Backend focused pull requests guide

Backend PRs require real local backend verification.

Required evidence:

- Run narrow targeted Go tests for the changed package.
- Run broader tests based on blast radius.
- Start the local stack with `make local-up` when validating API behavior.
- Send a real HTTP payload to the changed local endpoint or flow and include request shape plus response/status evidence.

If public API contracts change, run `make openapi`, `cd apps/web && pnpm generate`, and `cd packages/sdk && npm run generate`.

Worker changes require tested evidence for task constructor, queue options, handler registration, and handler behavior.

Database changes require migration/model/query tests and org-scope verification.

Auth, credentials, billing, RAG, webhooks, triggers, and sandbox orchestration changes require at least one realistic payload or fixture.

Sandbox-backed behavior that requires remote Daytona or production-like sandbox infrastructure may be marked untestable locally only after all local unit, handler, payload, and orchestration tests that can run have been run and reported.
