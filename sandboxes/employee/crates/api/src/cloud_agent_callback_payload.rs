use axum::{http::StatusCode, Json};
use domain::{SessionId, SessionStatus};
use serde_json::{json, Value};

use crate::cloud_agent_callbacks::{CloudAgentCallbackRequest, CloudAgentCallbackResponse};

pub(super) fn event_payload(
    session_id: &SessionId,
    request: &CloudAgentCallbackRequest,
    status: Option<SessionStatus>,
) -> Value {
    json!({
        "source": "cloud_agent_callback",
        "session_id": session_id.as_str(),
        "task_id": request.task_id,
        "agent_id": request.agent_id,
        "event_type": request.event_type,
        "event_id": request.event_id,
        "timestamp": request.timestamp,
        "metadata": request.metadata,
        "data": request.data,
        "status": status.map(session_status_name),
    })
}

pub(super) fn event_status(request: &CloudAgentCallbackRequest) -> Option<SessionStatus> {
    let event_type = request.event_type.to_ascii_lowercase();
    let data_status = request
        .data
        .get("status")
        .and_then(Value::as_str)
        .map(str::to_ascii_lowercase);
    if event_type.contains("error")
        || event_type.contains("failed")
        || data_status
            .as_deref()
            .is_some_and(|status| matches!(status, "error" | "errored" | "failed" | "failure"))
    {
        return Some(SessionStatus::Errored);
    }
    if matches!(
        event_type.as_str(),
        "conversationended" | "conversation_ended" | "done" | "completed" | "taskcompleted"
    ) || data_status
        .as_deref()
        .is_some_and(|status| matches!(status, "done" | "completed" | "complete" | "success"))
    {
        return Some(SessionStatus::Completed);
    }
    None
}

pub(super) fn idempotency_key(request: &CloudAgentCallbackRequest) -> String {
    format!(
        "cloud-agent-callback:{}:{}",
        request.task_id, request.event_id
    )
}

pub(super) fn validate(request: &CloudAgentCallbackRequest) -> Result<(), (StatusCode, String)> {
    for (field, value) in [
        ("task_id", request.task_id.as_str()),
        ("agent_id", request.agent_id.as_str()),
        ("session_id", request.session_id.as_str()),
        ("event_type", request.event_type.as_str()),
        ("event_id", request.event_id.as_str()),
    ] {
        if value.trim().is_empty() {
            return Err((StatusCode::BAD_REQUEST, format!("{field} is required")));
        }
    }
    Ok(())
}

pub(super) fn duplicate_response(
    session_id: &SessionId,
) -> (StatusCode, Json<CloudAgentCallbackResponse>) {
    (
        StatusCode::OK,
        Json(CloudAgentCallbackResponse {
            accepted: false,
            duplicate: true,
            session_id: Some(session_id.as_str().to_string()),
        }),
    )
}

fn session_status_name(status: SessionStatus) -> &'static str {
    match status {
        SessionStatus::Active => "active",
        SessionStatus::Completed => "completed",
        SessionStatus::Errored => "errored",
    }
}
