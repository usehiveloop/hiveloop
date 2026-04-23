use super::schema::flatten_schema;
use super::*;
use async_trait::async_trait;
use serde_json::json;

struct MockTool;

#[async_trait]
impl ToolExecutor for MockTool {
    fn name(&self) -> &str {
        "mock_tool"
    }
    fn description(&self) -> &str {
        "A mock tool for testing"
    }
    fn parameters_schema(&self) -> serde_json::Value {
        json!({
            "type": "object",
            "properties": {
                "input": { "type": "string" }
            }
        })
    }
    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let input = args.get("input").and_then(|v| v.as_str()).unwrap_or("none");
        Ok(format!("mock result: {}", input))
    }
    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[tokio::test]
async fn test_dynamic_tool_delegates_to_executor() {
    let executor: Arc<dyn ToolExecutor> = Arc::new(MockTool);
    let tool = DynamicTool::new(executor);

    assert_eq!(tool.name(), "mock_tool");

    let def = tool.definition("test".to_string()).await;
    assert_eq!(def.name, "mock_tool");
    assert_eq!(def.description, "A mock tool for testing");

    let result = tool.call(json!({"input": "hello"})).await;
    assert!(result.is_ok());
    assert_eq!(result.unwrap(), "mock result: hello");
}

#[test]
fn test_adapt_tools() {
    let executors: Vec<Arc<dyn ToolExecutor>> = vec![Arc::new(MockTool)];
    let tools = adapt_tools(executors).unwrap();
    assert_eq!(tools.len(), 1);
}

/// Regression test: the flattened schema for every builtin tool must be
/// byte-stable across two independent builds. HashMap iteration or
/// serde_json Map reorderings would break prompt cache hits silently —
/// this test catches it at compile/CI time.
#[test]
fn test_tool_definitions_are_byte_stable() {
    fn build_all_definitions() -> Vec<rig::completion::ToolDefinition> {
        let mut registry = tools::ToolRegistry::new();
        tools::builtin::register_builtin_tools(&mut registry);
        let mut defs: Vec<_> = registry
            .list()
            .iter()
            .filter_map(|(name, _)| registry.get(name))
            .map(|e| DynamicTool::new(e).definition_sync())
            .collect();
        defs.sort_by(|a, b| a.name.cmp(&b.name));
        defs
    }

    let a = build_all_definitions();
    let b = build_all_definitions();
    assert_eq!(a.len(), b.len(), "tool count drifted");

    for (x, y) in a.iter().zip(b.iter()) {
        assert_eq!(x.name, y.name, "tool name drifted");
        assert_eq!(x.description, y.description, "tool description drifted");
        let x_bytes = serde_json::to_vec(&x.parameters).unwrap();
        let y_bytes = serde_json::to_vec(&y.parameters).unwrap();
        assert_eq!(
            x_bytes, y_bytes,
            "schema for tool '{}' is not byte-stable; HashMap / Map ordering leak",
            x.name
        );
    }
}

/// Parallel regression: the `prefix_hash` helper must yield an identical
/// hex digest for two independent agent builds with the same preamble +
/// tool set. This is the same invariant that the provider cache will
/// check server-side — if this test flaps, our cache hit rate will too.
#[test]
fn test_prefix_hash_is_stable_across_builds() {
    fn build() -> String {
        let mut registry = tools::ToolRegistry::new();
        tools::builtin::register_builtin_tools(&mut registry);
        let defs: Vec<_> = registry
            .list()
            .iter()
            .filter_map(|(name, _)| registry.get(name))
            .map(|e| DynamicTool::new(e).definition_sync())
            .collect();
        crate::prefix_hash::prefix_hash_from_definitions("you are a helpful agent", &defs)
    }

    let h1 = build();
    let h2 = build();
    assert_eq!(h1.len(), 64);
    assert_eq!(h1, h2, "prefix hash drifted — cache will thrash");
}

#[test]
fn test_flatten_schema_removes_defs_and_inlines_refs() {
    let mut registry = tools::ToolRegistry::new();
    tools::builtin::register_builtin_tools(&mut registry);

    for (name, _desc) in registry.list() {
        if let Some(executor) = registry.get(name) {
            let mut schema = executor.parameters_schema();
            let had_defs = schema.get("definitions").is_some() || schema.get("$defs").is_some();

            flatten_schema(&mut schema);
            let schema_str = serde_json::to_string_pretty(&schema).unwrap();

            // After flattening, no schema should have $schema, title, definitions, $defs, or $ref
            assert!(
                schema.get("$schema").is_none(),
                "tool '{}' still has $schema after flatten",
                name
            );
            assert!(
                schema.get("title").is_none(),
                "tool '{}' still has title after flatten",
                name
            );
            assert!(
                schema.get("definitions").is_none(),
                "tool '{}' still has definitions after flatten",
                name
            );
            assert!(
                schema.get("$defs").is_none(),
                "tool '{}' still has $defs after flatten",
                name
            );
            assert!(
                !schema_str.contains("\"$ref\""),
                "tool '{}' still has $ref after flatten:\n{}",
                name,
                schema_str
            );

            if had_defs {
                eprintln!("=== TOOL: {} (flattened) ===\n{}\n", name, schema_str);
            }
        }
    }
}
