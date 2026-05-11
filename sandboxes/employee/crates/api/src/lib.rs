mod auth;
mod handlers;
mod http_gateway;
mod observability_handlers;
mod state;

use std::net::SocketAddr;

use axum::{
    routing::{get, post, put},
    Router,
};
use tokio::sync::oneshot;
use tracing::{info, warn};

pub use http_gateway::{HttpGatewayState, HttpStreamBroker, HttpStreamEvent};
pub use state::ApiState;

pub fn build_router(state: ApiState) -> Router {
    Router::new()
        .route(
            "/config",
            put(handlers::put_config).get(handlers::get_config),
        )
        .route("/sessions", get(handlers::list_sessions))
        .route(
            "/sessions/:channel/:thread_ts",
            get(handlers::get_session_detail),
        )
        .route("/healthz", get(handlers::healthz))
        .route("/readyz", get(handlers::readyz))
        .route("/debug/sentry-test", post(handlers::post_sentry_test))
        .route("/gateway/http/messages", post(handlers::post_http_message))
        .route(
            "/gateway/cloud-agents/callback",
            post(handlers::post_cloud_agent_callback),
        )
        .route(
            "/gateway/http/streams/:stream_id",
            get(handlers::get_http_stream),
        )
        .route(
            "/observability/traces/:trace_id/events",
            get(observability_handlers::get_trace_events),
        )
        .route(
            "/observability/traces/:trace_id/summary",
            get(observability_handlers::get_trace_summary),
        )
        .layer(axum::middleware::from_fn_with_state(
            state.clone(),
            auth::bearer_auth,
        ))
        .with_state(state)
}

pub async fn serve(
    bind_addr: SocketAddr,
    state: ApiState,
) -> (tokio::task::JoinHandle<()>, oneshot::Sender<()>) {
    let (cancel_signal, cancel_receiver) = oneshot::channel::<()>();
    let router = build_router(state);
    let handle = tokio::spawn(async move {
        match tokio::net::TcpListener::bind(bind_addr).await {
            Ok(listener) => {
                info!(%bind_addr, "control-plane HTTP server listening");
                let result = axum::serve(listener, router)
                    .with_graceful_shutdown(async move {
                        let _ = cancel_receiver.await;
                    })
                    .await;
                if let Err(error) = result {
                    warn!(%error, "control-plane HTTP server exited with error");
                }
            }
            Err(error) => {
                warn!(%bind_addr, %error, "control-plane HTTP bind failed");
            }
        }
    });
    (handle, cancel_signal)
}
