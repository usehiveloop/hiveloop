pub mod agent;
pub mod config;
pub mod conversation;
pub mod error;
pub mod mcp;
pub mod metrics;
pub mod permission;
pub mod provider;
pub mod skill;
pub mod tool;
pub mod webhook;

#[cfg(test)]
mod tests;

// Re-exports for convenience
pub use agent::{AgentConfig, AgentDefinition, AgentId, AgentSummary};
pub use config::{LogFormat, LspConfig, RuntimeConfig};
pub use conversation::{
    ContentBlock, ConversationId, ConversationRecord, Message, PaginatedConversations, Role,
    ToolCall, ToolResult,
};
pub use error::{BridgeError, Result};
pub use mcp::{McpServerDefinition, McpTransport};
pub use metrics::{AgentMetrics, GlobalMetrics, MetricsResponse, MetricsSnapshot};
pub use provider::{ProviderConfig, ProviderType};
pub use skill::{SkillDefinition, SkillId};
pub use tool::ToolDefinition;
pub use permission::{ApprovalDecision, ApprovalReply, ApprovalRequest, BulkApprovalReply, ToolPermission};
pub use webhook::{WebhookEventType, WebhookPayload};
