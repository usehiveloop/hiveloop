# Testing

Testing Bridge.

---

## Test Types

### Unit Tests

Test individual functions:

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_agent() {
        let json = r#"{"id": "test"}"#;
        let agent: Agent = serde_json::from_str(json).unwrap();
        assert_eq!(agent.id, "test");
    }
}
```

Run:

```bash
cargo test --workspace --exclude bridge-e2e
```

### Integration Tests

Test API endpoints:

```rust
#[tokio::test]
async fn test_push_agent() {
    let app = create_test_app().await;
    let response = app
        .post("/push/agents")
        .json(&agent_definition)
        .send()
        .await;
    
    assert_eq!(response.status(), 200);
}
```

### E2E Tests

Test full workflows with real LLM calls.

Requires API keys in `.env`:

```bash
FIREWORKS_API_KEY=...
ANTHROPIC_API_KEY=...
GEMINI_API_KEY=...
COHERE_API_KEY=...
```

Run:

```bash
cargo test -p bridge-e2e --test e2e_tests -- --test-threads=1
```

---

## Test Fixtures

Sample data in `fixtures/`:

```
fixtures/
├── agents/
│   ├── simple.json
│   └── with_tools.json
└── workspaces/
    └── sample_project/
```

Load in tests:

```rust
let agent = fs::read_to_string("fixtures/agents/simple.json").unwrap();
```

---

## Mocking

### Mock LLM Provider

```rust
struct MockProvider;

impl LLMProvider for MockProvider {
    async fn generate(&self, _request: Request) -> Result<Response> {
        Ok(Response {
            content: "Test response".to_string(),
            usage: Usage::default(),
        })
    }
}
```

### Mock MCP Server

See `e2e/mock-mcp-server/` for example.

---

## Test Utilities

Common helpers in test modules:

```rust
pub async fn create_test_app() -> TestApp {
    // Setup test Bridge instance
}

pub fn load_fixture(name: &str) -> String {
    fs::read_to_string(format!("fixtures/{}", name)).unwrap()
}
```

---

## CI Testing

GitHub Actions runs:

1. `cargo fmt --check`
2. `cargo clippy`
3. `cargo test --workspace --exclude bridge-e2e`

E2E tests run separately (require API keys).

---

## Debugging Tests

```bash
# Single test
cargo test test_name -- --nocapture

# With logs
RUST_LOG=debug cargo test test_name -- --nocapture

# E2E with specific provider
cargo test fireworks -- --test-threads=1
```

---

## Best Practices

1. **Test edge cases** — Empty input, large input, special characters
2. **Test errors** — Verify error messages are helpful
3. **Use fixtures** — Don't embed large JSON in test code
4. **Clean up** — Tests shouldn't leave state behind
5. **Fast tests** — Mock slow operations

---

## See Also

- [Development](index.md)
- [E2E tests in repository](../../e2e/)
