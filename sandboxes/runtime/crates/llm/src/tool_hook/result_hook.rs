//! `on_tool_result` impl — completion-side logging, event emission, and
//! context-pressure tracking.

use bridge_core::event::{BridgeEvent, BridgeEventType};
use rig::agent::HookAction;
use serde_json::json;
use tracing::info;

use super::result_classify::looks_like_failure;
use super::truncate::Truncated;
use super::ToolCallEmitter;

impl ToolCallEmitter {
    pub(super) async fn handle_tool_result(
        &self,
        tool_name: &str,
        tool_call_id: Option<String>,
        internal_call_id: &str,
        args: &str,
        result: &str,
    ) -> HookAction {
        let id = tool_call_id.unwrap_or_else(|| internal_call_id.to_string());

        // Look up pending timing from on_tool_call (path 8 — rig-core dispatch)
        let (duration_ms, effective_name) =
            if let Some((_, (start, ename))) = self.pending_tool_timings.remove(&id) {
                (Some(start.elapsed().as_millis() as u64), ename)
            } else {
                (None, tool_name.to_string())
            };

        let is_failure = looks_like_failure(result);

        if let Some(dur) = duration_ms {
            self.metrics
                .record_tool_call_detailed(&effective_name, false, is_failure, dur);
            if let Some(ref cm) = self.conversation_metrics {
                cm.record_tool_call(dur);
            }
        }

        info!(
            agent_id = %self.agent_id,
            conversation_id = %self.conversation_id,
            tool_name = %effective_name,
            tool_call_id = %id,
            duration_ms = duration_ms.unwrap_or(0),
            is_error = false,
            is_failure = is_failure,
            result = %Truncated::new(result, 80),
            "tool_call_complete"
        );

        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallCompleted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &id, "result": result, "is_error": false, "is_failure": is_failure, "duration_ms": duration_ms, "tool_name": &effective_name}),
        ));

        // Mid-turn context-pressure tracking — counts bytes of tool output
        // this turn, emits ContextPressureWarning once past the threshold.
        self.note_tool_output_bytes(result.len());

        let args_value = serde_json::from_str(args).unwrap_or(serde_json::Value::Null);
        self.persist_tool_interaction(&effective_name, &id, &args_value, result, false);

        // Emit a structured TodoUpdated event when the todowrite tool
        // completes. Read the todos from the call's *arguments* — the tool
        // result intentionally no longer echoes the list (see
        // `TodoWriteResult` doc on why), so the args are the only place
        // the full list lives at this point.
        if tool_name == "todowrite" {
            if let Some(todos) = args_value.get("todos") {
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::TodoUpdated,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"todos": todos}),
                ));
            }
        }

        HookAction::cont()
    }
}
