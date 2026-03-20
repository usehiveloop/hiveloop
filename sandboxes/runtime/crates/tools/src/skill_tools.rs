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
    /// Optional arguments/parameters for the skill.
    /// These will be substituted into the skill content where {{args}} appears.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub args: Option<String>,
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
            Some(s) => {
                // Substitute {{args}} template variable if args provided
                let content = if let Some(ref skill_args) = args.args {
                    if s.content.contains("{{args}}") {
                        s.content.replace("{{args}}", skill_args)
                    } else {
                        // If no {{args}} placeholder but args provided, append them
                        format!("{}\n\nArguments: {}", s.content, skill_args)
                    }
                } else {
                    s.content.clone()
                };

                Ok(format!(
                    "<skill_content name=\"{}\" title=\"{}\">\n{}\n</skill_content>",
                    s.id, s.title, content
                ))
            }
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

    fn as_any(&self) -> &dyn std::any::Any {
        self
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
                parameters_schema: None,
            },
            SkillDefinition {
                id: "pr-summary".to_string(),
                title: "PR Summary".to_string(),
                description: "Summarizes pull requests concisely".to_string(),
                content: "You are a PR summarizer.\n\nCreate a concise summary of the changes.".to_string(),
                parameters_schema: None,
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

    #[tokio::test]
    async fn execute_with_args_substitutes_template() {
        let skills = vec![SkillDefinition {
            id: "commit".to_string(),
            title: "Commit".to_string(),
            description: "Writes commit messages".to_string(),
            content: "Write a commit message for: {{args}}".to_string(),
            parameters_schema: None,
        }];
        let tool = SkillTool::new(skills);
        let args = serde_json::json!({
            "name": "commit",
            "args": "fix the login bug"
        });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("<skill_content"));
        assert!(result.contains("Write a commit message for: fix the login bug"));
        // Template variable should be substituted
        assert!(!result.contains("{{args}}"));
    }

    #[tokio::test]
    async fn execute_with_args_no_template_appends_args() {
        let skills = vec![SkillDefinition {
            id: "review".to_string(),
            title: "Review".to_string(),
            description: "Reviews code".to_string(),
            content: "You are a code reviewer. Review the code.".to_string(),
            parameters_schema: None,
        }];
        let tool = SkillTool::new(skills);
        let args = serde_json::json!({
            "name": "review",
            "args": "PR #123"
        });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("You are a code reviewer. Review the code."));
        assert!(result.contains("Arguments: PR #123"));
    }

    #[tokio::test]
    async fn execute_without_args_ignores_template() {
        let skills = vec![SkillDefinition {
            id: "commit".to_string(),
            title: "Commit".to_string(),
            description: "Writes commit messages".to_string(),
            content: "Write a commit message for: {{args}}".to_string(),
            parameters_schema: None,
        }];
        let tool = SkillTool::new(skills);
        let args = serde_json::json!({ "name": "commit" });
        let result = tool.execute(args).await.expect("execute");

        // When no args provided, {{args}} remains in content (or skill should handle it)
        assert!(result.contains("Write a commit message for: {{args}}"));
    }

    #[tokio::test]
    async fn execute_with_empty_args_string() {
        let skills = vec![SkillDefinition {
            id: "commit".to_string(),
            title: "Commit".to_string(),
            description: "Writes commit messages".to_string(),
            content: "Write a commit message for: {{args}}".to_string(),
            parameters_schema: None,
        }];
        let tool = SkillTool::new(skills);
        let args = serde_json::json!({
            "name": "commit",
            "args": ""
        });
        let result = tool.execute(args).await.expect("execute");

        // Empty string should still substitute (removes the placeholder)
        assert!(result.contains("Write a commit message for: "));
        assert!(!result.contains("{{args}}"));
    }
}
