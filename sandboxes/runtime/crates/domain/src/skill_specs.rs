use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SkillSpec {
    pub name: String,
    pub description: String,
    pub trigger: SkillTrigger,
    pub instructions: String,
    #[serde(default)]
    pub files: std::collections::HashMap<String, String>,
    #[serde(default)]
    pub category: Option<String>,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub related_skills: Vec<String>,
    #[serde(default)]
    pub required_environment_variables: Vec<String>,
    #[serde(default)]
    pub required_credential_files: Vec<String>,
    #[serde(default)]
    pub pinned: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum SkillTrigger {
    Always,
    Keyword { patterns: Vec<String> },
}
