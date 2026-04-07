use async_trait::async_trait;
use bridge_core::SkillDefinition;
use regex::Regex;
use schemars::JsonSchema;
use serde::Deserialize;
use std::collections::HashMap;

use crate::ToolExecutor;

/// Arguments for the skill tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct SkillToolArgs {
    /// The name of the skill to load (matches skill id or title, case-insensitive).
    pub name: String,
    /// Optional arguments/parameters for the skill.
    /// These will be substituted into the skill content where {{args}} or $ARGUMENTS appears.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub args: Option<String>,
    /// Request a specific supporting file by its relative path.
    /// When set, returns only that file's content instead of the full skill.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file: Option<String>,
}

/// A tool that loads domain-specific skill instructions by name.
///
/// When invoked, returns the full skill content from memory, with variable
/// substitution and file reference resolution applied.
pub struct SkillTool {
    skills: Vec<SkillDefinition>,
}

impl SkillTool {
    pub fn new(skills: Vec<SkillDefinition>) -> Self {
        Self { skills }
    }

    /// Get a reference to the skills held by this tool.
    pub fn skills(&self) -> &Vec<SkillDefinition> {
        &self.skills
    }
}

/// Substitute template variables in skill content.
///
/// Supported variables:
/// - `{{args}}` and `$ARGUMENTS` → full args string
/// - `$1`, `$2`, ... → positional args (whitespace-split)
/// - `${CLAUDE_SKILL_DIR}` → skill id (virtual path)
fn substitute_variables(content: &str, args: Option<&str>, skill_id: &str) -> String {
    let mut result = content.to_string();

    // Always substitute ${CLAUDE_SKILL_DIR}
    result = result.replace("${CLAUDE_SKILL_DIR}", skill_id);

    if let Some(args_str) = args {
        // Substitute $ARGUMENTS and {{args}} with the full args string
        result = result.replace("$ARGUMENTS", args_str);
        result = result.replace("{{args}}", args_str);

        // Substitute positional args: $1, $2, etc.
        let positional: Vec<&str> = args_str.split_whitespace().collect();
        // Replace in reverse order so $10 is replaced before $1
        for (i, val) in positional.iter().enumerate().rev() {
            let placeholder = format!("${}", i + 1);
            result = result.replace(&placeholder, val);
        }
    }

    result
}

/// Resolve markdown file references against the skill's supporting files.
///
/// For each markdown link `[label](path)` where `path` matches a key in the
/// files map, the link is replaced with the file's content wrapped in XML tags.
/// Non-matching links (external URLs, unknown paths) are left untouched.
fn resolve_file_references(content: &str, files: &HashMap<String, String>) -> String {
    if files.is_empty() {
        return content.to_string();
    }

    let re = Regex::new(r"\[([^\]]*)\]\(([^)]+)\)").unwrap();
    let mut result = content.to_string();
    // Collect matches first to avoid borrow issues with replacement
    let matches: Vec<(String, String, String)> = re
        .captures_iter(content)
        .filter_map(|cap| {
            let full = cap[0].to_string();
            let path = cap[2].to_string();
            if files.contains_key(&path) {
                Some((full, path, cap[1].to_string()))
            } else {
                None
            }
        })
        .collect();

    for (full_match, path, _label) in matches {
        if let Some(file_content) = files.get(&path) {
            let replacement = format!(
                "<skill_file path=\"{}\">\n{}\n</skill_file>",
                path, file_content
            );
            result = result.replace(&full_match, &replacement);
        }
    }

    result
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
                // If a specific file is requested, return just that file's content
                if let Some(ref file_path) = args.file {
                    return match s.files.get(file_path) {
                        Some(file_content) => Ok(format!(
                            "<skill_file name=\"{}\" path=\"{}\">\n{}\n</skill_file>",
                            s.id, file_path, file_content
                        )),
                        None => {
                            let available: Vec<&str> = s.files.keys().map(|k| k.as_str()).collect();
                            Err(format!(
                                "File '{}' not found in skill '{}'. Available files: [{}]",
                                file_path,
                                s.title,
                                available.join(", ")
                            ))
                        }
                    };
                }

                // Substitute variables, then resolve file references
                let content = substitute_variables(&s.content, args.args.as_deref(), &s.id);
                let content = resolve_file_references(&content, &s.files);

                // If no variable substitution happened but args were provided,
                // and the content doesn't contain any of our placeholders, append args
                let content = if args.args.is_some()
                    && !s.content.contains("{{args}}")
                    && !s.content.contains("$ARGUMENTS")
                    && !s.content.contains("$1")
                {
                    format!(
                        "{}\n\nArguments: {}",
                        content,
                        args.args.as_deref().unwrap_or("")
                    )
                } else {
                    content
                };

                let source_attr = format!("{:?}", s.source).to_lowercase();
                Ok(format!(
                    "<skill_content name=\"{}\" title=\"{}\" source=\"{}\">\n{}\n</skill_content>",
                    s.id, s.title, source_attr, content
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
    use bridge_core::SkillSource;

    fn make_skills() -> Vec<SkillDefinition> {
        vec![
            SkillDefinition {
                id: "code-review".to_string(),
                title: "Code Review".to_string(),
                description: "Reviews code for quality and best practices".to_string(),
                content: "You are a code review expert.\n\n## Guidelines\n- Check for bugs\n- Suggest improvements".to_string(),
                ..Default::default()
            },
            SkillDefinition {
                id: "pr-summary".to_string(),
                title: "PR Summary".to_string(),
                description: "Summarizes pull requests concisely".to_string(),
                content: "You are a PR summarizer.\n\nCreate a concise summary of the changes.".to_string(),
                ..Default::default()
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
            ..Default::default()
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
            ..Default::default()
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
            ..Default::default()
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
            ..Default::default()
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

    // --- New tests for multi-file skills ---

    fn make_multi_file_skill() -> SkillDefinition {
        let mut files = HashMap::new();
        files.insert(
            "reference.md".to_string(),
            "# API Reference\n\nGET /users - list users".to_string(),
        );
        files.insert(
            "examples/basic.md".to_string(),
            "# Basic Example\n\ncurl localhost/users".to_string(),
        );

        SkillDefinition {
            id: "api-docs".to_string(),
            title: "API Docs".to_string(),
            description: "API documentation skill".to_string(),
            content: "You are an API expert.\n\nSee [reference.md](reference.md) for the API.\nSee [examples](examples/basic.md) for usage.".to_string(),
            files,
            source: SkillSource::ControlPlane,
            ..Default::default()
        }
    }

    #[tokio::test]
    async fn execute_resolves_file_references() {
        let tool = SkillTool::new(vec![make_multi_file_skill()]);
        let args = serde_json::json!({ "name": "api-docs" });
        let result = tool.execute(args).await.expect("execute");

        // File references should be inlined
        assert!(result.contains("<skill_file path=\"reference.md\">"));
        assert!(result.contains("GET /users - list users"));
        assert!(result.contains("<skill_file path=\"examples/basic.md\">"));
        assert!(result.contains("curl localhost/users"));
        // Original markdown links should be replaced
        assert!(!result.contains("[reference.md](reference.md)"));
    }

    #[tokio::test]
    async fn execute_file_param_returns_specific_file() {
        let tool = SkillTool::new(vec![make_multi_file_skill()]);
        let args = serde_json::json!({
            "name": "api-docs",
            "file": "reference.md"
        });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("<skill_file"));
        assert!(result.contains("GET /users - list users"));
        // Should NOT contain the main skill content
        assert!(!result.contains("You are an API expert"));
    }

    #[tokio::test]
    async fn execute_file_param_returns_error_for_unknown_file() {
        let tool = SkillTool::new(vec![make_multi_file_skill()]);
        let args = serde_json::json!({
            "name": "api-docs",
            "file": "nonexistent.md"
        });
        let result = tool.execute(args).await;

        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(err.contains("not found"));
        assert!(err.contains("reference.md"));
    }

    #[test]
    fn substitute_variables_with_arguments() {
        let result = substitute_variables("Deploy $ARGUMENTS now", Some("staging"), "deploy");
        assert_eq!(result, "Deploy staging now");
    }

    #[test]
    fn substitute_variables_with_positional() {
        let result = substitute_variables("Deploy $1 to $2", Some("app production"), "deploy");
        assert_eq!(result, "Deploy app to production");
    }

    #[test]
    fn substitute_variables_with_skill_dir() {
        let result = substitute_variables("Run ${CLAUDE_SKILL_DIR}/script.sh", None, "my-skill");
        assert_eq!(result, "Run my-skill/script.sh");
    }

    #[test]
    fn substitute_variables_backward_compat_handlebars() {
        let result = substitute_variables("Do: {{args}}", Some("the thing"), "test");
        assert_eq!(result, "Do: the thing");
    }

    #[test]
    fn substitute_variables_no_args_no_op() {
        let result = substitute_variables("Static content {{args}} $ARGUMENTS", None, "test");
        // Only ${CLAUDE_SKILL_DIR} is substituted; {{args}} and $ARGUMENTS are left
        assert_eq!(result, "Static content {{args}} $ARGUMENTS");
    }

    #[test]
    fn resolve_file_references_inlines_matching_files() {
        let mut files = HashMap::new();
        files.insert("ref.md".to_string(), "reference content".to_string());

        let result = resolve_file_references("See [docs](ref.md) for details.", &files);
        assert!(result.contains("<skill_file path=\"ref.md\">"));
        assert!(result.contains("reference content"));
        assert!(!result.contains("[docs](ref.md)"));
    }

    #[test]
    fn resolve_file_references_leaves_external_links() {
        let files = HashMap::new();
        let input = "See [Google](https://google.com) for more.";
        let result = resolve_file_references(input, &files);
        assert_eq!(result, input);
    }

    #[test]
    fn resolve_file_references_empty_files_noop() {
        let input = "See [ref](ref.md)";
        let result = resolve_file_references(input, &HashMap::new());
        assert_eq!(result, input);
    }

    #[tokio::test]
    async fn execute_includes_source_attribute() {
        let mut skill = make_multi_file_skill();
        skill.source = SkillSource::ClaudeCode;
        let tool = SkillTool::new(vec![skill]);
        let args = serde_json::json!({ "name": "api-docs" });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("source=\"claudecode\""));
    }

    #[tokio::test]
    async fn execute_with_dollar_arguments_substitution() {
        let skills = vec![SkillDefinition {
            id: "deploy".to_string(),
            title: "Deploy".to_string(),
            description: "Deploys".to_string(),
            content: "Deploy $ARGUMENTS to production".to_string(),
            ..Default::default()
        }];
        let tool = SkillTool::new(skills);
        let args = serde_json::json!({
            "name": "deploy",
            "args": "my-app"
        });
        let result = tool.execute(args).await.expect("execute");

        assert!(result.contains("Deploy my-app to production"));
        // Should NOT append "Arguments:" since $ARGUMENTS was a placeholder
        assert!(!result.contains("Arguments: my-app"));
    }

    #[test]
    fn skills_getter_returns_all_skills() {
        let skills = make_skills();
        let tool = SkillTool::new(skills.clone());
        assert_eq!(tool.skills().len(), 2);
        assert_eq!(tool.skills()[0].id, "code-review");
    }
}
