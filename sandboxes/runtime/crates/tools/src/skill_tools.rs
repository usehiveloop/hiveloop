use async_trait::async_trait;
use bridge_core::SkillDefinition;
use schemars::JsonSchema;
use serde::Deserialize;

use crate::ToolExecutor;

/// Arguments for the skill tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct SkillToolArgs {
    /// The name of the skill to load (matches skill id or title, case-insensitive).
    pub name: String,
}

/// A tool that loads domain-specific skill instructions by name.
///
/// When invoked, returns the full skill content from memory.
pub struct SkillTool {
    skills: Vec<SkillDefinition>,
}

impl SkillTool {
    pub fn new(skills: Vec<SkillDefinition>) -> Self {
        Self { skills }
    }
}

#[async_trait]
impl ToolExecutor for SkillTool {
    fn name(&self) -> &str {
        "skill"
    }

    fn description(&self) -> &str {
        include_str!("instructions/skill.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(SkillToolArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: SkillToolArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let query = args.name.to_lowercase();

        let skill = self
            .skills
            .iter()
            .find(|s| s.id.to_lowercase() == query || s.title.to_lowercase() == query);

        match skill {
            Some(s) => Ok(format!(
                "<skill_content name=\"{}\" title=\"{}\">\n{}\n</skill_content>",
                s.id, s.title, s.content
            )),
            None => {
                let available: Vec<&str> = self.skills.iter().map(|s| s.title.as_str()).collect();
                Err(format!(
                    "Skill '{}' not found. Available skills: [{}]",
                    args.name,
                    available.join(", ")
                ))
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    fn make_skills() -> Vec<SkillDefinition> {
        vec![
            SkillDefinition {
                id: "code-review".to_string(),
                title: "Code Review".to_string(),
                description: "Reviews code for quality and best practices".to_string(),
                content: "You are a code review expert.\n\n## Guidelines\n- Check for bugs\n- Suggest improvements".to_string(),
            },
            SkillDefinition {
                id: "pr-summary".to_string(),
                title: "PR Summary".to_string(),
                description: "Summarizes pull requests concisely".to_string(),
                content: "You are a PR summarizer.\n\nCreate a concise summary of the changes.".to_string(),
            },
        ]
    }

    #[test]
    fn description_is_static_from_file() {
        let tool = SkillTool::new(make_skills());
        let desc = tool.description();

        // Description now comes from static file, not dynamically generated
        assert!(desc.contains("Execute a skill within the main conversation"));
        assert!(desc.contains("slash command"));
        assert!(desc.contains("BLOCKING REQUIREMENT"));
        // Skill content should NOT be in the description
        assert!(!desc.contains("You are a code review expert"));
        assert!(!desc.contains("You are a PR summarizer"));
    }

    #[tokio::test]
    async fn execute_returns_full_content_for_valid_skill() {
        let tool = SkillTool::new(make_skills());
        let args = serde_json::json!({ "name": "Code Review" });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("<skill_content"));
        assert!(result.contains("You are a code review expert"));
        assert!(result.contains("Check for bugs"));
    }

    #[tokio::test]
    async fn execute_returns_error_for_unknown_skill() {
        let tool = SkillTool::new(make_skills());
        let args = serde_json::json!({ "name": "nonexistent" });
        let result = tool.execute(args).await;

        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(err.contains("not found"));
        assert!(err.contains("Code Review"));
        assert!(err.contains("PR Summary"));
    }

    #[tokio::test]
    async fn execute_case_insensitive_matching_by_title() {
        let tool = SkillTool::new(make_skills());
        let args = serde_json::json!({ "name": "code review" });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("You are a code review expert"));
    }

    #[tokio::test]
    async fn execute_matches_by_id() {
        let tool = SkillTool::new(make_skills());
        let args = serde_json::json!({ "name": "pr-summary" });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("You are a PR summarizer"));
    }

    #[tokio::test]
    async fn execute_case_insensitive_matching_by_id() {
        let tool = SkillTool::new(make_skills());
        let args = serde_json::json!({ "name": "PR-SUMMARY" });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("You are a PR summarizer"));
    }
}
