use bridge_core::AgentDefinition;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushAgentsRequest {
    pub agents: Vec<AgentDefinition>,
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushDiffRequest {
    pub added: Vec<AgentDefinition>,
    pub updated: Vec<AgentDefinition>,
    pub removed: Vec<String>,
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpdateApiKeyRequest {
    pub api_key: String,
}

/// Response for pushing agents.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushAgentsResponse {
    /// Number of agents loaded.
    pub loaded: usize,
}

/// Response for upserting an agent.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpsertAgentResponse {
    /// Status of the operation: "unchanged", "updated", or "created".
    pub status: String,
}

/// Response for removing an agent.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RemoveAgentResponse {
    /// Status of the operation.
    pub status: String,
}

/// Response for updating an API key.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpdateApiKeyResponse {
    /// Status of the operation.
    pub status: String,
}

/// Response for pushing a diff.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushDiffResponse {
    /// Number of agents added.
    pub added: usize,
    /// Number of agents updated.
    pub updated: usize,
    /// Number of agents removed.
    pub removed: usize,
}
