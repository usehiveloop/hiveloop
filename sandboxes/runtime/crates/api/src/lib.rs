pub mod handlers;
pub mod middleware;
pub mod router;
pub mod sse;
pub mod state;

#[cfg(test)]
mod tests;

pub use router::build_router;
pub use state::AppState;

#[cfg(feature = "openapi")]
mod openapi {
    use utoipa::OpenApi;

    #[derive(OpenApi)]
    #[openapi(
        info(title = "Portal Bridge API", version = "0.1.0"),
        paths(
            crate::handlers::health::health,
            crate::handlers::agents::list_agents,
            crate::handlers::agents::get_agent,
            crate::handlers::conversations::create_conversation,
            crate::handlers::conversations::send_message,
            crate::handlers::conversations::end_conversation,
            crate::handlers::stream::stream_conversation,
            crate::handlers::metrics::get_metrics,
            crate::handlers::push::push_agents,
            crate::handlers::push::upsert_agent,
            crate::handlers::push::remove_agent,
            crate::handlers::push::hydrate_conversations,
            crate::handlers::push::push_diff,
            crate::handlers::conversations::abort_conversation,
            crate::handlers::permissions::list_approvals,
            crate::handlers::permissions::resolve_approval,
            crate::handlers::permissions::bulk_resolve_approvals,
        ),
        components(schemas(
            bridge_core::AgentDefinition,
            bridge_core::AgentConfig,
            bridge_core::AgentSummary,
            bridge_core::ProviderConfig,
            bridge_core::ProviderType,
            bridge_core::ToolDefinition,
            bridge_core::McpServerDefinition,
            bridge_core::McpTransport,
            bridge_core::SkillDefinition,
            bridge_core::Message,
            bridge_core::Role,
            bridge_core::ContentBlock,
            bridge_core::ToolCall,
            bridge_core::ToolResult,
            bridge_core::ConversationRecord,
            bridge_core::MetricsSnapshot,
            bridge_core::GlobalMetrics,
            bridge_core::MetricsResponse,
            crate::handlers::conversations::SendMessageRequest,
            crate::handlers::push::PushAgentsRequest,
            crate::handlers::push::HydrateConversationsRequest,
            crate::handlers::push::PushDiffRequest,
            bridge_core::ApprovalRequest,
            bridge_core::permission::ApprovalStatus,
            bridge_core::ApprovalDecision,
            bridge_core::ApprovalReply,
            bridge_core::BulkApprovalReply,
            bridge_core::IntegrationDefinition,
            bridge_core::IntegrationAction,
        )),
        security(("bearer" = []))
    )]
    pub struct ApiDoc;
}

#[cfg(feature = "openapi")]
pub use openapi::ApiDoc;
