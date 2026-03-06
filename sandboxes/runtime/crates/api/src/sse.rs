use axum::response::sse::Event;
use llm::SseEvent;

/// Convert an SseEvent into an axum SSE Event.
pub fn to_sse_event(event: &SseEvent) -> Result<Event, serde_json::Error> {
    let event_name = match event {
        SseEvent::MessageStart { .. } => "message_start",
        SseEvent::ContentDelta { .. } => "content_delta",
        SseEvent::ToolCallStart { .. } => "tool_call_start",
        SseEvent::ToolCallResult { .. } => "tool_call_result",
        SseEvent::MessageEnd { .. } => "message_end",
        SseEvent::TodoUpdated { .. } => "todo_updated",
        SseEvent::ToolApprovalRequired { .. } => "tool_approval_required",
        SseEvent::ToolApprovalResolved { .. } => "tool_approval_resolved",
        SseEvent::Error { .. } => "error",
        SseEvent::Done => "done",
    };

    let data = serde_json::to_string(event)?;
    Ok(Event::default().event(event_name).data(data))
}
