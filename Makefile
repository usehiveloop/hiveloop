.PHONY: build test test-e2e lint check-file-length vet check up down dev clean fetch-actions generate docker-build docker-run test-clean test-clean-auth test-clean-nango test-clean-proxy test-clean-connect test-clean-integrations test-auth test-nango test-proxy test-connect test-integrations test-connections test-setup openapi generate-auth-keys upload-skills build-employee-sandbox-templates employee-env-doctor employee-debug-pack test-services-up test-services-down ragtest-slack-live ragtest-kb-search-live seed-test local-up local-down local-reset local-status login-test asynq-peek

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
IMAGE   ?= usehiveloop/hiveloop

# Generate base64-encoded RSA private key for AUTH_RSA_PRIVATE_KEY env var
generate-auth-keys:
	@openssl genrsa 2048 2>/dev/null | base64 | tr -d '\n' && echo

# Generate provider action files from API specs (OpenAPI 3.x, OpenAPI 2.0, GraphQL)
fetch-actions: fetch-actions-oas3 fetch-actions-oas2 fetch-actions-graphql

fetch-actions-oas3:
	go run ./cmd/fetchactions-oas3

fetch-actions-oas2:
	go run ./cmd/fetchactions-oas2

fetch-actions-graphql:
	go run ./cmd/fetchactions-graphql

# Regenerate OpenAPI spec from handler annotations (Swagger 2.0 → OpenAPI 3.0, clean schema names)
openapi:
	swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
	npx swagger2openapi docs/swagger.json -o docs/openapi.json
	@python3 -c "\
	import json, re; \
	d = json.load(open('docs/openapi.json')); \
	raw = json.dumps(d); \
	raw = raw.replace('internal_handler.', ''); \
	raw = raw.replace('internal_handler_', ''); \
	raw = raw.replace('github_com_usehiveloop_hiveloop_internal_registry.', ''); \
	raw = raw.replace('github_com_usehiveloop_hiveloop_internal_model.', ''); \
	raw = raw.replace('github_com_usehiveloop_hiveloop_internal_mcp_catalog.', ''); \
	json.dump(json.loads(raw), open('docs/openapi.json','w'), indent=2) \
	"
	@echo "✓ docs/openapi.json updated"

# Build + push base sandbox images to GHCR and register Daytona snapshots
# (one per size: small, medium, large, xlarge) pointing at the GHCR image.
# Requires GHCR_USERNAME, GHCR_PAT (PAT with write:packages),
# SANDBOX_PROVIDER_KEY, SANDBOX_PROVIDER_URL, SANDBOX_TARGET.
# Usage: make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0
#        make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0 SIZE=small
# VERSION drives the GHCR image tag and Daytona snapshot name. Bump it on every
# rebuild — Daytona freezes the snapshot's mirrored image at create time, so
# reusing a VERSION leaves the old bytes in the control-plane registry.
# BRIDGE_VERSION is the usehiveloop/bridge release tag installed into the image
# and is independent of VERSION; keep it pinned and bump VERSION when you only
# need to rebuild the surrounding image.
build-templates:
	@test -n "$(VERSION)" || (echo "error: VERSION is required (e.g. make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0)" && exit 1)
	@test -n "$(BRIDGE_VERSION)" || (echo "error: BRIDGE_VERSION is required (e.g. make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0)" && exit 1)
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) go run ./cmd/buildtemplates bridge -version=$(VERSION) -bridge-version=$(BRIDGE_VERSION) -size=$(or $(SIZE),all)

# Register Daytona snapshots from a usehiveloop/hermes image already published
# to GHCR by the hermes repo's CI. No image build happens here — the snapshots
# point at ghcr.io/usehiveloop/hermes:<HERMES_VERSION>.
# Requires SANDBOX_PROVIDER_KEY, SANDBOX_PROVIDER_URL, SANDBOX_TARGET.
# Usage: make build-hermes-templates HERMES_VERSION=v0.0.1
#        make build-hermes-templates HERMES_VERSION=v0.0.1 SIZE=small
build-hermes-templates:
	@test -n "$(HERMES_VERSION)" || (echo "error: HERMES_VERSION is required (e.g. make build-hermes-templates HERMES_VERSION=v0.0.1)" && exit 1)
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) go run ./cmd/buildtemplates hermes -version=$(HERMES_VERSION) -size=$(or $(SIZE),all)

# Register Daytona snapshots from a usehiveloop/employee-sandbox image already published.
# Requires SANDBOX_PROVIDER_KEY, SANDBOX_PROVIDER_URL, SANDBOX_TARGET.
# Usage: make build-employee-sandbox-templates EMPLOYEE_SANDBOX_VERSION=v0.0.1
#        make build-employee-sandbox-templates EMPLOYEE_SANDBOX_VERSION=v0.0.1 SIZE=small
build-employee-sandbox-templates:
	@test -n "$(EMPLOYEE_SANDBOX_VERSION)" || (echo "error: EMPLOYEE_SANDBOX_VERSION is required (e.g. make build-employee-sandbox-templates EMPLOYEE_SANDBOX_VERSION=v0.0.1)" && exit 1)
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) go run ./cmd/buildtemplates employee-sandbox -version=$(EMPLOYEE_SANDBOX_VERSION) -size=$(or $(SIZE),all)

# Inspect a running employee sandbox's process env using the redacted doctor.
# Usage: make employee-env-doctor SANDBOX_ID=48a54bb8-cd44-4454-845d-3be611f9090b
#        make employee-env-doctor SANDBOX_ID=... DOCTOR_SENSITIVE=1
DOCTOR_JSON ?= false
DOCTOR_INCLUDE_UNEXPECTED ?= 1
DOCTOR_SENSITIVE ?= 0
DOCTOR_ENV_FILE ?= .env
DOCTOR_PID ?=
employee-env-doctor:
	@test -n "$(SANDBOX_ID)" || (echo "error: SANDBOX_ID is required (e.g. make employee-env-doctor SANDBOX_ID=48a54bb8-cd44-4454-845d-3be611f9090b)" && exit 1)
	@flags="-id $(SANDBOX_ID) -json=$(DOCTOR_JSON) -env-file=$(DOCTOR_ENV_FILE)"; \
	if [ -n "$(DOCTOR_PID)" ]; then flags="$$flags -pid $(DOCTOR_PID)"; fi; \
	if [ "$(DOCTOR_INCLUDE_UNEXPECTED)" = "1" ]; then flags="$$flags -include-unexpected"; fi; \
	if [ "$(DOCTOR_SENSITIVE)" = "1" ]; then flags="$$flags --sensitive"; fi; \
	go run ./cmd/employee-env-doctor $$flags

# Upload a debug collector to an employee sandbox, run it, download the archive,
# extract it locally, and print absolute paths to extracted files.
# Usage: make employee-debug-pack SANDBOX_ID=48a54bb8-cd44-4454-845d-3be611f9090b
#        make employee-debug-pack SANDBOX_ID=... DEBUG_SENSITIVE=1
DEBUG_ENV_FILE ?= .env
DEBUG_LOCAL_DIR ?= /tmp
DEBUG_SENSITIVE ?= 0
DEBUG_TIMEOUT ?= 10m
employee-debug-pack:
	@test -n "$(SANDBOX_ID)" || (echo "error: SANDBOX_ID is required (e.g. make employee-debug-pack SANDBOX_ID=48a54bb8-cd44-4454-845d-3be611f9090b)" && exit 1)
	@flags="-id $(SANDBOX_ID) -env-file=$(DEBUG_ENV_FILE) -local-dir=$(DEBUG_LOCAL_DIR) -timeout=$(DEBUG_TIMEOUT)"; \
	if [ "$(DEBUG_SENSITIVE)" = "1" ]; then flags="$$flags --sensitive"; fi; \
	go run ./cmd/employee-debug-pack $$flags

# Upload skill definitions to Hiveloop API (reads HIVELOOP_SKILLS_API_KEY from .env)
upload-skills:
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) go run ./global-skills/upload

# Generate Bridge Go client from OpenAPI spec.
# Bridge emits OpenAPI 3.1 schemas oapi-codegen can't handle:
#   1. {oneOf: [{type:null}, {$ref}]} for nullable refs → collapse to the $ref
#   2. {type: ["integer", "null"]} array-form types → strip "null", keep scalar
generate-bridge-client:
	jq 'walk( \
		if type == "object" and has("oneOf") and (.oneOf | type == "array") and (.oneOf | length == 2) and (.oneOf | any(. == {"type":"null"})) then \
			(.oneOf | map(select(. != {"type":"null"}))[0]) \
		elif type == "object" and has("type") and (.type | type == "array") then \
			.type |= (map(select(. != "null")) | if length == 1 then .[0] else . end) \
		else . end)' \
		openapi/bridge.json > openapi/bridge.generated.json
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		--config=internal/bridge/oapi-codegen.yaml openapi/bridge.generated.json
	rm openapi/bridge.generated.json

# Generate all embedded assets. Note: the model registry is hand-curated in
# internal/registry/models.go and is NOT a generate target — additions go
# through code review, not regeneration.
generate: fetch-actions

# Build the binary
build:
	go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" \
		-o bin/hiveloop ./cmd/server

# Run unit tests with race detection
test:
	go test ./internal/... -v -race -count=1

# Run e2e tests (requires docker-compose stack running)
test-e2e:
	go test ./e2e/... -v -count=1 -timeout=5m

# Start services and wait for healthy (no teardown, no tests)
test-setup:
	docker compose up -d postgres redis minio qdrant
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U hiveloop -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@until curl -fsS http://localhost:9000/minio/health/ready >/dev/null 2>&1; do sleep 1; done
	@echo "  ✓ MinIO"
	@until curl -fsS http://localhost:$${QDRANT_HTTP_PORT:-6333}/readyz >/dev/null 2>&1; do sleep 1; done
	@echo "  ✓ Qdrant"
	docker compose run --rm minio-setup
	@echo ""
	@echo "  Infrastructure ready. Run tests with:"
	@echo "    make test-auth"
	@echo "    make test-nango"
	@echo "    make test-proxy"
	@echo "    make test-connect"

# --- Targeted test commands (no teardown, assumes stack is running) ---

# Auth middleware + org e2e tests
test-auth:
	go test ./internal/middleware/... -v -race -count=1 -run "Auth|MultiAuth_JWTPath"
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestOrg"

# Nango integration CRUD e2e tests
test-nango:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Integration"

# LLM proxy e2e tests (OpenRouter, Fireworks, streaming, tool calls)
test-proxy:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Proxy|TestE2E_Fireworks"

# Connect widget API e2e tests
test-connect:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Connect"

# Connection + scoped token e2e tests
test-connections:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Connection|TestE2E_ScopedToken"

# All integration e2e tests (nango + connect + proxy)
test-integrations:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Integration|TestE2E_Connect|TestE2E_Proxy|TestE2E_Fireworks"

# Run linter
lint:
	golangci-lint run ./...

# Enforce the 300-line ceiling on hand-written Go files. Honours
# scripts/file-length-allowlist.txt for grandfathered exceptions.
check-file-length:
	./scripts/check-go-file-length.sh

# Enforce log-emit-site budget (see scripts/check-log-budget.sh).
check-log-budget:
	./scripts/check-log-budget.sh

# Run go vet
vet:
	go vet ./...

# Run all checks: vet, lint, file-length, log-budget, test, build
check: vet lint check-file-length check-log-budget test build

# Start local development stack (infra only, no proxy)
up:
	docker compose up -d postgres redis mailpit

# Start dev infra, wait for healthy, then run server with hot reload (air)
dev: up
	@echo ""
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U hiveloop -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@echo ""
	@echo "========================================"
	@echo "  Starting HiveLoop (hot reload, debug)"
	@echo "========================================"
	@echo "  Postgres:  localhost:5433"
	@echo "  Redis:     localhost:6379"
	@echo ""
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) air

# Clean slate: tear down, rebuild, run all tests
test-clean:
	@./scripts/test-clean.sh

# Auth middleware + e2e org tests
test-clean-auth:
	@./scripts/test-clean.sh auth

# Nango integration CRUD tests
test-clean-nango:
	@./scripts/test-clean.sh nango

# LLM proxy tests (OpenRouter, Fireworks, streaming, tool calls)
test-clean-proxy:
	@./scripts/test-clean.sh proxy

# Connect widget API tests
test-clean-connect:
	@./scripts/test-clean.sh connect

# All integration tests (nango + connect + proxy)
test-clean-integrations:
	@./scripts/test-clean.sh integrations

# Stop local development stack
down:
	docker compose down -v

# Remove build artifacts
clean:
	rm -rf bin/

# Build Docker image
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t $(IMAGE):$(VERSION) \
		-t $(IMAGE):latest \
		-f docker/Dockerfile .

# Run Docker image locally (connects to host docker-compose infra)
docker-run:
	docker run --rm --network host \
		-e DATABASE_URL=postgres://hiveloop:localdev@localhost:5433/hiveloop?sslmode=disable \
		-e KMS_TYPE=aead \
		-e KMS_KEY=$${KMS_KEY} \
		-e REDIS_ADDR=localhost:6379 \
		-e JWT_SIGNING_KEY=local-dev-signing-key \
		$(IMAGE):latest

# --- RAG test-service targets (Phase 0) ---

# Start the docker-compose services the RAG integration tests need:
# postgres (metadata) + redis (locks) + minio (object storage) + qdrant
# (vector search). Creates the hiveloop-rag-test bucket as a side effect.
test-services-up:
	POSTGRES_PASSWORD=$${POSTGRES_PASSWORD:-localdev} docker compose up -d postgres redis minio qdrant
	@until curl -fsS http://localhost:$${QDRANT_HTTP_PORT:-6333}/readyz >/dev/null 2>&1; do sleep 1; done
	POSTGRES_PASSWORD=$${POSTGRES_PASSWORD:-localdev} docker compose run --rm minio-setup

# Stop those services (keeps data volumes). Use `make down` for a full
# teardown.
test-services-down:
	POSTGRES_PASSWORD=$${POSTGRES_PASSWORD:-localdev} docker compose stop postgres redis minio qdrant

# Run the real Slack bot-token RAG ingestion test against local Postgres/Qdrant.
# Requires read-only Slack bot token and embedding env. By default it loads
# .env.rag plus ../employee.hiveloop.com/.env when present.
ragtest-slack-live:
	@set -a; \
	[ ! -f .env.rag ] || . ./.env.rag; \
	[ ! -f ../employee.hiveloop.com/.env ] || . ../employee.hiveloop.com/.env; \
	set +a; \
	HIVELOOP_E2E_SLACK_RAG=1 \
	HIVELOOP_E2E_KEEP_SLACK_RAG=$${HIVELOOP_E2E_KEEP_SLACK_RAG:-1} \
	QDRANT_HOST=$${QDRANT_HOST:-localhost} \
	QDRANT_PORT=$${QDRANT_PORT:-6334} \
	DATABASE_URL=$${DATABASE_URL:-postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable} \
	go test ./internal/rag/connectors/slack -run TestSlackBotProfileRAGIngestion_Live -count=1 -v

# Run live semantic KB search against an existing local Qdrant collection.
# Requires HIVELOOP_E2E_KB_COLLECTION and HIVELOOP_E2E_KB_ORG_ID.
ragtest-kb-search-live:
	@set -a; \
	[ ! -f .env.rag ] || . ./.env.rag; \
	set +a; \
	HIVELOOP_E2E_KB_SEARCH=1 \
	QDRANT_HOST=$${QDRANT_HOST:-localhost} \
	QDRANT_PORT=$${QDRANT_PORT:-6334} \
	go test ./internal/rag -run TestSearchKnowledgeBase_LiveSlackCollection -count=1 -v

seed-test:
	@PORT=$${DB_PORT:-$$(test -s /tmp/agent-test/pg.port && cat /tmp/agent-test/pg.port || echo 5432)}; \
	PGPASSWORD=$${POSTGRES_PASSWORD:-localdev} psql -q \
		-h $${DB_HOST:-localhost} -p $$PORT \
		-U $${DB_USER:-hiveloop} -d $${DB_NAME:-hiveloop} \
		-f scripts/seed-test-data.sql

# --- Local test stack ---

local-up:
	@./scripts/local-up.sh
	@$(MAKE) -s seed-test

local-down:
	@./scripts/local-down.sh

local-reset:
	@./scripts/local-down.sh
	@$(MAKE) -s local-up

local-status:
	@curl -s -o /dev/null -w "fake-nango (13004) %{http_code}\n" http://localhost:13004/providers.json || true
	@curl -s -o /dev/null -w "backend    (18080) %{http_code}\n" http://localhost:18080/healthz || true
	@curl -s -o /dev/null -w "frontend   (31112) %{http_code}\n" http://localhost:31112/ || true
	@for f in /tmp/agent-test/*.pid; do \
		[ -f "$$f" ] || continue; \
		PID=$$(cat $$f); \
		ps -p $$PID > /dev/null 2>&1 && echo "  alive: $$(basename $$f .pid) (pid $$PID)" || echo "  DEAD : $$(basename $$f .pid)"; \
	done

login-test:
	@./scripts/login-test-session.sh

asynq-peek:
	@./scripts/asynq-peek.sh
