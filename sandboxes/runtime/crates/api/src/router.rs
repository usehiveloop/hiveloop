use axum::middleware as axum_mw;
use axum::routing::{delete, get, patch, post, put};
use axum::Router;
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;

use crate::handlers::{agents, conversations, health, metrics, permissions, push, stream};
use crate::middleware::bearer_auth;
use crate::state::AppState;

/// Build the axum router with all routes and middleware.
pub fn build_router(state: AppState) -> Router {
    // Push routes — authenticated via bearer token
    let push_routes = Router::new()
        .route("/push/agents", post(push::push_agents))
        .route("/push/agents/{agent_id}", put(push::upsert_agent))
        .route("/push/agents/{agent_id}", delete(push::remove_agent))
        .route(
            "/push/agents/{agent_id}/conversations",
            post(push::hydrate_conversations),
        )
        .route(
            "/push/agents/{agent_id}/api-key",
            patch(push::update_agent_api_key),
        )
        .route("/push/diff", post(push::push_diff))
        .layer(axum_mw::from_fn_with_state(state.clone(), bearer_auth));

    Router::new()
        // Health
        .route("/health", get(health::health))
        // Agents
        .route("/agents", get(agents::list_agents))
        .route("/agents/{agent_id}", get(agents::get_agent))
        // Conversations
        .route(
            "/agents/{agent_id}/conversations",
            post(conversations::create_conversation),
        )
        .route(
            "/conversations/{conv_id}/messages",
            post(conversations::send_message),
        )
        .route(
            "/conversations/{conv_id}",
            delete(conversations::end_conversation),
        )
        .route(
            "/conversations/{conv_id}/abort",
            post(conversations::abort_conversation),
        )
        // Tool approvals
        .route(
            "/agents/{agent_id}/conversations/{conv_id}/approvals",
            get(permissions::list_approvals),
        )
        .route(
            "/agents/{agent_id}/conversations/{conv_id}/approvals",
            post(permissions::bulk_resolve_approvals),
        )
        .route(
            "/agents/{agent_id}/conversations/{conv_id}/approvals/{request_id}",
            post(permissions::resolve_approval),
        )
        // SSE streaming
        .route(
            "/conversations/{conv_id}/stream",
            get(stream::stream_conversation),
        )
        // Metrics
        .route("/metrics", get(metrics::get_metrics))
        // Push (authenticated)
        .merge(push_routes)
        // Middleware
        .layer(TraceLayer::new_for_http())
        .layer(CorsLayer::permissive())
        .with_state(state)
}
