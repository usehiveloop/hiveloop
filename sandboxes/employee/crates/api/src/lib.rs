mod auth;
mod cloud_agent_callback_payload;
mod cloud_agent_callbacks;
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
pub use state::{ApiState, CloudAgentCallbackDeliverer, OutboundConfigReloader};

#[cfg(feature = "openapi")]
mod openapi {
    use utoipa::OpenApi;

    #[derive(OpenApi)]
    #[openapi(
        info(title = "Employee Bridge API", version = "0.0.1"),
        paths(
            crate::handlers::put_config,
            crate::handlers::put_runtime_env,
            crate::handlers::get_config,
            crate::handlers::list_sessions,
            crate::handlers::get_session_detail,
            crate::handlers::healthz,
            crate::handlers::readyz,
            crate::handlers::post_http_message,
            crate::handlers::get_http_stream,
            crate::cloud_agent_callbacks::post_cloud_agent_callback,
            crate::observability_handlers::get_trace_events,
            crate::observability_handlers::get_trace_summary,
        ),
        components(schemas(
            domain::AgentDefinition,
            domain::AgentMeta,
            domain::PromptFragments,
            domain::PromptFragment,
            domain::Limits,
            domain::ContextConfig,
            domain::MemoryContextConfig,
            domain::MemoryContextEntry,
            domain::CompactionConfig,
            domain::ModelConfig,
            domain::ReasoningEffort,
            domain::ToolSpec,
            domain::BashConfig,
            domain::ReadFileConfig,
            domain::WriteFileConfig,
            domain::McpSpec,
            domain::ToolFilter,
            domain::SkillSpec,
            domain::SkillTrigger,
            domain::SubagentSpec,
            domain::SubagentDefinition,
            domain::SlackConfig,
            domain::AllowBotsMode,
            domain::ProgressiveMessages,
            domain::ThreadContextConfig,
            domain::OutboundEvent,
            domain::OutboundChannelSpec,
            domain::OutboundChannelKind,
            domain::Attachment,
            domain::LinkPreview,
            domain::HistoryMessage,
            domain::MessageHandle,
            domain::SessionId,
            domain::SessionStatus,
            domain::Session,
            domain::EventKind,
            domain::SessionEvent,
            domain::CronJobState,
            domain::CronJobSource,
            domain::CronJob,
            observability::ObservabilityEventType,
            observability::EventTimings,
            observability::ModelUsage,
            observability::ToolUsage,
            observability::ObservabilityEvent,
            observability::TraceSummary,
            crate::handlers::ConfigResponse,
            crate::handlers::ListSessionsParams,
            crate::handlers::ListSessionsResponse,
            crate::handlers::SessionDetailResponse,
            crate::cloud_agent_callbacks::CloudAgentCallbackRequest,
            crate::cloud_agent_callbacks::CloudAgentCallbackResponse,
            crate::http_gateway::HttpStreamEvent,
            crate::http_gateway::HttpMessageRequest,
            crate::http_gateway::HttpMessageResponse,
        )),
        modifiers(&SecurityAddon),
        security(("bearer" = []))
    )]
    pub struct ApiDoc;

    struct SecurityAddon;

    impl utoipa::Modify for SecurityAddon {
        fn modify(&self, openapi: &mut utoipa::openapi::OpenApi) {
            use utoipa::openapi::security::{HttpAuthScheme, HttpBuilder, SecurityScheme};

            if let Some(components) = openapi.components.as_mut() {
                components.add_security_scheme(
                    "bearer",
                    SecurityScheme::Http(
                        HttpBuilder::new()
                            .scheme(HttpAuthScheme::Bearer)
                            .bearer_format("runtime-secret")
                            .build(),
                    ),
                );
            }
        }
    }
}

#[cfg(feature = "openapi")]
pub use openapi::ApiDoc;

pub fn build_router(state: ApiState) -> Router {
    Router::new()
        .route(
            "/config",
            put(handlers::put_config).get(handlers::get_config),
        )
        .route("/config/env", put(handlers::put_runtime_env))
        .route("/sessions", get(handlers::list_sessions))
        .route(
            "/sessions/:channel/:thread_ts",
            get(handlers::get_session_detail),
        )
        .route("/healthz", get(handlers::healthz))
        .route("/readyz", get(handlers::readyz))
        .route("/gateway/http/messages", post(handlers::post_http_message))
        .route(
            "/gateway/cloud-agents/callback",
            post(cloud_agent_callbacks::post_cloud_agent_callback),
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

#[cfg(all(test, feature = "openapi"))]
mod openapi_tests {
    use super::ApiDoc;
    use std::collections::BTreeSet;
    use utoipa::OpenApi;

    #[test]
    fn openapi_paths_match_router_routes() {
        let spec = ApiDoc::openapi();
        let actual: BTreeSet<String> = spec.paths.paths.keys().cloned().collect();
        let expected = BTreeSet::from([
            "/config".to_string(),
            "/config/env".to_string(),
            "/gateway/cloud-agents/callback".to_string(),
            "/gateway/http/messages".to_string(),
            "/gateway/http/streams/{stream_id}".to_string(),
            "/healthz".to_string(),
            "/observability/traces/{trace_id}/events".to_string(),
            "/observability/traces/{trace_id}/summary".to_string(),
            "/readyz".to_string(),
            "/sessions".to_string(),
            "/sessions/{channel}/{thread_ts}".to_string(),
        ]);

        assert_eq!(actual, expected);
    }

    #[test]
    fn openapi_auth_documents_healthz_as_anonymous_only() {
        let spec = serde_json::to_value(ApiDoc::openapi()).expect("serialize OpenAPI spec");

        assert_eq!(spec["security"], serde_json::json!([{ "bearer": [] }]));
        assert_eq!(
            spec["paths"]["/healthz"]["get"]["security"],
            serde_json::json!([{}])
        );
        assert_eq!(
            spec["paths"]["/readyz"]["get"]["security"],
            serde_json::json!([{ "bearer": [] }])
        );
    }
}
