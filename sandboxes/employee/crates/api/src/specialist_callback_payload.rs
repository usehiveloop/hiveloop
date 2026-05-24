use axum::{http::StatusCode, Json};
use domain::{SessionId, SessionStatus};
use serde_json::{json, Value};

use crate::specialist_callbacks::{SpecialistCallbackRequest, SpecialistCallbackResponse};

pub(super) fn event_payload(
    session_id: &SessionId,
    request: &SpecialistCallbackRequest,
    status: Option<SessionStatus>,
) -> Value {
    json!({
        "source": "specialist_callback",
        "session_id": session_id.as_str(),
        "task_id": request.task_id,
        "specialist_id": request.specialist_id,
        "event_type": request.event_type,
        "event_id": request.event_id,
        "timestamp": request.timestamp,
        "metadata": request.metadata,
        "data": request.data,
        "status": status.map(session_status_name),
    })
}

pub(super) fn event_status(request: &SpecialistCallbackRequest) -> Option<SessionStatus> {
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

pub(super) fn idempotency_key(request: &SpecialistCallbackRequest) -> String {
    format!(
        "specialist-callback:{}:{}",
        request.task_id, request.event_id
    )
}

pub(super) fn validate(request: &SpecialistCallbackRequest) -> Result<(), (StatusCode, String)> {
    for (field, value) in [
        ("task_id", request.task_id.as_str()),
        ("specialist_id", request.specialist_id.as_str()),
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
) -> (StatusCode, Json<SpecialistCallbackResponse>) {
    (
        StatusCode::OK,
        Json(SpecialistCallbackResponse {
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
