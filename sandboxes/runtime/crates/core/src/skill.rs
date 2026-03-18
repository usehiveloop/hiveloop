use serde::{Deserialize, Serialize};

/// Type alias for skill identifiers.
pub type SkillId = String;

/// Definition of a skill that can be activated by an agent.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SkillDefinition {
    /// Unique identifier for the skill
    pub id: SkillId,
    /// Human-readable title
    pub title: String,
    /// Description of what the skill does
    pub description: String,
    /// Full skill prompt/instructions content
    /// Can contain template variables like {{args}} that will be substituted
    pub content: String,
    /// Optional JSON Schema for structured parameters (for future use)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub parameters_schema: Option<serde_json::Value>,
}
