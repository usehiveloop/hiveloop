.PHONY: build test test-e2e lint vet check up down dev clean fetch-models generate docker-build docker-run test-clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
IMAGE   ?= llmvault/llmvault

# Fetch models.dev provider catalog and write internal/registry/models.json
fetch-models:
	go run ./cmd/fetchmodels

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
	docker compose up -d postgres redis zitadel zitadel-init

# Start dev infra, wait for all services to be healthy, print URLs
dev: up
	@echo ""
	@echo "Waiting for services..."
	@until docker compose exec -T postgres pg_isready -U llmvault -q 2>/dev/null; do sleep 1; done
	@echo "  ✓ Postgres"
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
	@echo "  ✓ Redis"
	@until curl -sf http://localhost:8085/debug/ready >/dev/null 2>&1; do sleep 2; done
	@echo "  ✓ ZITADEL"
	@echo ""
	@echo "========================================"
	@echo "  LLMVault dev stack is ready"
	@echo "========================================"
	@echo ""
	@echo "  ZITADEL Console:  http://localhost:8085/ui/console"
	@echo "  ZITADEL Login:    http://localhost:8085/ui/login"
	@echo "  Postgres:         localhost:5433  (user: llmvault, databases: llmvault + llmvault_test)"
	@echo "  Redis:            localhost:6379"
	@echo ""
	@echo "  Default ZITADEL admin login:"
	@echo "    Email:    zitadel-admin@zitadel.localhost"
	@echo "    Password: Password1!"
	@echo ""
	@echo "  Copy the ZITADEL credentials from init output into your .env:"
	@echo "    docker compose logs zitadel-init"
	@echo ""

# Clean slate: tear down, rebuild, run all tests
test-clean:
	@./scripts/test-clean.sh

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
