use axum::extract::{Path, State};
use axum::http::{HeaderMap, StatusCode};
use axum::response::IntoResponse;
use axum::Json;
use bridge_core::AgentDefinition;
use std::collections::HashMap;
use std::sync::Arc;

use crate::store::{MockStore, ReceivedWebhook};

/// GET /agents — list all agents.
pub async fn list_agents(State(store): State<Arc<MockStore>>) -> Json<Vec<AgentDefinition>> {
    Json(store.get_all_agents())
}

/// GET /agents/:id — get a single agent.
pub async fn get_agent(
    State(store): State<Arc<MockStore>>,
    Path(id): Path<String>,
) -> impl IntoResponse {
    match store.get_agent(&id) {
        Some(agent) => (StatusCode::OK, Json(serde_json::to_value(agent).unwrap())).into_response(),
        None => (
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"error": "agent not found"})),
        )
            .into_response(),
    }
}

/// POST /agents — create a new agent.
pub async fn create_agent(
    State(store): State<Arc<MockStore>>,
    Json(agent): Json<AgentDefinition>,
) -> impl IntoResponse {
    let version = store.create_agent(agent);
    (
        StatusCode::CREATED,
        Json(serde_json::json!({"status": "created", "version": version})),
    )
}

/// PUT /agents/:id — update an existing agent.
pub async fn update_agent(
    State(store): State<Arc<MockStore>>,
    Path(id): Path<String>,
    Json(agent): Json<AgentDefinition>,
) -> impl IntoResponse {
    match store.update_agent(&id, agent) {
        Some(version) => (
            StatusCode::OK,
            Json(serde_json::json!({"status": "updated", "version": version})),
        )
            .into_response(),
        None => (
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"error": "agent not found"})),
        )
            .into_response(),
    }
}

/// DELETE /agents/:id — delete an agent.
pub async fn delete_agent(
    State(store): State<Arc<MockStore>>,
    Path(id): Path<String>,
) -> impl IntoResponse {
    if store.delete_agent(&id) {
        (
            StatusCode::OK,
            Json(serde_json::json!({"status": "deleted"})),
        )
            .into_response()
    } else {
        (
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"error": "agent not found"})),
        )
            .into_response()
    }
}

/// GET /agents/:agent_id/skills/:skill_id — get a skill definition.
pub async fn get_skill(
    State(store): State<Arc<MockStore>>,
    Path((agent_id, skill_id)): Path<(String, String)>,
) -> impl IntoResponse {
    match store.get_agent(&agent_id) {
        Some(agent) => {
            if let Some(skill) = agent.skills.iter().find(|s| s.id == skill_id) {
                (StatusCode::OK, Json(serde_json::to_value(skill).unwrap())).into_response()
            } else {
                (
                    StatusCode::NOT_FOUND,
                    Json(serde_json::json!({"error": "skill not found"})),
                )
                    .into_response()
            }
        }
        None => (
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"error": "agent not found"})),
        )
            .into_response(),
    }
}

/// POST /webhooks/receive — log a received webhook.
pub async fn receive_webhook(
    State(store): State<Arc<MockStore>>,
    headers: HeaderMap,
    Json(body): Json<serde_json::Value>,
) -> impl IntoResponse {
    let mut header_map = HashMap::new();
    for (name, value) in headers.iter() {
        if let Ok(v) = value.to_str() {
            header_map.insert(name.to_string(), v.to_string());
        }
    }

    store.push_webhook(ReceivedWebhook {
        timestamp: chrono::Utc::now(),
        headers: header_map,
        body,
    });

    (
        StatusCode::OK,
        Json(serde_json::json!({"status": "received"})),
    )
}

/// GET /webhooks/log — get all received webhooks.
pub async fn get_webhook_log(State(store): State<Arc<MockStore>>) -> Json<Vec<ReceivedWebhook>> {
    Json(store.get_all_webhooks())
}

/// DELETE /webhooks/log — clear all received webhooks.
pub async fn clear_webhook_log(State(store): State<Arc<MockStore>>) -> impl IntoResponse {
    store.clear_webhooks();
    (
        StatusCode::OK,
        Json(serde_json::json!({"status": "cleared"})),
    )
}
