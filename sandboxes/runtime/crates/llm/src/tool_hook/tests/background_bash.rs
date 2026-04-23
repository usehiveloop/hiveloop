//! Tests for the background-bash interception path.

use std::sync::Arc;

use bridge_core::event::BridgeEventType;
use rig::agent::{PromptHook, ToolCallHookAction};
use tools::agent::{
    AgentContext, AgentTaskHandle, AgentTaskNotification, AgentTaskResult, SubAgentRunner,
    TaskBudget, AGENT_CONTEXT,
};

use super::support::{make_bus, make_emitter, TestModel};

struct MockRunner;

#[async_trait::async_trait]
impl SubAgentRunner for MockRunner {
    fn available_subagents(&self) -> Vec<(String, String)> {
        vec![]
    }
    async fn run_foreground(
        &self,
        _: &str,
        _: &str,
        _: Option<&str>,
    ) -> Result<AgentTaskResult, String> {
        Err("not implemented".to_string())
    }
    async fn run_background(&self, _: &str, _: &str, _: &str) -> Result<AgentTaskHandle, String> {
        Err("not implemented".to_string())
    }
}

#[tokio::test]
async fn test_emitter_intercepts_bash_background() {
    let (notif_tx, mut notif_rx) = tokio::sync::mpsc::channel::<AgentTaskNotification>(16);
    let ctx = AgentContext {
        runner: Arc::new(MockRunner),
        notification_tx: notif_tx,
        depth: 0,
        max_depth: 3,
        task_budget: Arc::new(TaskBudget::new(50)),
    };

    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let emitter = make_emitter(bus);

    let action = AGENT_CONTEXT
        .scope(ctx, async {
            PromptHook::<TestModel>::on_tool_call(
                &emitter,
                "bash",
                Some("call_bg".to_string()),
                "int_bg",
                r#"{"command":"echo hook_bg_test","background":true,"description":"bg test"}"#,
            )
            .await
        })
        .await;

    // Should return Skip with the immediate JSON result
    match action {
        ToolCallHookAction::Skip { reason } => {
            let parsed: serde_json::Value =
                serde_json::from_str(&reason).expect("parse skip reason");
            assert!(parsed.get("task_id").is_some());
            assert_eq!(parsed["status"], "running");
        }
        other => panic!("expected Skip, got {:?}", other),
    }

    // Verify events: ToolCallStarted + ToolCallCompleted
    let start_event = ws_rx.try_recv().expect("should have tool_call_start");
    assert_eq!(start_event.event_type, BridgeEventType::ToolCallStarted);
    assert_eq!(start_event.data["id"], "call_bg");

    let result_event = ws_rx.try_recv().expect("should have tool_call_result");
    assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
    assert_eq!(result_event.data["id"], "call_bg");

    // Wait for the background notification
    let notification = tokio::time::timeout(std::time::Duration::from_secs(5), notif_rx.recv())
        .await
        .expect("notification should arrive")
        .expect("channel should not be closed");

    assert_eq!(notification.description, "bg test");
    let output = notification.output.expect("should be Ok");
    assert!(output.contains("hook_bg_test"));
}

#[tokio::test]
async fn test_emitter_does_not_intercept_foreground_bash() {
    let bus = make_bus();
    let emitter = make_emitter(bus);

    // bash without background: true should Continue normally
    let action = PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "bash",
        Some("call_fg".to_string()),
        "int_fg",
        r#"{"command":"echo hello"}"#,
    )
    .await;

    assert_eq!(action, ToolCallHookAction::Continue);
}
