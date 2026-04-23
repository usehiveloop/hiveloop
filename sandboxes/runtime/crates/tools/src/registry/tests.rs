use super::*;

/// A minimal test tool.
struct StubTool {
    tool_name: String,
}

#[async_trait]
impl ToolExecutor for StubTool {
    fn name(&self) -> &str {
        &self.tool_name
    }
    fn description(&self) -> &str {
        "stub"
    }
    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::json!({})
    }
    async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
        Ok("ok".to_string())
    }
    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

fn make_registry() -> ToolRegistry {
    let mut reg = ToolRegistry::new();
    reg.register(Arc::new(StubTool {
        tool_name: "bash".to_string(),
    }));
    reg.register(Arc::new(StubTool {
        tool_name: "Read".to_string(),
    }));
    reg.register(Arc::new(StubTool {
        tool_name: "edit".to_string(),
    }));
    reg.register(Arc::new(StubTool {
        tool_name: "RipGrep".to_string(),
    }));
    reg
}

#[test]
fn test_case_insensitive_lookup_exact() {
    let reg = make_registry();
    assert!(reg.get_case_insensitive("bash").is_some());
}

#[test]
fn test_case_insensitive_lookup_wrong_case() {
    let reg = make_registry();
    // "Bash" should match "bash"
    assert!(reg.get_case_insensitive("Bash").is_some());
    // "read" should match "Read"
    assert!(reg.get_case_insensitive("read").is_some());
}

#[test]
fn test_suggest_tool_close_match() {
    let reg = make_registry();
    // "rread" is close to "Read"
    let suggestion = reg.suggest_tool("rread");
    assert!(suggestion.is_some());
}

#[test]
fn test_suggest_tool_no_match() {
    let reg = make_registry();
    let suggestion = reg.suggest_tool("zzzzzzzzz");
    assert!(suggestion.is_none());
}

#[test]
fn test_unknown_tool_error_with_suggestion() {
    let reg = make_registry();
    let err = reg.unknown_tool_error("bassh");
    assert!(err.contains("Did you mean"));
    assert!(err.contains("bash"));
}

#[test]
fn test_unknown_tool_error_no_suggestion() {
    let reg = make_registry();
    let err = reg.unknown_tool_error("zzzzzzzzz");
    assert!(err.contains("Unknown tool 'zzzzzzzzz'"));
    assert!(err.contains("Available tools:"));
    assert!(!err.contains("Did you mean"));
}

#[test]
fn test_tool_names() {
    let reg = make_registry();
    let names = reg.tool_names();
    assert_eq!(names.len(), 4);
    assert!(names.contains(&"bash".to_string()));
    assert!(names.contains(&"Read".to_string()));
}

#[test]
fn test_list_is_sorted_by_name() {
    let reg = make_registry();
    let list = reg.list();
    let names: Vec<&str> = list.iter().map(|(n, _)| *n).collect();
    // Expected: uppercase sorts before lowercase in byte order
    assert_eq!(names, vec!["Read", "RipGrep", "bash", "edit"]);
}

#[test]
fn test_list_is_deterministic_across_calls() {
    // Build many registries with the same tools in varying insert order;
    // list() must always yield the same sequence. This protects the
    // prompt-cache prefix from HashMap-iteration-order drift.
    let mut results: Vec<Vec<String>> = Vec::new();
    for _ in 0..20 {
        let mut reg = ToolRegistry::new();
        // Insert in a different order each time — tool name-sort should neutralize it.
        for name in ["zzz", "aaa", "mmm", "bbb", "ccc"] {
            reg.register(Arc::new(StubTool {
                tool_name: name.to_string(),
            }));
        }
        let names: Vec<String> = reg.list().iter().map(|(n, _)| n.to_string()).collect();
        results.push(names);
    }
    for window in results.windows(2) {
        assert_eq!(window[0], window[1], "tool list order drifted");
    }
    assert_eq!(results[0], vec!["aaa", "bbb", "ccc", "mmm", "zzz"]);
}

#[test]
fn test_tool_names_is_sorted() {
    let reg = make_registry();
    let names = reg.tool_names();
    let mut sorted = names.clone();
    sorted.sort();
    assert_eq!(names, sorted);
}

#[test]
fn test_format_validation_error_missing_fields() {
    let schema = serde_json::json!({
        "properties": {
            "command": {"type": "string"},
            "timeout": {"type": "number"}
        },
        "required": ["command"]
    });
    let msg = format_validation_error("bash", "missing required property", &schema);
    assert!(msg.contains("Invalid arguments for tool 'bash'"));
    assert!(msg.contains("missing required"));
    assert!(msg.contains("command"));
}

#[test]
fn test_format_validation_error_wrong_type() {
    let schema = serde_json::json!({
        "properties": {
            "command": {"type": "string"}
        },
        "required": ["command"]
    });
    let msg = format_validation_error("bash", "expected string, got number", &schema);
    assert!(msg.contains("wrong type"));
    assert!(msg.contains("command"));
}

#[test]
fn test_format_validation_error_no_required_fields() {
    let schema = serde_json::json!({
        "properties": {
            "optional_field": {"type": "string"}
        }
    });
    let msg = format_validation_error("test", "some error", &schema);
    assert!(msg.contains("Invalid arguments for tool 'test'"));
    assert!(msg.contains("See tool description"));
}
