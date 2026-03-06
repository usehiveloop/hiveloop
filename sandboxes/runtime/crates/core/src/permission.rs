use serde::{Deserialize, Serialize};

/// Permission level for a tool within an agent.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub enum ToolPermission {
    /// Execute immediately without any approval.
    Allow,
    /// Block execution and return an error to the LLM.
    Deny,
    /// Pause execution and wait for user approval via HTTP.
    RequireApproval,
}

/// Status of an approval request.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ApprovalStatus {
    Pending,
    Approved,
    Denied,
}

/// The user's decision on an approval request.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ApprovalDecision {
    Approve,
    Deny,
}

/// A pending approval request for a tool call.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ApprovalRequest {
    /// Unique ID for this approval request.
    pub id: String,
    /// Agent that initiated the tool call.
    pub agent_id: String,
    /// Conversation in which the tool call occurred.
    pub conversation_id: String,
    /// Name of the tool being called.
    pub tool_name: String,
    /// The LLM's tool call ID.
    pub tool_call_id: String,
    /// Arguments passed to the tool.
    pub arguments: serde_json::Value,
    /// Current status of the approval request.
    pub status: ApprovalStatus,
    /// When the approval request was created.
    pub created_at: chrono::DateTime<chrono::Utc>,
}

/// HTTP request body for resolving a single approval.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ApprovalReply {
    pub decision: ApprovalDecision,
}

/// HTTP request body for resolving multiple approvals at once.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BulkApprovalReply {
    pub request_ids: Vec<String>,
    pub decision: ApprovalDecision,
}
