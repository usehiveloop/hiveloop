use axum::routing::{delete, get, post};
use axum::Router;
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;

use crate::handlers::{agents, conversations, health, metrics, stream};
use crate::state::AppState;

/// Build the axum router with all routes and middleware.
pub fn build_router(state: AppState) -> Router {
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
        // SSE streaming
        .route(
            "/conversations/{conv_id}/stream",
            get(stream::stream_conversation),
        )
        // Metrics
        .route("/metrics", get(metrics::get_metrics))
        // Middleware
        .layer(TraceLayer::new_for_http())
        .layer(CorsLayer::permissive())
        .with_state(state)
}
