.PHONY: build build-release run run-release check fmt fmt-check lint test test-all test-unit test-e2e test-lsp test-lsp-integration test-e2e-llm test-e2e-observability test-e2e-approval test-e2e-integration-real test-e2e-codedb test-e2e-parallel setup-lsp openapi tools tools-debug tools-readonly tools-readonly-debug clean

# --- Build ---

build: ## Build debug binary
	cargo build -p bridge

build-release: ## Build optimized release binary
	cargo build --release -p bridge

# --- Run ---

run: ## Run bridge (debug)
	cargo run -p bridge

run-release: ## Run bridge (release)
	cargo run --release -p bridge

# --- Check / Lint / Format ---

check: ## Type-check all crates
	cargo check --workspace

fmt: ## Format all code
	cargo fmt --all

fmt-check: ## Check formatting without modifying
	cargo fmt --all -- --check

lint: ## Run clippy linter
	cargo clippy --workspace -- -D warnings

# --- Tests ---

test: ## Run all unit tests (fast, no servers)
	cargo test --workspace

test-unit: ## Run library tests only
	cargo test --workspace --lib

test-e2e: ## Run e2e tests (single-threaded)
	cargo test -p bridge-e2e --test e2e_tests -- --test-threads=1

test-lsp: ## Run LSP unit tests
	cargo test -p lsp

test-lsp-integration: ## Run LSP integration tests (requires setup-lsp)
	cargo test -p lsp -- --ignored

test-all: ## Run everything (requires FIREWORKS_API_KEY in env or .env file)
	@if [ -f .env ] && [ -z "$$FIREWORKS_API_KEY" ]; then \
		export $$(grep -v '^#' .env | grep FIREWORKS_API_KEY | xargs); \
	fi; \
	if [ -z "$$FIREWORKS_API_KEY" ]; then \
		echo "Error: FIREWORKS_API_KEY is not set and no .env file found"; \
		exit 1; \
	fi; \
	$(MAKE) setup-lsp && \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test --workspace --exclude bridge-e2e -- --include-ignored && \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test -p bridge-e2e -- --include-ignored --test-threads=1

# --- E2E LLM Tests (require FIREWORKS_API_KEY in .env or environment) ---

test-e2e-llm: ## Run real LLM e2e tests
	@if [ -z "$$FIREWORKS_API_KEY" ] && [ -f .env ]; then export $$(grep -v '^#' .env | grep FIREWORKS_API_KEY | xargs); fi; \
	if [ -z "$$FIREWORKS_API_KEY" ]; then echo "Error: FIREWORKS_API_KEY not set. Add it to .env or export it."; exit 1; fi; \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test -p bridge-e2e --test real_e2e_tests -- --ignored --test-threads=1 --nocapture

test-e2e-observability: ## Run observability e2e tests (webhook token/model data)
	@if [ -z "$$FIREWORKS_API_KEY" ] && [ -f .env ]; then export $$(grep -v '^#' .env | grep FIREWORKS_API_KEY | xargs); fi; \
	if [ -z "$$FIREWORKS_API_KEY" ]; then echo "Error: FIREWORKS_API_KEY not set. Add it to .env or export it."; exit 1; fi; \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test -p bridge-e2e --test observability_e2e_tests -- --ignored --test-threads=1 --nocapture

test-e2e-approval: ## Run approval flow e2e tests
	@if [ -z "$$FIREWORKS_API_KEY" ] && [ -f .env ]; then export $$(grep -v '^#' .env | grep FIREWORKS_API_KEY | xargs); fi; \
	if [ -z "$$FIREWORKS_API_KEY" ]; then echo "Error: FIREWORKS_API_KEY not set. Add it to .env or export it."; exit 1; fi; \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test -p bridge-e2e --test approval_e2e_tests -- --ignored --test-threads=1 --nocapture

test-e2e-integration-real: ## Run integration e2e tests with real LLM
	@if [ -z "$$FIREWORKS_API_KEY" ] && [ -f .env ]; then export $$(grep -v '^#' .env | grep FIREWORKS_API_KEY | xargs); fi; \
	if [ -z "$$FIREWORKS_API_KEY" ]; then echo "Error: FIREWORKS_API_KEY not set. Add it to .env or export it."; exit 1; fi; \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test -p bridge-e2e --test integration_real_e2e_tests -- --ignored --test-threads=1 --nocapture

test-e2e-codedb: ## Run codedb e2e tests (requires codedb binary)
	@if [ -z "$$FIREWORKS_API_KEY" ] && [ -f .env ]; then export $$(grep -v '^#' .env | grep FIREWORKS_API_KEY | xargs); fi; \
	if [ -z "$$FIREWORKS_API_KEY" ]; then echo "Error: FIREWORKS_API_KEY not set. Add it to .env or export it."; exit 1; fi; \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test -p bridge-e2e --test codedb_e2e_tests -- --ignored --test-threads=1 --nocapture

test-e2e-parallel: ## Run parallel subagent e2e tests
	@if [ -z "$$FIREWORKS_API_KEY" ] && [ -f .env ]; then export $$(grep -v '^#' .env | grep FIREWORKS_API_KEY | xargs); fi; \
	if [ -z "$$FIREWORKS_API_KEY" ]; then echo "Error: FIREWORKS_API_KEY not set. Add it to .env or export it."; exit 1; fi; \
	FIREWORKS_API_KEY="$$FIREWORKS_API_KEY" cargo test -p bridge-e2e --test parallel_e2e_tests -- --test-threads=1 --nocapture

# --- Setup ---

setup-lsp: ## Install LSP servers for integration tests
	./scripts/setup-lsp-servers.sh

# --- OpenAPI ---

openapi: ## Generate OpenAPI v3 spec (openapi.json)
	cargo run -p bridge --features openapi --bin gen-openapi

# --- CLI ---

tools: ## List available tools (JSON format)
	./target/release/bridge tools list --json

tools-debug: ## List available tools using debug build
	cargo run -p bridge --bin bridge -- tools list --json

tools-readonly: ## List read-only tools (JSON format)
	./target/release/bridge tools list --read-only

tools-readonly-debug: ## List read-only tools using debug build
	cargo run -p bridge --bin bridge -- tools list --read-only

# --- Clean ---

clean: ## Remove build artifacts
	cargo clean

# --- Help ---

help: ## Show this help
	@grep -E '^[[:alnum:]_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-26s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
