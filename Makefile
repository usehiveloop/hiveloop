.PHONY: build test test-e2e lint vet check up down clean fetch-models generate docker-build docker-run

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
IMAGE   ?= llmvault/proxy-bridge

# Fetch models.dev provider catalog and write internal/registry/models.json
fetch-models:
	go run ./cmd/fetchmodels

# Generate all embedded assets (currently just models)
generate: fetch-models

# Build the binary
build:
	go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" \
		-o bin/proxy-bridge ./cmd/server

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
	docker compose up -d postgres vault vault-init redis zitadel-db zitadel zitadel-init

# Start full stack including proxy
up-all:
	docker compose up -d --build

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
		-e DATABASE_URL=postgres://proxybridge:localdev@localhost:5433/proxybridge?sslmode=disable \
		-e VAULT_ADDR=http://localhost:8200 \
		-e VAULT_TOKEN=dev-token \
		-e REDIS_ADDR=localhost:6379 \
		-e JWT_SIGNING_KEY=local-dev-signing-key \
		$(IMAGE):latest
