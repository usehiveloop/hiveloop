.PHONY: build test test-e2e test-e2e-vault lint vet check up down dev clean fetch-models generate docker-build docker-run test-clean test-clean-logto test-clean-nango test-clean-proxy test-clean-connect test-clean-vault test-clean-integrations test-logto test-nango test-proxy test-connect test-vault test-integrations test-setup vault-up vault-dev openapi

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
IMAGE   ?= llmvault/llmvault

# Fetch models.dev provider catalog and write internal/registry/models.json
fetch-models:
	go run ./cmd/fetchmodels

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
	raw = raw.replace('github_com_useportal_llmvault_internal_registry.', ''); \
	raw = raw.replace('github_com_useportal_llmvault_internal_model.', ''); \
	json.dump(json.loads(raw), open('docs/openapi.json','w'), indent=2) \
	"
	@echo "✓ docs/openapi.json updated"

# Generate all embedded assets (currently just models)
generate: fetch-models

# Build the binary
build:
	go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" \
		-o bin/llmvault ./cmd/server

# Run unit tests with race detection
test:
	go test ./internal/... -v -race -count=1

# Run e2e tests (requires docker-compose stack running)
test-e2e:
	go test ./e2e/... -v -count=1 -timeout=5m

# Run Vault-specific e2e tests (requires docker-compose with Vault running)
test-e2e-vault:
	go test ./e2e/... -v -count=1 -timeout=5m -run "VaultE2E"

# Start services and wait for healthy (no teardown, no tests)
test-setup:
	docker compose up -d postgres redis vault
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U llmvault -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@until docker compose exec -T vault vault status 2>/dev/null | grep -q "Version"; do sleep 1; done
	@echo "  ✓ Vault"
	@echo "  Waiting for Vault Transit key..."
	@until docker compose exec -T vault vault read transit/keys/llmvault-key 2>/dev/null | grep -q "type"; do sleep 2; done
	@echo "  ✓ Vault Transit key ready"
	@echo ""
	@echo "  Infrastructure ready. Run tests with:"
	@echo "    make test-logto"
	@echo "    make test-nango"
	@echo "    make test-proxy"
	@echo "    make test-connect"
	@echo "    make test-vault"

# --- Targeted test commands (no teardown, assumes stack is running) ---

# Logto auth middleware + org e2e tests
test-logto:
	go test ./internal/middleware/... -v -race -count=1 -run "Logto|MultiAuth_LogtoPath"
	go test ./e2e/... -v -count=1 -timeout=5m -run "TestOrg"

# Nango integration CRUD e2e tests
test-nango:
	go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Integration"

# LLM proxy e2e tests (OpenRouter, Fireworks, streaming, tool calls)
test-proxy:
	go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Proxy|TestE2E_Fireworks"

# Connect widget API e2e tests
test-connect:
	go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Connect"

# Vault KMS e2e tests
test-vault:
	go test ./e2e/... -v -count=1 -timeout=5m -run "TestVaultE2E"

# All integration e2e tests (nango + connect + proxy + vault)
test-integrations:
	go test ./e2e/... -v -count=1 -timeout=5m -run "TestE2E_Integration|TestE2E_Connect|TestE2E_Proxy|TestE2E_Fireworks|TestVaultE2E"

# Run linter
lint:
	golangci-lint run ./...

# Run go vet
vet:
	go vet ./...

# Run all checks: vet, lint, test, build
check: vet lint test build

# Start local development stack (infra only, no proxy)
up:
	docker compose up -d postgres redis mailpit

# Start local development stack with Vault (infra only, no proxy)
vault-up:
	docker compose up -d postgres redis vault mailpit

# Start dev stack with Vault, wait for all services
vault-dev: vault-up
	@echo ""
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U llmvault -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@until docker compose exec -T vault vault status 2>/dev/null | grep -q "Version"; do sleep 1; done
	@echo "  ✓ Vault"
	@until curl -sf http://localhost:8025/livez >/dev/null 2>&1; do sleep 2; done
	@echo "  ✓ Mailpit"
	@echo ""
	@echo "========================================"
	@echo "  LLMVault dev stack with Vault is ready"
	@echo "========================================"
	@echo ""
	@echo "  Mailpit UI:       http://localhost:8025"
	@echo "  Vault UI:         http://localhost:8200"
	@echo "  Postgres:         localhost:5433"
	@echo "  Redis:            localhost:6379"
	@echo ""
	@echo "  Hosted services:"
	@echo "    Logto:          https://auth.dev.llmvault.dev"
	@echo "    Nango:          https://integrations.dev.llmvault.dev"
	@echo ""
	@echo "  Vault credentials:"
	@echo "    Token: llmvault-dev-token"
	@echo "    Key:   llmvault-key"
	@echo ""
	@echo "  Add to your .env for Vault KMS:"
	@echo "    KMS_TYPE=vault"
	@echo "    KMS_KEY=llmvault-key"
	@echo "    VAULT_ADDRESS=http://localhost:8200"
	@echo "    VAULT_TOKEN=llmvault-dev-token"
	@echo ""

# Start dev infra, wait for all services to be healthy, print URLs
dev: up
	@echo ""
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U llmvault -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@until curl -sf http://localhost:8025/livez >/dev/null 2>&1; do sleep 2; done
	@echo "  ✓ Mailpit"
	@echo ""
	@echo "========================================"
	@echo "  LLMVault dev stack is ready"
	@echo "========================================"
	@echo ""
	@echo "  Mailpit UI:       http://localhost:8025"
	@echo "  Postgres:         localhost:5433  (user: llmvault, databases: llmvault + llmvault_test)"
	@echo "  Redis:            localhost:6379"
	@echo ""
	@echo "  Hosted services:"
	@echo "    Logto:          https://auth.dev.llmvault.dev"
	@echo "    Nango:          https://integrations.dev.llmvault.dev"
	@echo ""

# Clean slate: tear down, rebuild, run all tests
test-clean:
	@./scripts/test-clean.sh

# Logto auth tests (internal middleware + e2e org tests)
test-clean-logto:
	@./scripts/test-clean.sh logto

# Nango integration CRUD tests
test-clean-nango:
	@./scripts/test-clean.sh nango

# LLM proxy tests (OpenRouter, Fireworks, streaming, tool calls)
test-clean-proxy:
	@./scripts/test-clean.sh proxy

# Connect widget API tests
test-clean-connect:
	@./scripts/test-clean.sh connect

# Vault KMS tests
test-clean-vault:
	@./scripts/test-clean.sh vault

# All integration tests (nango + connect + proxy + vault)
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
		-e DATABASE_URL=postgres://llmvault:localdev@localhost:5433/llmvault?sslmode=disable \
		-e KMS_TYPE=aead \
		-e KMS_KEY=$${KMS_KEY} \
		-e REDIS_ADDR=localhost:6379 \
		-e JWT_SIGNING_KEY=local-dev-signing-key \
		$(IMAGE):latest
