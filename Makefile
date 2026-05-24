.PHONY: build test test-e2e lint check-file-length vet check up down dev dev-nango dev-nango-secret clean fetch-actions generate docker-build docker-run test-clean test-clean-auth test-clean-nango test-clean-proxy test-clean-connect test-clean-integrations test-auth test-nango test-real-nango test-proxy test-connect test-integrations test-connections test-sandbox-docker test-setup test-setup-nango openapi generate-auth-keys generate-bridge-client generate-employee-bridge-client build-employee-sandbox-templates employee-env-doctor employee-debug-pack test-services-up test-services-down ragtest-slack-live ragtest-kb-search-live seed-test local-up local-down local-reset local-status login-test asynq-peek
.PHONY: sandbox-specialists-build sandbox-specialists-test sandbox-specialists-fmt-check sandbox-specialists-clippy sandbox-specialists-openapi sandbox-employee-build sandbox-employee-test sandbox-employee-fmt-check sandbox-employee-openapi employee-openapi sandbox-employee-image sandbox-employee-image-test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
IMAGE   ?= usehivy/hivy
SANDBOX_SPECIALISTS_DIR ?= sandboxes/specialists
SANDBOX_EMPLOYEE_DIR ?= sandboxes/employee
GO_BIN ?= $(shell if command -v go >/dev/null 2>&1; then command -v go; elif [ -x /opt/homebrew/bin/go ]; then echo /opt/homebrew/bin/go; elif [ -x /usr/local/go/bin/go ]; then echo /usr/local/go/bin/go; else echo go; fi)
DEV_COMPOSE_SERVICES ?= postgres redis nango qdrant minio minio-setup hindsight api worker web
NANGO_SECRET_SQL = SELECT secret_key FROM nango._nango_environments WHERE name='\''prod'\'' LIMIT 1

# Generate base64-encoded RSA private key for HIVY_AUTH_RSA_PRIVATE_KEY env var
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
	swag init -g main.go -d cmd/server,internal -o docs --parseDependency --parseInternal --useStructName
	npx swagger2openapi docs/swagger.json -o docs/openapi.json
	@python3 -c "\
	import json, re; \
	d = json.load(open('docs/openapi.json')); \
	raw = json.dumps(d); \
	raw = raw.replace('internal_handler.', ''); \
	raw = raw.replace('internal_handler_', ''); \
	raw = raw.replace('github_com_usehivy_hivy_internal_registry.', ''); \
	raw = raw.replace('github_com_usehivy_hivy_internal_model.', ''); \
	raw = raw.replace('github_com_usehivy_hivy_internal_mcp_catalog.', ''); \
	json.dump(json.loads(raw), open('docs/openapi.json','w'), indent=2) \
	"
	@echo "✓ docs/openapi.json updated"

# Build + push base sandbox images to GHCR and register Daytona snapshots
# (one per size: small, medium, large, xlarge) pointing at the GHCR image.
# Requires GHCR_USERNAME, GHCR_PAT (PAT with write:packages),
# HIVY_DAYTONA_API_KEY, HIVY_DAYTONA_API_URL, HIVY_DAYTONA_TARGET.
# Usage: make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0
#        make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0 SIZE=small
#        make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0 BRIDGE_BINARY=sandboxes/specialists/target/release/bridge
# VERSION drives the GHCR image tag and Daytona snapshot name. Bump it on every
# rebuild — Daytona freezes the snapshot's mirrored image at create time, so
# reusing a VERSION leaves the old bytes in the control-plane registry.
# BRIDGE_VERSION is the monorepo release tag installed into the image, either
# from ghcr.io/usehivy/hivy release assets or from BRIDGE_BINARY when
# building the specialist bridge directly from sandboxes/specialists.
build-templates:
	@test -n "$(VERSION)" || (echo "error: VERSION is required (e.g. make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0)" && exit 1)
	@test -n "$(BRIDGE_VERSION)" || (echo "error: BRIDGE_VERSION is required (e.g. make build-templates VERSION=1.0.1 BRIDGE_VERSION=v1.0.0)" && exit 1)
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) go run ./cmd/buildtemplates bridge -version=$(VERSION) -bridge-version=$(BRIDGE_VERSION) -size=$(or $(SIZE),all) $(if $(BRIDGE_BINARY),-bridge-binary=$(BRIDGE_BINARY),)

# Register Daytona snapshots from a usehivy/employee-sandbox image already published.
# Requires HIVY_DAYTONA_API_KEY, HIVY_DAYTONA_API_URL, HIVY_DAYTONA_TARGET.
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
	$(GO_BIN) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		--config=internal/bridge/oapi-codegen.yaml openapi/bridge.generated.json
	rm openapi/bridge.generated.json

# Generate Employee Bridge Go client from OpenAPI spec.
generate-employee-bridge-client:
	jq 'walk( \
		if type == "object" and has("oneOf") and (.oneOf | type == "array") and (.oneOf | length == 2) and (.oneOf | any(. == {"type":"null"})) then \
			(.oneOf | map(select(. != {"type":"null"}))[0]) \
		elif type == "object" and has("type") and (.type | type == "array") then \
			.type |= (map(select(. != "null")) | if length == 1 then .[0] else . end) \
		else . end)' \
		$(SANDBOX_EMPLOYEE_DIR)/openapi.json > $(SANDBOX_EMPLOYEE_DIR)/openapi.generated.json
	$(GO_BIN) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		--config=internal/employeebridge/oapi-codegen.yaml $(SANDBOX_EMPLOYEE_DIR)/openapi.generated.json
	rm $(SANDBOX_EMPLOYEE_DIR)/openapi.generated.json

sandbox-specialists-build:
	cd $(SANDBOX_SPECIALISTS_DIR) && cargo build --release -p bridge

sandbox-specialists-test:
	cd $(SANDBOX_SPECIALISTS_DIR) && cargo test --workspace --exclude storage-e2e

sandbox-specialists-fmt-check:
	cd $(SANDBOX_SPECIALISTS_DIR) && cargo fmt --all -- --check

sandbox-specialists-clippy:
	cd $(SANDBOX_SPECIALISTS_DIR) && cargo clippy --workspace -- -D warnings

sandbox-specialists-openapi:
	$(MAKE) -C $(SANDBOX_SPECIALISTS_DIR) openapi
	cp $(SANDBOX_SPECIALISTS_DIR)/openapi.json openapi/bridge.json
	$(MAKE) generate-bridge-client

sandbox-employee-build:
	cd $(SANDBOX_EMPLOYEE_DIR) && cargo build --workspace --all-targets --locked

sandbox-employee-test:
	cd $(SANDBOX_EMPLOYEE_DIR) && cargo test --workspace --locked

sandbox-employee-fmt-check:
	cd $(SANDBOX_EMPLOYEE_DIR) && cargo fmt --all --check

sandbox-employee-openapi employee-openapi:
	$(MAKE) -C $(SANDBOX_EMPLOYEE_DIR) openapi

sandbox-employee-image:
	cd $(SANDBOX_EMPLOYEE_DIR) && cargo build --release --locked -p employee-bridge && scripts/build_runtime_image.sh

sandbox-employee-image-test:
	cd $(SANDBOX_EMPLOYEE_DIR) && scripts/test_runtime_image.sh

# Generate all embedded assets. Note: the model registry is hand-curated in
# internal/registry/models.go and is NOT a generate target — additions go
# through code review, not regeneration.
generate: fetch-actions

# Build the binary
build:
	go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" \
		-o bin/hivy ./cmd/server

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
	@until docker compose exec -T postgres pg_isready -U hivy -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@until curl -fsS http://localhost:9000/minio/health/ready >/dev/null 2>&1; do sleep 1; done
	@echo "  ✓ MinIO"
	@until curl -fsS http://localhost:$${HIVY_COMPOSE_QDRANT_HTTP_PORT:-6333}/readyz >/dev/null 2>&1; do sleep 1; done
	@echo "  ✓ Qdrant"
	docker compose run --rm minio-setup
	@echo ""
	@echo "  Infrastructure ready. Run tests with:"
	@echo "    make test-auth"
	@echo "    make test-nango"
	@echo "    make test-proxy"
	@echo "    make test-connect"

# Start the real Nango service in addition to core infra. The Nango secret is
# generated by Nango on first boot; tests read it from the local nango database.
test-setup-nango:
	docker compose up -d postgres redis nango
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U hivy -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@until curl -fsS http://localhost:$${HIVY_COMPOSE_NANGO_PORT:-23003}/health >/dev/null 2>&1; do sleep 1; done
	@echo "  ✓ Nango"

# --- Targeted test commands (no teardown, assumes stack is running) ---

# Auth middleware + org e2e tests
test-auth:
	go test ./internal/middleware/... -v -race -count=1 -run "Auth|MultiAuth_JWTPath"
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestOrg"

# Nango integration CRUD e2e tests
test-nango:
	$(MAKE) test-real-nango
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Integration"

test-real-nango:
	@secret=$$(docker compose exec -T postgres sh -lc 'psql -U "$$POSTGRES_USER" -d nango -Atc "SELECT secret_key FROM nango._nango_environments WHERE name='\''prod'\'' LIMIT 1"'); \
	if [ -z "$$secret" ]; then echo "Nango secret not found; run make test-setup-nango"; exit 1; fi; \
	RUN_REAL_NANGO_TESTS=1 \
	HIVY_NANGO_ENDPOINT=http://localhost:$${HIVY_COMPOSE_NANGO_PORT:-23003} \
	HIVY_NANGO_SECRET_KEY=$$secret \
	go test ./internal/nango -v -count=1 -run TestRealNango

# LLM proxy e2e tests (OpenRouter, Fireworks, streaming, tool calls)
test-proxy:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Proxy|TestE2E_Fireworks"

# Connect widget API e2e tests
test-connect:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Connect"

# Connection + scoped token e2e tests
test-connections:
	go test ./e2e/... -v -race -count=1 -timeout=5m -run "TestE2E_Connection|TestE2E_ScopedToken"

# Docker sandbox provider integration tests
test-sandbox-docker:
	go test ./internal/sandbox/docker -v -count=1

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

# Start local Nango first and print the generated Hivy API secret.
dev-nango:
	docker compose up -d postgres redis nango
	@echo "Waiting for Nango..."
	@until docker compose exec -T postgres sh -lc 'pg_isready -U "$$POSTGRES_USER" -q' 2>/dev/null; do sleep 1; done
	@until curl -fsS http://localhost:$${HIVY_COMPOSE_NANGO_PORT:-23003}/health >/dev/null 2>&1; do sleep 1; done
	@secret=$$(docker compose exec -T postgres sh -lc 'psql -U "$$POSTGRES_USER" -d nango -Atc "$(NANGO_SECRET_SQL)"'); \
	if [ -z "$$secret" ]; then echo "Nango secret not found after Nango became healthy"; exit 1; fi; \
	echo "export HIVY_NANGO_SECRET_KEY=$$secret"

dev-nango-secret:
	@secret=$$(docker compose exec -T postgres sh -lc 'psql -U "$$POSTGRES_USER" -d nango -Atc "$(NANGO_SECRET_SQL)"'); \
	if [ -z "$$secret" ]; then echo "Nango secret not found; run make dev-nango first" >&2; exit 1; fi; \
	printf '%s\n' "$$secret"

# Start the complete local development stack through docker compose in the background.
up:
	@$(MAKE) -s dev-nango >/dev/null
	@secret=$$($(MAKE) -s dev-nango-secret); \
	echo "Starting Hivy dev stack in background with local Nango secret"; \
	HIVY_NANGO_SECRET_KEY="$$secret" docker compose up -d --build $(DEV_COMPOSE_SERVICES)

# Start the complete local development stack through docker compose in foreground dev mode.
dev:
	@$(MAKE) -s dev-nango >/dev/null
	@secret=$$($(MAKE) -s dev-nango-secret); \
	echo "Starting Hivy dev stack with local Nango secret"; \
	HIVY_NANGO_SECRET_KEY="$$secret" docker compose up --build $(DEV_COMPOSE_SERVICES)

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
		-e HIVY_ENVIRONMENT=development \
		-e HIVY_PORT=8080 \
		-e HIVY_LOG_LEVEL=info \
		-e HIVY_LOG_FORMAT=text \
		-e HIVY_DB_HOST=localhost \
		-e HIVY_DB_PORT=5433 \
		-e HIVY_DB_USER=hivy \
		-e HIVY_DB_PASSWORD=localdev \
		-e HIVY_DB_NAME=hivy \
		-e HIVY_DB_SSLMODE=disable \
		-e HIVY_KMS_TYPE=aead \
		-e HIVY_KMS_KEY=$${HIVY_KMS_KEY} \
		-e HIVY_REDIS_ADDR=localhost:6379 \
		-e HIVY_REDIS_CACHE_TTL=30m \
		-e HIVY_MEM_CACHE_TTL=5m \
		-e HIVY_MEM_CACHE_MAX_SIZE=10000 \
		-e HIVY_JWT_SIGNING_KEY=local-dev-signing-key \
		-e HIVY_AUTH_RSA_PRIVATE_KEY=$${HIVY_AUTH_RSA_PRIVATE_KEY} \
		-e HIVY_FRONTEND_URL=http://localhost:30112 \
		-e HIVY_SPECIALIST_SANDBOX_RUNTIME_VERSION=dev \
		$(IMAGE):latest

# --- RAG test-service targets (Phase 0) ---

# Start the docker-compose services the RAG integration tests need:
# postgres (metadata) + redis (locks) + minio (object storage) + qdrant
# (vector search). Creates the hivy-rag-test bucket as a side effect.
test-services-up:
	HIVY_DB_PASSWORD=$${HIVY_DB_PASSWORD:-localdev} docker compose up -d postgres redis minio qdrant
	@until curl -fsS http://localhost:$${HIVY_COMPOSE_QDRANT_HTTP_PORT:-6333}/readyz >/dev/null 2>&1; do sleep 1; done
	HIVY_DB_PASSWORD=$${HIVY_DB_PASSWORD:-localdev} docker compose run --rm minio-setup

# Stop those services (keeps data volumes). Use `make down` for a full
# teardown.
test-services-down:
	HIVY_DB_PASSWORD=$${HIVY_DB_PASSWORD:-localdev} docker compose stop postgres redis minio qdrant

# Run the real Slack bot-token RAG ingestion test against local Postgres/Qdrant.
# Requires read-only Slack bot token and embedding env. By default it loads
# .env.rag plus sandboxes/employee/.env when present.
ragtest-slack-live:
	@set -a; \
	[ ! -f .env.rag ] || . ./.env.rag; \
	[ ! -f sandboxes/employee/.env ] || . sandboxes/employee/.env; \
	set +a; \
	HIVY_E2E_SLACK_RAG=1 \
	HIVY_E2E_KEEP_SLACK_RAG=$${HIVY_E2E_KEEP_SLACK_RAG:-1} \
	HIVY_QDRANT_HOST=$${HIVY_QDRANT_HOST:-localhost} \
	HIVY_QDRANT_PORT=$${HIVY_QDRANT_PORT:-6334} \
	HIVY_DATABASE_URL=$${HIVY_DATABASE_URL:-postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable} \
	go test ./internal/rag/connectors/slack -run TestSlackBotProfileRAGIngestion_Live -count=1 -v

# Run live semantic KB search against an existing local Qdrant collection.
# Requires HIVY_E2E_KB_COLLECTION and HIVY_E2E_KB_ORG_ID.
ragtest-kb-search-live:
	@set -a; \
	[ ! -f .env.rag ] || . ./.env.rag; \
	set +a; \
	HIVY_E2E_KB_SEARCH=1 \
	HIVY_QDRANT_HOST=$${HIVY_QDRANT_HOST:-localhost} \
	HIVY_QDRANT_PORT=$${HIVY_QDRANT_PORT:-6334} \
	go test ./internal/rag -run TestSearchKnowledgeBase_LiveSlackCollection -count=1 -v

seed-test:
	@PG_PORT=$${HIVY_DB_PORT:-$$(test -s /tmp/agent-test/pg.port && cat /tmp/agent-test/pg.port || echo 5432)}; \
	PGPASSWORD=$${HIVY_DB_PASSWORD:-localdev} psql -q \
		-h $${HIVY_DB_HOST:-localhost} -p $$PG_PORT \
		-U $${HIVY_DB_USER:-hivy} -d $${HIVY_DB_NAME:-hivy} \
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
