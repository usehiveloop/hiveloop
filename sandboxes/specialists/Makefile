.PHONY: build build-release run run-release check fmt fmt-check lint test test-unit test-e2e test-e2e-opencode openapi clean help

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

test: ## Run all unit + integration tests
	cargo test --workspace

test-unit: ## Run library tests only
	cargo test --workspace --lib

test-e2e: ## Tear down all stale state, rebuild the docker image, run the Claude harness E2E (6 phases)
	@echo "→ stopping any running bridge-e2e container"
	@docker rm -f bridge-e2e >/dev/null 2>&1 || true
	@echo "→ removing any stale bridge-e2e image"
	@docker rmi -f bridge-e2e:latest >/dev/null 2>&1 || true
	@echo "→ removing dangling event traces"
	@rm -f /tmp/bridge_events.* /tmp/phase3_dump.txt /tmp/e2e_run.log 2>/dev/null || true
	./scripts/e2e_claude.sh

test-e2e-opencode: ## Tear down + rebuild + run the OpenCode harness E2E. Requires OPENCODE_PROVIDER_TYPE / OPENCODE_MODEL / OPENCODE_API_KEY env (optional OPENCODE_BASE_URL).
	@echo "→ stopping any running bridge-e2e container"
	@docker rm -f bridge-e2e >/dev/null 2>&1 || true
	@echo "→ removing any stale bridge-e2e image"
	@docker rmi -f bridge-e2e:latest >/dev/null 2>&1 || true
	@echo "→ removing dangling event traces"
	@rm -f /tmp/bridge_events.* 2>/dev/null || true
	./scripts/e2e_opencode.sh

# --- OpenAPI ---

openapi: ## Generate OpenAPI v3 spec (openapi.json)
	cargo run -p bridge --features openapi --bin gen-openapi

# --- Clean ---

clean: ## Remove build artifacts
	cargo clean

# --- Help ---

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
