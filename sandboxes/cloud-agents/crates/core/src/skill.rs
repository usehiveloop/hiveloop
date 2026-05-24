use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Type alias for skill identifiers.
pub type SkillId = String;

/// Where a skill originated from.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum SkillSource {
    /// Pushed by the control plane API.
    #[default]
    ControlPlane,
    /// Discovered from .claude/skills/ or .claude/commands/.
    ClaudeCode,
    /// Discovered from .agent/skills/.
    AgentSkills,
    /// Discovered from .cursor/rules/ or .cursorrules.
    CursorRules,
    /// Discovered from .github/copilot-instructions.md.
    GitHubCopilot,
    /// Discovered from .windsurf/rules/ or .windsurfrules.
    WindsurfRules,
}

/// Frontmatter configuration parsed from a SKILL.md YAML header.
///
/// Only applicable to sources that support frontmatter (ClaudeCode, AgentSkills).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SkillFrontmatter {
    /// List of tools the skill is allowed to use.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub allowed_tools: Option<Vec<String>>,
    /// Reasoning effort level (e.g., "low", "medium", "high").
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub effort: Option<String>,
    /// Execution context. "fork" runs in an isolated subagent.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub context: Option<String>,
    /// Glob patterns for auto-activation on matching files.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub paths: Option<Vec<String>>,
    /// When true, only users can invoke this skill (hidden from the model).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub disable_model_invocation: Option<bool>,
    /// When false, only the model can invoke this skill (hidden from slash commands).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub user_invocable: Option<bool>,
    /// Hint text shown for the argument placeholder in autocomplete.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub argument_hint: Option<String>,
    /// Shell to use for shell-related operations (e.g., "bash", "powershell").
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub shell: Option<String>,
    /// Sub-agent type to delegate to when context is "fork".
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub agent: Option<String>,
}

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
    /// Full skill prompt/instructions content (the SKILL.md body or single-content string).
    /// Can contain template variables like {{args}}, $ARGUMENTS, $1, $2, etc.
    pub content: String,
    /// Optional JSON Schema for structured parameters (for future use)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub parameters_schema: Option<serde_json::Value>,
    /// Supporting files: relative_path -> file_content.
    /// Contains files referenced by the skill (e.g., "api-reference.md", "examples/ex1.md").
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub files: HashMap<String, String>,
    /// Parsed frontmatter configuration from SKILL.md YAML header.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub frontmatter: Option<SkillFrontmatter>,
    /// Where this skill originated from.
    #[serde(default)]
    pub source: SkillSource,
}
