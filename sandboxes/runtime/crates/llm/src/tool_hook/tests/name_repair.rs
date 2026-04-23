//! Tests for the tool-name resolution / auto-repair paths.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use bridge_core::event::BridgeEventType;
use rig::agent::{PromptHook, ToolCallHookAction};
use tools::ToolExecutor;

use super::support::{make_bus, make_emitter, make_emitter_with, TestModel};

#[tokio::test]
async fn test_emitter_intercepts_unknown_tool() {
    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let tool_names: HashSet<String> = ["bash", "read", "edit", "grep"]
        .iter()
        .map(|s| s.to_string())
        .collect();
    let emitter = make_emitter_with(bus, tool_names, HashMap::new());

    let action = PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "bassh",
        Some("call_typo".to_string()),
        "int_typo",
        r#"{"command":"echo hello"}"#,
    )
    .await;

    // Should return Skip with an error suggesting "bash"
    match &action {
        ToolCallHookAction::Skip { reason } => {
            assert!(reason.contains("Unknown tool 'bassh'"));
            assert!(reason.contains("bash"));
        }
        other => panic!("expected Skip, got {:?}", other),
    }

    // Should emit ToolCallStarted and ToolCallCompleted (error)
    let _start = ws_rx.try_recv().expect("should have tool_call_start");
    let result_event = ws_rx.try_recv().expect("should have tool_call_result");
    assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
    assert_eq!(result_event.data["is_error"], true);
    let result_str = result_event.data["result"].as_str().unwrap();
    assert!(result_str.contains("Unknown tool"));
}

#[tokio::test]
async fn test_emitter_allows_known_tools() {
    let bus = make_bus();
    let tool_names: HashSet<String> = ["bash", "read", "edit"]
        .iter()
        .map(|s| s.to_string())
        .collect();
    let emitter = make_emitter_with(bus, tool_names, HashMap::new());

    // Known tool should pass through
    let action = PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "bash",
        Some("call_ok".to_string()),
        "int_ok",
        r#"{"command":"echo hello"}"#,
    )
    .await;

    assert_eq!(action, ToolCallHookAction::Continue);
}

#[tokio::test]
async fn test_emitter_empty_tool_names_skips_check() {
    let bus = make_bus();
    let emitter = make_emitter(bus);

    // With empty tool_names, all tools should pass through (backward compat)
    let action = PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "anything_goes",
        Some("call_any".to_string()),
        "int_any",
        "{}",
    )
    .await;

    assert_eq!(action, ToolCallHookAction::Continue);
}

struct StubBash(&'static str);

#[async_trait::async_trait]
impl ToolExecutor for StubBash {
    fn name(&self) -> &str {
        "bash"
    }
    fn description(&self) -> &str {
        "stub"
    }
    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::json!({})
    }
    async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
        Ok(self.0.to_string())
    }
    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[tokio::test]
async fn test_emitter_auto_repairs_case_mismatch() {
    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let tool_names: HashSet<String> = ["bash", "Read", "edit"]
        .iter()
        .map(|s| s.to_string())
        .collect();

    let mut executors: HashMap<String, Arc<dyn ToolExecutor>> = HashMap::new();
    executors.insert(
        "bash".to_string(),
        Arc::new(StubBash("repaired_bash_output")),
    );

    let emitter = make_emitter_with(bus, tool_names, executors);

    // "Bash" should auto-repair to "bash" and execute directly
    let action = PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "Bash",
        Some("call_case".to_string()),
        "int_case",
        r#"{"command":"echo hello"}"#,
    )
    .await;

    match &action {
        ToolCallHookAction::Skip { reason } => {
            assert!(
                reason.contains("repaired_bash_output"),
                "should contain the tool output: {}",
                reason
            );
        }
        other => panic!("expected Skip with repaired output, got {:?}", other),
    }

    // Should emit ToolCallStarted + ToolCallCompleted
    let _start = ws_rx.try_recv().expect("should have tool_call_start");
    let result_event = ws_rx.try_recv().expect("should have tool_call_result");
    assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
    assert_eq!(result_event.data["is_error"], false);
    let result_str = result_event.data["result"].as_str().unwrap();
    assert!(result_str.contains("repaired_bash_output"));
}

#[tokio::test]
async fn test_emitter_auto_repairs_whitespace() {
    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let tool_names: HashSet<String> = ["bash", "Read", "edit"]
        .iter()
        .map(|s| s.to_string())
        .collect();

    let mut executors: HashMap<String, Arc<dyn ToolExecutor>> = HashMap::new();
    executors.insert("bash".to_string(), Arc::new(StubBash("trimmed_output")));

    let emitter = make_emitter_with(bus, tool_names, executors);

    // " bash" (leading space) should auto-repair to "bash"
    let action = PromptHook::<TestModel>::on_tool_call(
        &emitter,
        " bash",
        Some("call_ws".to_string()),
        "int_ws",
        r#"{"command":"echo hello"}"#,
    )
    .await;

    match &action {
        ToolCallHookAction::Skip { reason } => {
            assert!(
                reason.contains("trimmed_output"),
                "should contain the tool output: {}",
                reason
            );
        }
        other => panic!("expected Skip with repaired output, got {:?}", other),
    }

    let _start = ws_rx.try_recv().expect("should have tool_call_start");
    let result_event = ws_rx.try_recv().expect("should have tool_call_result");
    assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
    assert_eq!(result_event.data["is_error"], false);
}
