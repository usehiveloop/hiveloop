use agent::cloud_agents::CloudAgentEvent;
use axum::{extract::State, http::StatusCode, Json};
use chrono::{DateTime, Utc};
use domain::{EventKind, Session, SessionId, SessionStatus};
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

use crate::cloud_agent_callback_payload::{
    duplicate_response, event_payload, event_status, idempotency_key, validate,
};
use crate::state::ApiState;

#[derive(Debug, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CloudAgentCallbackRequest {
    pub task_id: String,
    pub agent_id: String,
    pub session_id: String,
    pub event_type: String,
    pub event_id: String,
    pub timestamp: DateTime<Utc>,
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub metadata: Map<String, Value>,
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub data: Value,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CloudAgentCallbackResponse {
    pub accepted: bool,
    pub duplicate: bool,
    pub session_id: Option<String>,
}

fn capture_failure(
    phase: &'static str,
    request: &CloudAgentCallbackRequest,
    status: StatusCode,
    message: &str,
) {
    let sentry_error = anyhow::anyhow!("cloud agent callback {phase}: {status}");
    sentry::with_scope(
        |scope| {
            scope.set_level(Some(if status.is_server_error() {
                sentry::Level::Error
            } else {
                sentry::Level::Warning
            }));
            scope.set_tag("service", "employee-bridge");
            scope.set_tag("feature", "cloud_agents");
            scope.set_tag("cloud_agent.operation", "callback");
            scope.set_tag("cloud_agent.phase", phase);
            scope.set_tag("http.status_code", status.as_u16().to_string());
            set_non_empty_tag(scope, "cloud_agent.task_id", &request.task_id);
            set_non_empty_tag(scope, "cloud_agent_id", &request.agent_id);
            set_non_empty_tag(scope, "cloud_agent.event_type", &request.event_type);
            set_non_empty_tag(scope, "cloud_agent.event_id", &request.event_id);
            set_non_empty_tag(scope, "employee.session_id", &request.session_id);
            scope.set_extra("reason", message.to_string().into());
        },
        || sentry::capture_error(sentry_error.root_cause()),
    );
}

fn set_non_empty_tag(scope: &mut sentry::Scope, key: &'static str, value: &str) {
    if !value.trim().is_empty() {
        scope.set_tag(key, value.to_string());
    }
}

#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/gateway/cloud-agents/callback",
    request_body = CloudAgentCallbackRequest,
    responses(
        (status = 202, description = "Callback accepted", body = CloudAgentCallbackResponse),
        (status = 200, description = "Duplicate callback ignored", body = CloudAgentCallbackResponse),
        (status = 400, description = "Invalid callback payload"),
        (status = 404, description = "No matching session route"),
        (status = 502, description = "Failed to deliver callback"),
        (status = 500, description = "Failed to persist callback")
    ),
    security(("bearer" = []))
))]
pub async fn post_cloud_agent_callback(
    State(state): State<ApiState>,
    Json(request): Json<CloudAgentCallbackRequest>,
) -> Result<(StatusCode, Json<CloudAgentCallbackResponse>), (StatusCode, String)> {
    validate(&request).map_err(|error| {
        capture_failure("validate_callback", &request, error.0, &error.1);
        error
    })?;

    let session_id = SessionId::from(request.session_id.trim());
    let session = state.session_repo.get(&session_id).await.map_err(|error| {
        let message = format!("load session: {error}");
        capture_failure(
            "load_session",
            &request,
            StatusCode::INTERNAL_SERVER_ERROR,
            &message,
        );
        (StatusCode::INTERNAL_SERVER_ERROR, message)
    })?;
    let Some(session) = session else {
        let message = format!("session `{session_id}` not found");
        capture_failure(
            "session_not_found",
            &request,
            StatusCode::NOT_FOUND,
            &message,
        );
        return Err((StatusCode::NOT_FOUND, message));
    };

    if !persist_and_deliver(&state, &session, &request).await? {
        return Ok(duplicate_response(&session_id));
    }
    Ok((
        StatusCode::ACCEPTED,
        Json(CloudAgentCallbackResponse {
            accepted: true,
            duplicate: false,
            session_id: Some(session_id.as_str().to_string()),
        }),
    ))
}

async fn persist_and_deliver(
    state: &ApiState,
    session: &Session,
    request: &CloudAgentCallbackRequest,
) -> Result<bool, (StatusCode, String)> {
    let session_id = &session.id;
    let status = event_status(request);
    if matches!(status, Some(SessionStatus::Errored)) {
        capture_failure(
            "agent_error_event",
            request,
            StatusCode::INTERNAL_SERVER_ERROR,
            "cloud agent reported an error event",
        );
    }
    let payload = event_payload(session_id, request, status);
    let inserted_id = state
        .event_repo
        .append_idempotent(
            session_id,
            EventKind::CloudAgentEvent,
            payload.clone(),
            &idempotency_key(request),
        )
        .await
        .map_err(|error| {
            let message = format!("append session event: {error}");
            capture_failure(
                "append_session_event",
                request,
                StatusCode::INTERNAL_SERVER_ERROR,
                &message,
            );
            (StatusCode::INTERNAL_SERVER_ERROR, message)
        })?;
    if inserted_id.is_none() {
        return Ok(false);
    }

    touch_session(state, session_id, request, status).await?;
    if let Some(index) = state.cloud_task_index.as_ref() {
        index
            .append_event(
                &request.task_id,
                CloudAgentEvent {
                    event_type: request.event_type.clone(),
                    created_at: Some(request.timestamp.to_rfc3339()),
                    data: request.data.clone(),
                },
            )
            .await;
    }
    deliver(state, session, payload, request).await?;
    Ok(true)
}

async fn touch_session(
    state: &ApiState,
    session_id: &SessionId,
    request: &CloudAgentCallbackRequest,
    status: Option<SessionStatus>,
) -> Result<(), (StatusCode, String)> {
    state
        .session_repo
        .touch(session_id, request.timestamp)
        .await
        .map_err(|error| {
            let message = format!("touch session: {error}");
            capture_failure(
                "touch_session",
                request,
                StatusCode::INTERNAL_SERVER_ERROR,
                &message,
            );
            (StatusCode::INTERNAL_SERVER_ERROR, message)
        })?;
    if let Some(status) = status {
        state
            .session_repo
            .set_status(session_id, status)
            .await
            .map_err(|error| {
                let message = format!("set session status: {error}");
                capture_failure(
                    "set_session_status",
                    request,
                    StatusCode::INTERNAL_SERVER_ERROR,
                    &message,
                );
                (StatusCode::INTERNAL_SERVER_ERROR, message)
            })?;
    }
    Ok(())
}

async fn deliver(
    state: &ApiState,
    session: &Session,
    payload: Value,
    request: &CloudAgentCallbackRequest,
) -> Result<(), (StatusCode, String)> {
    let session_id = &session.id;
    if session_id.as_str().starts_with("http-") {
        return deliver_http(state, session_id, payload, request).await;
    }
    let Some(deliverer) = state.cloud_agent_callback_deliverer.as_ref() else {
        return delivery_error(
            request,
            "cloud agent callback deliverer is not enabled".to_string(),
        );
    };
    deliverer
        .deliver_cloud_agent_callback(session_id, payload)
        .await
        .map_err(|error| {
            let message = format!("deliver callback: {error}");
            capture_failure(
                "deliver_callback",
                request,
                StatusCode::BAD_GATEWAY,
                &message,
            );
            (StatusCode::BAD_GATEWAY, message)
        })
}

async fn deliver_http(
    state: &ApiState,
    session_id: &SessionId,
    payload: Value,
    request: &CloudAgentCallbackRequest,
) -> Result<(), (StatusCode, String)> {
    let Some(http_gateway) = state.http_gateway.as_ref() else {
        return delivery_error(request, "http gateway is not enabled".to_string());
    };
    let Some(stream_id) = http_gateway
        .broker
        .stream_id_for_session(session_id.as_str())
        .await
    else {
        return delivery_error(
            request,
            format!("no active http stream for session `{session_id}`"),
        );
    };
    http_gateway
        .broker
        .publish(&stream_id, "cloud_agent_event", payload)
        .await;
    Ok(())
}

fn delivery_error<T>(
    request: &CloudAgentCallbackRequest,
    message: String,
) -> Result<T, (StatusCode, String)> {
    capture_failure(
        "deliver_callback",
        request,
        StatusCode::BAD_GATEWAY,
        &message,
    );
    Err((StatusCode::BAD_GATEWAY, message))
}
