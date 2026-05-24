//! Translate ACP `SessionUpdate` and lifecycle moments into [`BridgeEvent`].

use agent_client_protocol::schema::{
    ContentBlock, ContentChunk, SessionUpdate, ToolCall, ToolCallStatus, ToolCallUpdate,
};
use bridge_core::event::{BridgeEvent, BridgeEventType};
use serde_json::{json, Value};

/// Map a single ACP `SessionUpdate` into zero or more `BridgeEvent`s,
/// stamping agent_id / conversation_id from the surrounding context.
pub fn map_update(
    agent_id: &str,
    conversation_id: &str,
    update: &SessionUpdate,
) -> Vec<BridgeEvent> {
    match update {
        SessionUpdate::AgentMessageChunk(chunk) => {
            vec![BridgeEvent::new(
                BridgeEventType::ResponseChunk,
                agent_id,
                conversation_id,
                chunk_payload(chunk),
            )]
        }
        SessionUpdate::AgentThoughtChunk(chunk) => {
            vec![BridgeEvent::new(
                BridgeEventType::ReasoningDelta,
                agent_id,
                conversation_id,
                chunk_payload(chunk),
            )]
        }
        SessionUpdate::UserMessageChunk(chunk) => {
            // Forwarded back to clients as MessageReceived for symmetry —
            // the API layer already emits this on send_message, but ACP may
            // also re-emit on session resume.
            vec![BridgeEvent::new(
                BridgeEventType::MessageReceived,
                agent_id,
                conversation_id,
                chunk_payload(chunk),
            )]
        }
        SessionUpdate::ToolCall(call) => {
            vec![BridgeEvent::new(
                BridgeEventType::ToolCallStarted,
                agent_id,
                conversation_id,
                tool_call_payload(call),
            )]
        }
        SessionUpdate::ToolCallUpdate(update) => {
            vec![BridgeEvent::new(
                tool_call_update_event_type(update),
                agent_id,
                conversation_id,
                tool_call_update_payload(update),
            )]
        }
        SessionUpdate::Plan(plan) => {
            vec![BridgeEvent::new(
                BridgeEventType::TodoUpdated,
                agent_id,
                conversation_id,
                json!({ "plan": serde_json::to_value(plan).unwrap_or(Value::Null) }),
            )]
        }
        // Updates we don't translate yet — drop silently.
        SessionUpdate::AvailableCommandsUpdate(_)
        | SessionUpdate::CurrentModeUpdate(_)
        | SessionUpdate::ConfigOptionUpdate(_)
        | SessionUpdate::SessionInfoUpdate(_)
        | SessionUpdate::UsageUpdate(_) => Vec::new(),
        // Future variants (`#[non_exhaustive]`).
        _ => Vec::new(),
    }
}

fn chunk_payload(chunk: &ContentChunk) -> Value {
    json!({ "content": content_block_payload(&chunk.content) })
}

fn content_block_payload(block: &ContentBlock) -> Value {
    match block {
        ContentBlock::Text(t) => json!({ "type": "text", "text": t.text }),
        ContentBlock::Image(_) => json!({ "type": "image" }),
        ContentBlock::Audio(_) => json!({ "type": "audio" }),
        ContentBlock::ResourceLink(r) => {
            json!({ "type": "resource_link", "uri": r.uri, "name": r.name })
        }
        ContentBlock::Resource(_) => json!({ "type": "resource" }),
        _ => json!({ "type": "unknown" }),
    }
}

fn tool_call_payload(call: &ToolCall) -> Value {
    json!({
        "tool_call_id": call.tool_call_id.0.as_ref(),
        "title": call.title,
        "status": format!("{:?}", call.status).to_ascii_lowercase(),
        "kind": format!("{:?}", call.kind).to_ascii_lowercase(),
        "raw_input": call.raw_input.clone().unwrap_or(Value::Null),
    })
}

fn tool_call_update_event_type(update: &ToolCallUpdate) -> BridgeEventType {
    match update.fields.status {
        Some(ToolCallStatus::Completed) | Some(ToolCallStatus::Failed) => {
            BridgeEventType::ToolCallCompleted
        }
        _ => BridgeEventType::ToolCallStarted,
    }
}

fn tool_call_update_payload(update: &ToolCallUpdate) -> Value {
    json!({
        "tool_call_id": update.tool_call_id.0.as_ref(),
        "status": update.fields.status.map(|s| format!("{:?}", s).to_ascii_lowercase()),
        "title": update.fields.title,
        "raw_output": update.fields.raw_output.clone().unwrap_or(Value::Null),
    })
}
