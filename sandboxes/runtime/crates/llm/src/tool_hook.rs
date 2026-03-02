use crate::SseEvent;
use rig::agent::{HookAction, PromptHook, ToolCallHookAction};
use rig::completion::CompletionModel;
use tokio::sync::mpsc;
use tracing::debug;

/// A [`PromptHook`] that emits [`SseEvent::ToolCallStart`] and
/// [`SseEvent::ToolCallResult`] events through an SSE channel whenever the
/// agent loop invokes a tool.
#[derive(Clone)]
pub struct ToolCallEmitter {
    pub sse_tx: mpsc::Sender<SseEvent>,
}

impl<M: CompletionModel> PromptHook<M> for ToolCallEmitter {
    async fn on_tool_call(
        &self,
        tool_name: &str,
        tool_call_id: Option<String>,
        internal_call_id: &str,
        args: &str,
    ) -> ToolCallHookAction {
        let id = tool_call_id.unwrap_or_else(|| internal_call_id.to_string());
        let arguments = serde_json::from_str(args)
            .unwrap_or_else(|_| serde_json::Value::String(args.to_string()));

        debug!(tool_name = tool_name, id = %id, "tool call start");

        let _ = self
            .sse_tx
            .send(SseEvent::ToolCallStart {
                id,
                name: tool_name.to_string(),
                arguments,
            })
            .await;

        ToolCallHookAction::Continue
    }

    async fn on_tool_result(
        &self,
        tool_name: &str,
        tool_call_id: Option<String>,
        internal_call_id: &str,
        _args: &str,
        result: &str,
    ) -> HookAction {
        let id = tool_call_id.unwrap_or_else(|| internal_call_id.to_string());

        debug!(tool_name = tool_name, id = %id, "tool call result");

        let _ = self
            .sse_tx
            .send(SseEvent::ToolCallResult {
                id,
                result: result.to_string(),
                is_error: false,
            })
            .await;

        HookAction::cont()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::BridgeCompletionModel;

    #[tokio::test]
    async fn test_emitter_sends_tool_call_start() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter { sse_tx: tx };

        let action = PromptHook::<BridgeCompletionModel>::on_tool_call(
            &emitter,
            "web_search",
            Some("call_123".to_string()),
            "int_456",
            r#"{"query":"test"}"#,
        )
        .await;

        assert_eq!(action, ToolCallHookAction::Continue);

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallStart {
                id,
                name,
                arguments,
            } => {
                assert_eq!(id, "call_123");
                assert_eq!(name, "web_search");
                assert_eq!(arguments, serde_json::json!({"query": "test"}));
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_sends_tool_call_result() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter { sse_tx: tx };

        let action = PromptHook::<BridgeCompletionModel>::on_tool_result(
            &emitter,
            "web_search",
            Some("call_123".to_string()),
            "int_456",
            r#"{"query":"test"}"#,
            r#"{"results": ["page1"]}"#,
        )
        .await;

        assert_eq!(action, HookAction::cont());

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallResult {
                id,
                result,
                is_error,
            } => {
                assert_eq!(id, "call_123");
                assert_eq!(result, r#"{"results": ["page1"]}"#);
                assert!(!is_error);
            }
            other => panic!("expected ToolCallResult, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_returns_continue() {
        let (tx, _rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter { sse_tx: tx };

        let tool_action = PromptHook::<BridgeCompletionModel>::on_tool_call(
            &emitter,
            "test_tool",
            None,
            "internal_1",
            "{}",
        )
        .await;
        assert_eq!(tool_action, ToolCallHookAction::Continue);

        let result_action = PromptHook::<BridgeCompletionModel>::on_tool_result(
            &emitter,
            "test_tool",
            None,
            "internal_1",
            "{}",
            "ok",
        )
        .await;
        assert_eq!(result_action, HookAction::cont());
    }

    #[tokio::test]
    async fn test_emitter_uses_internal_call_id_when_no_tool_call_id() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter { sse_tx: tx };

        PromptHook::<BridgeCompletionModel>::on_tool_call(
            &emitter,
            "my_tool",
            None, // no tool_call_id
            "internal_99",
            "{}",
        )
        .await;

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallStart { id, .. } => {
                assert_eq!(id, "internal_99");
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_handles_invalid_json_args() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter { sse_tx: tx };

        PromptHook::<BridgeCompletionModel>::on_tool_call(
            &emitter,
            "my_tool",
            Some("call_1".to_string()),
            "int_1",
            "not valid json",
        )
        .await;

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallStart { arguments, .. } => {
                assert_eq!(arguments, serde_json::Value::String("not valid json".to_string()));
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }
    }
}
