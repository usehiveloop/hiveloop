# Development

Contributing to Bridge development.

---

## Getting Started

Clone and build:

```bash
git clone https://github.com/useportal-app/bridge.git
cd bridge
make build
```

---

## Project Structure

```
crates/
├── bridge/     # Main binary
├── api/        # HTTP API
├── core/       # Domain models
├── runtime/    # Agent runtime
├── llm/        # LLM providers
├── tools/      # Built-in tools
├── mcp/        # MCP client
├── webhooks/   # Webhook dispatch
└── lsp/        # LSP integration

e2e/            # End-to-end tests
fixtures/       # Test data
```

---

## Development Commands

```bash
make build          # Debug build
make build-release  # Optimized build
make test           # Unit tests
make test-e2e       # E2E tests
make lint           # Run clippy
make fmt            # Format code
make check          # Type check
```

---

## Testing

### Unit Tests

```bash
cargo test --workspace --exclude bridge-e2e
```

### E2E Tests

Requires API keys:

```bash
export FIREWORKS_API_KEY="..."
export ANTHROPIC_API_KEY="..."
cargo test -p bridge-e2e --test e2e_tests
```

---

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run `make test` and `make lint`
6. Submit a pull request

---

## Read More

- [Architecture Deep Dive](architecture-deep-dive.md)
- [Testing Guide](testing.md)
- [Adding a Tool](adding-a-tool.md)
