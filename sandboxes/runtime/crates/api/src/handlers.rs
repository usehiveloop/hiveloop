use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::IntoResponse,
    Json,
};
use chrono::{DateTime, Utc};
use domain::{AgentDefinition, SessionId, SessionStatus};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use storage::{SessionListCursor, SessionListFilter};
use tracing::warn;

use crate::http_gateway::{stream_response, HttpMessageRequest, HttpMessageResponse};
use crate::state::ApiState;

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ConfigResponse {
    applied_at: DateTime<Utc>,
    definition: AgentDefinition,
}

const FORBIDDEN_RUNTIME_ENV_KEYS: &[&str] = &[
    "GROQ_API_KEY",
    "OPENROUTER_API_KEY",
    "OPENAI_API_KEY",
    "TOGETHER_API_KEY",
];
const MAX_RUNTIME_ENV_KEYS: usize = 128;
const MAX_RUNTIME_ENV_KEY_LENGTH: usize = 128;
const MAX_RUNTIME_ENV_VALUE_LENGTH: usize = 8192;
const MAX_RUNTIME_ENV_PAYLOAD_BYTES: usize = 64 * 1024;

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RuntimeEnvUpdate {
    #[serde(flatten)]
    pub entries: HashMap<String, String>,
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RuntimeEnvResponse {
    applied_at: DateTime<Utc>,
    key_count: usize,
}

impl RuntimeEnvUpdate {
    fn validate(&self) -> Result<(), String> {
        if self.entries.len() > MAX_RUNTIME_ENV_KEYS {
            return Err(format!(
                "too many environment variables; max {}",
                MAX_RUNTIME_ENV_KEYS
            ));
        }

        for (key, value) in &self.entries {
            if !is_valid_env_key(key) {
                return Err(format!("invalid environment key: {key}"));
            }
            if FORBIDDEN_RUNTIME_ENV_KEYS
                .binary_search(&key.as_str())
                .is_ok()
            {
                return Err(format!("forbidden environment key: {key}"));
            }
            if value.len() > MAX_RUNTIME_ENV_VALUE_LENGTH {
                return Err(format!(
                    "environment value too long for {key}; max {} chars",
                    MAX_RUNTIME_ENV_VALUE_LENGTH
                ));
            }
        }

        let payload_size = self.estimated_payload_bytes();
        if payload_size > MAX_RUNTIME_ENV_PAYLOAD_BYTES {
            return Err("runtime env payload too large".to_string());
        }

        Ok(())
    }

    fn estimated_payload_bytes(&self) -> usize {
        std::mem::size_of::<usize>()
            + self
                .entries
                .iter()
                .map(|(key, value)| key.len() + value.len())
                .sum::<usize>()
    }
}

fn is_valid_env_key(key: &str) -> bool {
    if key.is_empty() || key.len() > MAX_RUNTIME_ENV_KEY_LENGTH {
        return false;
    }
    let mut chars = key.chars();
    if let Some(first) = chars.next() {
        if !(first == '_' || first.is_ascii_alphabetic()) {
            return false;
        }
    }
    chars.all(|ch| ch == '_' || ch.is_ascii_alphanumeric())
}

fn redacted_env_keys(entries: &HashMap<String, String>) -> Vec<&str> {
    entries.keys().map(|key| key.as_str()).collect()
}

#[cfg_attr(feature = "openapi", utoipa::path(
    put,
    path = "/config",
    request_body = AgentDefinition,
    responses(
        (status = 200, description = "Configuration applied", body = ConfigResponse),
        (status = 500, description = "Failed to persist or apply configuration")
    ),
    security(("bearer" = []))
))]
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

#[cfg_attr(feature = "openapi", utoipa::path(
    put,
    path = "/config/env",
    request_body = RuntimeEnvUpdate,
    responses(
        (status = 200, description = "Environment override applied", body = RuntimeEnvResponse),
        (status = 400, description = "Invalid environment payload"),
        (status = 413, description = "Payload too large"),
        (status = 500, description = "Failed to apply environment overrides")
    ),
    security(("bearer" = []))
))]
pub async fn put_runtime_env(
    State(state): State<ApiState>,
    Json(overrides): Json<RuntimeEnvUpdate>,
) -> Result<Json<RuntimeEnvResponse>, (StatusCode, String)> {
    if let Err(error) = overrides.validate() {
        warn!(
            error = %error,
            keys = ?redacted_env_keys(&overrides.entries),
            key_count = overrides.entries.len(),
            payload_size = overrides.estimated_payload_bytes(),
            "runtime env update rejected"
        );
        let status = if error.contains("payload too large") {
            StatusCode::PAYLOAD_TOO_LARGE
        } else {
            StatusCode::BAD_REQUEST
        };
        return Err((status, error));
    }

    state
        .config_store
        .set_runtime_env(overrides.entries.clone());

    Ok(Json(RuntimeEnvResponse {
        applied_at: Utc::now(),
        key_count: overrides.entries.len(),
    }))
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/config",
    responses(
        (status = 200, description = "Current agent configuration", body = AgentDefinition)
    ),
    security(("bearer" = []))
))]
pub async fn get_config(State(state): State<ApiState>) -> Json<AgentDefinition> {
    let snapshot = state.config_store.snapshot();
    Json((*snapshot).clone())
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ListSessionsParams {
    pub cursor: Option<String>,
    pub status: Option<String>,
    pub limit: Option<u32>,
    pub session_id: Option<String>,
    pub channel: Option<String>,
    pub thread_ts: Option<String>,
    pub agent_session_id: Option<String>,
    pub q: Option<String>,
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ListSessionsResponse {
    pub items: Vec<domain::Session>,
    pub next_cursor: Option<String>,
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/sessions",
    params(
        ("cursor" = Option<String>, Query, description = "RFC 3339 cursor from the previous page"),
        ("status" = Option<String>, Query, description = "Session status filter: active, completed, or errored"),
        ("session_id" = Option<String>, Query, description = "Exact session ID filter"),
        ("channel" = Option<String>, Query, description = "Exact channel filter"),
        ("thread_ts" = Option<String>, Query, description = "Exact thread timestamp filter"),
        ("agent_session_id" = Option<String>, Query, description = "Exact agent session ID filter"),
        ("q" = Option<String>, Query, description = "Prefix search over session identifiers"),
        ("limit" = Option<u32>, Query, description = "Maximum sessions to return, clamped from 1 to 200")
    ),
    responses(
        (status = 200, description = "Sessions page", body = ListSessionsResponse),
        (status = 400, description = "Invalid cursor or status"),
        (status = 500, description = "Failed to list sessions")
    ),
    security(("bearer" = []))
))]
pub async fn list_sessions(
    State(state): State<ApiState>,
    Query(params): Query<ListSessionsParams>,
) -> Result<Json<ListSessionsResponse>, (StatusCode, String)> {
    let cursor = params
        .cursor
        .as_deref()
        .map(parse_session_cursor)
        .transpose()
        .map_err(|error| (StatusCode::BAD_REQUEST, error))?;
    let status = params
        .status
        .as_deref()
        .map(parse_status)
        .transpose()
        .map_err(|error| (StatusCode::BAD_REQUEST, error))?;
    let limit = params.limit.unwrap_or(50).clamp(1, 200);

    let mut sessions = state
        .session_repo
        .list(
            SessionListFilter {
                cursor,
                status,
                session_id: clean_optional(params.session_id),
                channel: clean_optional(params.channel),
                thread_ts: clean_optional(params.thread_ts),
                agent_session_id: clean_optional(params.agent_session_id),
                search: clean_optional(params.q),
            },
            limit + 1,
        )
        .await
        .map_err(|error| (StatusCode::INTERNAL_SERVER_ERROR, format!("list: {error}")))?;

    let has_more = sessions.len() > limit as usize;
    if has_more {
        sessions.truncate(limit as usize);
    }
    let next_cursor = if has_more {
        sessions
            .last()
            .map(|session| encode_session_cursor(session.last_activity_at, session.id.as_str()))
    } else {
        None
    };

    Ok(Json(ListSessionsResponse {
        items: sessions,
        next_cursor,
    }))
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SessionDetailResponse {
    pub session: domain::Session,
    pub events: Vec<domain::SessionEvent>,
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/sessions/{session_id}",
    params(("session_id" = String, Path, description = "Session identifier")),
    responses(
        (status = 200, description = "Session details", body = SessionDetailResponse),
        (status = 404, description = "Session not found"),
        (status = 500, description = "Failed to load session details")
    ),
    security(("bearer" = []))
))]
pub async fn get_session_detail(
    State(state): State<ApiState>,
    Path(session_id): Path<String>,
) -> Result<Json<SessionDetailResponse>, StatusCode> {
    let session_id = SessionId::from(session_id);
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

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/healthz",
    responses(
        (status = 200, description = "Bridge process is alive")
    ),
    security(())
))]
pub async fn healthz() -> impl IntoResponse {
    StatusCode::OK
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/readyz",
    responses(
        (status = 200, description = "Bridge is ready"),
        (status = 503, description = "Bridge is not ready")
    ),
    security(("bearer" = []))
))]
pub async fn readyz(State(state): State<ApiState>) -> impl IntoResponse {
    if state.is_ready() {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    }
}

#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/gateway/http/messages",
    request_body = HttpMessageRequest,
    responses(
        (status = 200, description = "Message accepted and stream created", body = HttpMessageResponse),
        (status = 500, description = "Failed to inject message"),
        (status = 503, description = "HTTP gateway is not enabled")
    ),
    security(("bearer" = []))
))]
pub async fn post_http_message(
    State(state): State<ApiState>,
    Json(request): Json<HttpMessageRequest>,
) -> Result<Json<HttpMessageResponse>, (StatusCode, String)> {
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

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/gateway/http/streams/{stream_id}",
    params(("stream_id" = String, Path, description = "HTTP stream identifier")),
    responses(
        (status = 200, description = "Server-sent event stream", content_type = "text/event-stream"),
        (status = 404, description = "Stream not found"),
        (status = 503, description = "HTTP gateway is not enabled")
    ),
    security(("bearer" = []))
))]
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

fn parse_session_cursor(raw: &str) -> Result<SessionListCursor, String> {
    let (timestamp, id) = match raw.split_once('|') {
        Some((timestamp, id)) => (timestamp, Some(id.trim().to_string())),
        None => (raw, None),
    };
    Ok(SessionListCursor {
        last_activity_at: parse_cursor(timestamp)?,
        id: id.filter(|value| !value.is_empty()),
    })
}

fn encode_session_cursor(last_activity_at: DateTime<Utc>, id: &str) -> String {
    format!("{}|{}", last_activity_at.to_rfc3339(), id)
}

fn clean_optional(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}

fn parse_status(raw: &str) -> Result<SessionStatus, String> {
    match raw {
        "active" => Ok(SessionStatus::Active),
        "completed" => Ok(SessionStatus::Completed),
        "errored" => Ok(SessionStatus::Errored),
        other => Err(format!("invalid status `{other}`")),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    use std::collections::HashMap;

    #[test]
    fn runtime_env_update_accepts_valid_payload() {
        let update = RuntimeEnvUpdate {
            entries: HashMap::from([
                ("GOOD_KEY".to_string(), "value".to_string()),
                ("ANOTHER_KEY_1".to_string(), "another".to_string()),
            ]),
        };

        assert!(
            update.validate().is_ok(),
            "expected valid env payload to pass"
        );
    }

    #[test]
    fn runtime_env_update_rejects_invalid_key() {
        let update = RuntimeEnvUpdate {
            entries: HashMap::from([("1BAD_KEY".to_string(), "value".to_string())]),
        };
        assert_eq!(
            update.validate().unwrap_err(),
            "invalid environment key: 1BAD_KEY"
        );
    }

    #[test]
    fn runtime_env_update_rejects_forbidden_key() {
        let update = RuntimeEnvUpdate {
            entries: HashMap::from([("OPENAI_API_KEY".to_string(), "value".to_string())]),
        };
        assert_eq!(
            update.validate().unwrap_err(),
            "forbidden environment key: OPENAI_API_KEY"
        );
    }

    #[test]
    fn runtime_env_update_rejects_too_many_keys() {
        let mut entries = HashMap::new();
        for i in 0..=MAX_RUNTIME_ENV_KEYS {
            entries.insert(format!("KEY_{i}"), "value".to_string());
        }

        let update = RuntimeEnvUpdate { entries };
        assert_eq!(
            update.validate().unwrap_err(),
            format!(
                "too many environment variables; max {}",
                MAX_RUNTIME_ENV_KEYS
            )
        );
    }

    #[test]
    fn runtime_env_update_rejects_long_environment_value() {
        let update = RuntimeEnvUpdate {
            entries: HashMap::from([("VALUE_TOO_LONG".to_string(), "x".repeat(8193))]),
        };

        assert_eq!(
            update.validate().unwrap_err(),
            format!(
                "environment value too long for VALUE_TOO_LONG; max {} chars",
                MAX_RUNTIME_ENV_VALUE_LENGTH
            )
        );
    }

    #[test]
    fn runtime_env_update_rejects_oversize_payload() {
        let mut entries = HashMap::new();
        for i in 0..9 {
            entries.insert(format!("OVERSIZE_PAYLOAD_{i}"), "x".repeat(8192));
        }
        let update = RuntimeEnvUpdate { entries };

        assert_eq!(
            update.validate().unwrap_err(),
            "runtime env payload too large"
        );
    }

    #[test]
    fn runtime_env_update_accepts_empty_payload() {
        let update = RuntimeEnvUpdate {
            entries: HashMap::new(),
        };
        assert!(
            update.validate().is_ok(),
            "empty payload should be accepted as clear overlay"
        );
    }
}
