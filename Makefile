.PHONY: build test test-e2e lint vet check up down clean

# Build the binary
build:
	go build -o bin/proxy-bridge ./cmd/server

# Run unit tests with race detection
test:
	go test ./internal/... -v -race -count=1

# Run e2e tests (requires docker-compose stack running)
test-e2e:
	go test ./e2e/... -v -tags=e2e -count=1 -timeout=5m

# Run linter
lint:
	golangci-lint run ./...

# Run go vet
vet:
	go vet ./...

# Run all checks: vet, lint, test, build
check: vet lint test build

# Start local development stack
up:
	docker compose up -d

# Stop local development stack
down:
	docker compose down -v

# Remove build artifacts
clean:
	rm -rf bin/
