.PHONY: build test test-e2e lint check-file-length vet check up down dev clean fetch-actions generate docker-build docker-run test-clean test-clean-auth test-clean-nango test-clean-proxy test-clean-connect test-clean-integrations test-auth test-nango test-proxy test-connect test-integrations test-connections test-setup openapi generate-auth-keys upload-skills test-services-up test-services-down seed-test local-up local-down local-reset local-status login-test asynq-peek

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
# Usage: make build-templates VERSION=0.10.0
#        make build-templates VERSION=0.10.0 SIZE=small
#        make build-templates VERSION=0.10.0 SIZE=small,medium
#        make build-templates VERSION=0.10.0 FLAVOR=dev-box
#        make build-templates VERSION=0.10.0 FLAVOR=dev-box SIZE=medium
build-templates:
	@test -n "$(VERSION)" || (echo "error: VERSION is required (e.g. make build-templates VERSION=0.10.0)" && exit 1)
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) go run ./cmd/buildtemplates -version=$(VERSION) -flavor=$(or $(FLAVOR),bridge) -size=$(or $(SIZE),all)

# Upload skill definitions to Hiveloop API (reads HIVELOOP_SKILLS_API_KEY from .env)
upload-skills:
	env $$(grep -v '^\s*\#' .env | grep -v '^\s*$$' | xargs) go run ./skills/upload

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
	docker compose up -d postgres redis
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U hiveloop -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
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

# Run go vet
vet:
	go vet ./...

# Run all checks: vet, lint, file-length, test, build
check: vet lint check-file-length test build

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
# postgres (for all tranches) + redis (Phase 2 locks) + minio (Phase 0
# LanceDB spike + Phase 2 vectorstore tests). Creates the
# hiveloop-rag-test bucket as a side effect.
test-services-up:
	POSTGRES_PASSWORD=$${POSTGRES_PASSWORD:-localdev} docker compose up -d postgres redis minio
	POSTGRES_PASSWORD=$${POSTGRES_PASSWORD:-localdev} docker compose run --rm minio-setup

# Stop those services (keeps data volumes). Use `make down` for a full
# teardown.
test-services-down:
	POSTGRES_PASSWORD=$${POSTGRES_PASSWORD:-localdev} docker compose stop postgres redis minio

# Seed deterministic test data (users, orgs, integrations, API key, agent,
# revoked connection). Idempotent. Requires the backend to have booted at
# least once so AutoMigrate has created the schema.
seed-test:
	@PGPASSWORD=$${POSTGRES_PASSWORD:-localdev} psql -q \
		-h $${DB_HOST:-localhost} -p $${DB_PORT:-5433} \
		-U $${DB_USER:-hiveloop} -d $${DB_NAME:-hiveloop} \
		-f scripts/seed-test-data.sql

# --- Local test stack (native postgres + redis + supervised apps) ---

# Bring up the full stack: pg + redis (system services) + fake-nango +
# backend + frontend. Apps run under supervisors that restart on crash with
# a 2s delay. Pids in /tmp/agent-test/.
local-up:
	@./scripts/local-up.sh

# Stop the supervised app processes. Leaves postgres/redis running.
# HARD=1 also wipes /tmp/agent-test logs + env files.
local-down:
	@./scripts/local-down.sh

# Restart the apps and re-seed. Use after big code changes or to clear state.
local-reset:
	@./scripts/local-down.sh
	@./scripts/local-up.sh
	@$(MAKE) -s seed-test

# Quick health check across the stack.
local-status:
	@curl -s -o /dev/null -w "fake-nango (13004) %{http_code}\n" http://localhost:13004/providers.json || true
	@curl -s -o /dev/null -w "backend    (18080) %{http_code}\n" http://localhost:18080/healthz || true
	@curl -s -o /dev/null -w "frontend   (31112) %{http_code}\n" http://localhost:31112/ || true
	@for f in /tmp/agent-test/*.pid; do \
		[ -f "$$f" ] || continue; \
		PID=$$(cat $$f); \
		ps -p $$PID > /dev/null 2>&1 && echo "  alive: $$(basename $$f .pid) (pid $$PID)" || echo "  DEAD : $$(basename $$f .pid)"; \
	done

# Drive the OTP login flow programmatically. Stores __session in the
# agent-browser Chrome session so subsequent UI calls are authed.
login-test:
	@./scripts/login-test-session.sh

# Print queue counts (pending/active/scheduled/retry/archived) per asynq queue.
# VERBOSE=1 also samples task type names.
asynq-peek:
	@./scripts/asynq-peek.sh
