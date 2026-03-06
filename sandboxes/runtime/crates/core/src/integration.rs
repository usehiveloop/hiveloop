use serde::{Deserialize, Serialize};

use crate::permission::ToolPermission;

/// Definition of an external integration available to an agent.
///
/// Each integration represents a connection to an external service (e.g., GitHub,
/// Mailchimp, Slack) managed by the control plane. The control plane handles
/// authentication and proxies requests to the underlying service.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct IntegrationDefinition {
    /// Integration identifier (e.g., "github", "mailchimp", "slack").
    pub name: String,
    /// Human-readable description of the integration.
    pub description: String,
    /// Available actions within this integration.
    pub actions: Vec<IntegrationAction>,
}

/// A single action within an integration that an agent can invoke.
///
/// Each action maps to a specific API operation on the external service.
/// The control plane defines the schema and permissions for each action.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[cfg_attr(feature = "openapi", schema(no_recursion))]
pub struct IntegrationAction {
    /// Action identifier (e.g., "create_pull_request", "send_message").
    pub name: String,
    /// Human-readable description of what this action does.
    pub description: String,
    /// JSON Schema for the action's parameters.
    pub parameters_schema: serde_json::Value,
    /// Permission level for this action.
    pub permission: ToolPermission,
}
