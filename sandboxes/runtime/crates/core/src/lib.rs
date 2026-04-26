pub mod agent;
pub mod artifacts;
pub mod config;
pub mod conversation;
pub mod error;
pub mod event;
pub mod integration;
pub mod mcp;
pub mod metrics;
pub mod permission;
pub mod provider;
pub mod skill;
pub mod tool;

#[cfg(test)]
mod tests;

// Re-exports for convenience
pub use agent::{AgentConfig, AgentDefinition, AgentId, AgentSummary};
pub use artifacts::ArtifactsConfig;
pub use config::{LogFormat, LspConfig, RuntimeConfig, WebhookConfig};
pub use conversation::{
    ContentBlock, ConversationId, ConversationRecord, Message, PaginatedConversations, Role,
    ToolCall, ToolResult,
};
pub use error::{BridgeError, Result};
pub use event::{BridgeEvent, BridgeEventType};
pub use integration::{IntegrationAction, IntegrationDefinition};
pub use mcp::{McpServerDefinition, McpTransport};
pub use metrics::{
    AgentMetrics, GlobalMetrics, MetricsResponse, MetricsSnapshot, ToolCallStats,
    ToolCallStatsSnapshot,
};
pub use permission::{
    ApprovalDecision, ApprovalReply, ApprovalRequest, BulkApprovalReply, ToolPermission,
};
pub use provider::{ProviderConfig, ProviderType};
pub use skill::{SkillDefinition, SkillFrontmatter, SkillId, SkillSource};
pub use tool::ToolDefinition;
