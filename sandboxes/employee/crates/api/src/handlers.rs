use agent::cloud_agents::CloudAgentEvent;
use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::IntoResponse,
    Json,
};
use chrono::{DateTime, Utc};
use domain::{AgentDefinition, EventKind, SessionId, SessionStatus};
use serde::{Deserialize, Serialize};
use serde_json::{json, Map, Value};

use crate::http_gateway::{stream_response, HttpMessageRequest};
use crate::state::ApiState;

#[derive(Serialize)]
pub struct ConfigResponse {
    applied_at: DateTime<Utc>,
    definition: AgentDefinition,
}

pub async fn put_config(
    State(state): State<ApiState>,
    Json(definition): Json<AgentDefinition>,
) -> Result<Json<ConfigResponse>, (StatusCode, String)> {
    state
        .config_repo
        .upsert(&definition)
        .await
        .map_err(|error| {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("persist: {error}"),
            )
        })?;
    state.skill_writer.sync(&definition.skills);
    if let Some(registry) = state.mcp_registry.as_ref() {
        registry.reload_from_specs(&definition.mcp_servers).await;
    }
    if let Some(reloader) = state.outbound_reloader.as_ref() {
        reloader
            .reload_outbound_channels(&definition.outbound_channels)
            .await
            .map_err(|error| {
                (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    format!("reload outbound channels: {error}"),
                )
            })?;
    }
    state.config_store.replace(definition.clone());
    state.mark_config_loaded();
    Ok(Json(ConfigResponse {
        applied_at: Utc::now(),
        definition,
    }))
}

pub async fn get_config(State(state): State<ApiState>) -> Json<AgentDefinition> {
    let snapshot = state.config_store.snapshot();
    Json((*snapshot).clone())
}

#[derive(Deserialize)]
pub struct ListSessionsParams {
    pub cursor: Option<String>,
    pub status: Option<String>,
    pub limit: Option<u32>,
}

#[derive(Serialize)]
pub struct ListSessionsResponse {
    pub items: Vec<domain::Session>,
    pub next_cursor: Option<String>,
}

pub async fn list_sessions(
    State(state): State<ApiState>,
    Query(params): Query<ListSessionsParams>,
) -> Result<Json<ListSessionsResponse>, (StatusCode, String)> {
    let cursor = params
        .cursor
        .as_deref()
        .map(parse_cursor)
        .transpose()
        .map_err(|error| (StatusCode::BAD_REQUEST, error))?;
    let status = params
        .status
        .as_deref()
        .map(parse_status)
        .transpose()
        .map_err(|error| (StatusCode::BAD_REQUEST, error))?;
    let limit = params.limit.unwrap_or(50).clamp(1, 200);

    let sessions = state
        .session_repo
        .list(cursor, status, limit)
        .await
        .map_err(|error| (StatusCode::INTERNAL_SERVER_ERROR, format!("list: {error}")))?;

    let next_cursor = sessions
        .last()
        .map(|session| session.last_activity_at.to_rfc3339());

    Ok(Json(ListSessionsResponse {
        items: sessions,
        next_cursor,
    }))
}

#[derive(Serialize)]
pub struct SessionDetailResponse {
    pub session: domain::Session,
    pub events: Vec<domain::SessionEvent>,
}

pub async fn get_session_detail(
    State(state): State<ApiState>,
    Path((channel, thread_ts)): Path<(String, String)>,
) -> Result<Json<SessionDetailResponse>, StatusCode> {
    let session_id = SessionId::from_slack(&channel, &thread_ts);
    let session = state
        .session_repo
        .get(&session_id)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?
        .ok_or(StatusCode::NOT_FOUND)?;
    let events = state
        .event_repo
        .list_recent(&session_id, 100)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    Ok(Json(SessionDetailResponse { session, events }))
}

pub async fn healthz() -> impl IntoResponse {
    StatusCode::OK
}

pub async fn readyz(State(state): State<ApiState>) -> impl IntoResponse {
    if state.is_ready() {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    }
}

pub async fn post_http_message(
    State(state): State<ApiState>,
    Json(request): Json<HttpMessageRequest>,
) -> Result<Json<crate::http_gateway::HttpMessageResponse>, (StatusCode, String)> {
    let Some(http_gateway) = state.http_gateway.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "http gateway is not enabled".to_string(),
        ));
    };
    http_gateway
        .inject_message(request)
        .await
        .map(Json)
        .map_err(|error| {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("inject message: {error}"),
            )
        })
}

#[derive(Debug, Deserialize)]
pub struct CloudAgentCallbackRequest {
    pub task_id: String,
    pub agent_id: String,
    pub event_type: String,
    pub event_id: String,
    pub timestamp: DateTime<Utc>,
    pub metadata: Map<String, Value>,
    pub data: Value,
}

#[derive(Debug, Serialize)]
pub struct CloudAgentCallbackResponse {
    pub accepted: bool,
    pub duplicate: bool,
    pub session_id: Option<String>,
}

pub async fn post_cloud_agent_callback(
    State(state): State<ApiState>,
    Json(request): Json<CloudAgentCallbackRequest>,
) -> Result<(StatusCode, Json<CloudAgentCallbackResponse>), (StatusCode, String)> {
    validate_cloud_agent_callback(&request)?;

    let session_id = resolve_cloud_callback_session(&state, &request)
        .await
        .ok_or_else(|| {
            (
                StatusCode::NOT_FOUND,
                format!("no session route found for task `{}`", request.task_id),
            )
        })?;

    if state
        .session_repo
        .get(&session_id)
        .await
        .map_err(|error| {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("load session: {error}"),
            )
        })?
        .is_none()
    {
        return Err((
            StatusCode::NOT_FOUND,
            format!("session `{session_id}` not found"),
        ));
    }

    let inserted = persist_and_publish_cloud_agent_callback(&state, &session_id, &request).await?;
    if !inserted {
        return Ok(cloud_agent_duplicate_response(&session_id));
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

pub async fn get_http_stream(
    State(state): State<ApiState>,
    Path(stream_id): Path<String>,
) -> Result<impl IntoResponse, StatusCode> {
    let Some(http_gateway) = state.http_gateway.as_ref() else {
        return Err(StatusCode::SERVICE_UNAVAILABLE);
    };
    stream_response(http_gateway.broker.clone(), stream_id)
        .await
        .map(IntoResponse::into_response)
        .ok_or(StatusCode::NOT_FOUND)
}

async fn resolve_cloud_callback_session(
    state: &ApiState,
    request: &CloudAgentCallbackRequest,
) -> Option<SessionId> {
    if let Some(session_id) = session_id_from_metadata(&request.metadata) {
        return Some(session_id);
    }
    let index = state.cloud_task_index.as_ref()?;
    let task = index.resolve_task(&request.task_id).await?;
    session_id_from_metadata(&task.metadata)
}

async fn persist_and_publish_cloud_agent_callback(
    state: &ApiState,
    session_id: &SessionId,
    request: &CloudAgentCallbackRequest,
) -> Result<bool, (StatusCode, String)> {
    let status = cloud_agent_event_status(request);
    let payload = cloud_agent_event_payload(session_id, request, status);

    let inserted_id = state
        .event_repo
        .append_idempotent(
            session_id,
            EventKind::CloudAgentEvent,
            payload.clone(),
            &cloud_callback_idempotency_key(request),
        )
        .await
        .map_err(|error| {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("append session event: {error}"),
            )
        })?;
    if inserted_id.is_none() {
        return Ok(false);
    }

    state
        .session_repo
        .touch(session_id, request.timestamp)
        .await
        .map_err(|error| {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("touch session: {error}"),
            )
        })?;

    if let Some(status) = status {
        state
            .session_repo
            .set_status(session_id, status)
            .await
            .map_err(|error| {
                (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    format!("set session status: {error}"),
                )
            })?;
    }

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

    if let Some(http_gateway) = state.http_gateway.as_ref() {
        if let Some(stream_id) = http_gateway
            .broker
            .stream_id_for_session(session_id.as_str())
            .await
        {
            http_gateway
                .broker
                .publish(&stream_id, "cloud_agent_event", payload)
                .await;
        }
    }

    Ok(true)
}

fn cloud_agent_duplicate_response(
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

fn validate_cloud_agent_callback(
    request: &CloudAgentCallbackRequest,
) -> Result<(), (StatusCode, String)> {
    for (field, value) in [
        ("task_id", request.task_id.as_str()),
        ("agent_id", request.agent_id.as_str()),
        ("event_type", request.event_type.as_str()),
        ("event_id", request.event_id.as_str()),
    ] {
        if value.trim().is_empty() {
            return Err((StatusCode::BAD_REQUEST, format!("{field} is required")));
        }
    }
    Ok(())
}

fn session_id_from_metadata(metadata: &Map<String, Value>) -> Option<SessionId> {
    metadata
        .get("session_id")
        .and_then(Value::as_str)
        .filter(|value| !value.trim().is_empty())
        .map(SessionId::from)
        .or_else(|| {
            let channel = metadata.get("channel").and_then(Value::as_str)?;
            let thread_ts = metadata.get("thread_ts").and_then(Value::as_str)?;
            if channel.trim().is_empty() || thread_ts.trim().is_empty() {
                return None;
            }
            Some(SessionId::from_slack(channel, thread_ts))
        })
}

fn cloud_agent_event_payload(
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

fn cloud_agent_event_status(request: &CloudAgentCallbackRequest) -> Option<SessionStatus> {
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

fn session_status_name(status: SessionStatus) -> &'static str {
    match status {
        SessionStatus::Active => "active",
        SessionStatus::Completed => "completed",
        SessionStatus::Errored => "errored",
    }
}

fn cloud_callback_idempotency_key(request: &CloudAgentCallbackRequest) -> String {
    format!(
        "cloud-agent-callback:{}:{}",
        request.task_id, request.event_id
    )
}

fn parse_cursor(raw: &str) -> Result<DateTime<Utc>, String> {
    DateTime::parse_from_rfc3339(raw)
        .map(|dt| dt.with_timezone(&Utc))
        .map_err(|error| format!("invalid cursor `{raw}`: {error}"))
}

fn parse_status(raw: &str) -> Result<SessionStatus, String> {
    match raw {
        "active" => Ok(SessionStatus::Active),
        "completed" => Ok(SessionStatus::Completed),
        "errored" => Ok(SessionStatus::Errored),
        other => Err(format!("invalid status `{other}`")),
    }
}
