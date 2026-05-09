mod auth;
mod handlers;
mod state;

use std::net::SocketAddr;

use axum::{
    routing::{get, put},
    Router,
};
use tokio::sync::oneshot;
use tracing::{info, warn};

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
