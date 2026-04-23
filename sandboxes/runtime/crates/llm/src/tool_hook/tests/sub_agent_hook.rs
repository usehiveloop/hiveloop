//! Tests for the `sub_agent` tool interception path.

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
        vec![("coder".to_string(), "A coding agent".to_string())]
    }
    async fn run_foreground(
        &self,
        subagent: &str,
        prompt: &str,
        _task_id: Option<&str>,
    ) -> Result<AgentTaskResult, String> {
        Ok(AgentTaskResult {
            task_id: "agent-task-789".to_string(),
            output: format!("Result from {} for: {}", subagent, prompt),
        })
    }
    async fn run_background(&self, _: &str, _: &str, _: &str) -> Result<AgentTaskHandle, String> {
        Ok(AgentTaskHandle {
            task_id: "bg-agent-456".to_string(),
        })
    }
}

#[tokio::test]
async fn test_emitter_intercepts_sub_agent_tool() {
    let (notif_tx, _notif_rx) = tokio::sync::mpsc::channel::<AgentTaskNotification>(16);
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
                "sub_agent",
                Some("call_sub_agent".to_string()),
                "int_sub_agent",
                r#"{"description":"test task","prompt":"write hello world","subagentName":"coder"}"#,
            )
            .await
        })
        .await;

    // Should return Skip with the foreground result
    match &action {
        ToolCallHookAction::Skip { reason } => {
            assert!(reason.contains("agent-task-789"), "should contain task_id");
            assert!(
                reason.contains("Result from coder"),
                "should contain subagent output"
            );
            assert!(
                reason.contains("<task_result>"),
                "should contain task_result tags"
            );
        }
        other => panic!("expected Skip, got {:?}", other),
    }

    // Verify events: ToolCallStarted + ToolCallCompleted
    let start_event = ws_rx.try_recv().expect("should have tool_call_start");
    assert_eq!(start_event.event_type, BridgeEventType::ToolCallStarted);
    assert_eq!(start_event.data["id"], "call_sub_agent");
    assert_eq!(start_event.data["name"], "sub_agent");

    let result_event = ws_rx.try_recv().expect("should have tool_call_result");
    assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
    assert_eq!(result_event.data["id"], "call_sub_agent");
    assert_eq!(result_event.data["is_error"], false);
    let result_str = result_event.data["result"].as_str().unwrap();
    assert!(result_str.contains("Result from coder"));
}
