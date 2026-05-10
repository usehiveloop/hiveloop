use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::IntoResponse,
    Json,
};
use chrono::{DateTime, Utc};
use domain::{AgentDefinition, SessionId, SessionStatus};
use serde::{Deserialize, Serialize};

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
