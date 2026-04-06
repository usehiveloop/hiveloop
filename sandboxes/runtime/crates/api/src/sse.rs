use axum::response::sse::Event;
use bridge_core::event::{BridgeEvent, BridgeEventType};

/// Convert a BridgeEvent into an axum SSE Event.
pub fn to_sse_event(event: &BridgeEvent) -> Result<Event, serde_json::Error> {
    let event_name = match event.event_type {
        BridgeEventType::ConversationCreated => "conversation_created",
        BridgeEventType::MessageReceived => "message_received",
        BridgeEventType::ResponseStarted => "message_start",
        BridgeEventType::ResponseChunk => "content_delta",
        BridgeEventType::ResponseCompleted => "message_end",
        BridgeEventType::ToolCallStarted => "tool_call_start",
        BridgeEventType::ToolCallCompleted => "tool_call_result",
        BridgeEventType::ConversationEnded => "conversation_ended",
        BridgeEventType::AgentError => "error",
        BridgeEventType::TodoUpdated => "todo_updated",
        BridgeEventType::TurnCompleted => "turn_completed",
        BridgeEventType::ToolApprovalRequired => "tool_approval_required",
        BridgeEventType::ToolApprovalResolved => "tool_approval_resolved",
        BridgeEventType::ConversationCompacted => "conversation_compacted",
        BridgeEventType::BackgroundTaskCompleted => "background_task_completed",
        BridgeEventType::ReasoningDelta => "reasoning_delta",
        BridgeEventType::Done => "done",
        BridgeEventType::SubAgentStarted => "sub_agent_started",
        BridgeEventType::SubAgentCompleted => "sub_agent_completed",
    };

    let data = serde_json::to_string(event)?;
    Ok(Event::default().event(event_name).data(data))
}
