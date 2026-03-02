use serde::{Deserialize, Serialize};

/// Type alias for skill identifiers.
pub type SkillId = String;

/// Definition of a skill that can be activated by an agent.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct SkillDefinition {
    /// Unique identifier for the skill
    pub id: SkillId,
    /// Human-readable title
    pub title: String,
    /// Description of what the skill does
    pub description: String,
}
