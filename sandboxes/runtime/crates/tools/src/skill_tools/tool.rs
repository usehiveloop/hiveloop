use async_trait::async_trait;
use bridge_core::SkillDefinition;
use schemars::JsonSchema;
use serde::Deserialize;
use std::path::PathBuf;

use super::substitute::{resolve_file_references, substitute_variables};
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
    /// Base directory where `.skills/<skill-id>/` directories are written.
    /// When set, `${CLAUDE_SKILL_DIR}` resolves to `<base_dir>/.skills/<skill-id>`.
    base_dir: Option<PathBuf>,
}

impl SkillTool {
    pub fn new(skills: Vec<SkillDefinition>) -> Self {
        Self {
            skills,
            base_dir: None,
        }
    }

    /// Create a SkillTool whose skill files are materialized under `base_dir`.
    pub fn with_base_dir(skills: Vec<SkillDefinition>, base_dir: PathBuf) -> Self {
        Self {
            skills,
            base_dir: Some(base_dir),
        }
    }

    /// Get a reference to the skills held by this tool.
    pub fn skills(&self) -> &Vec<SkillDefinition> {
        &self.skills
    }
}

#[async_trait]
impl ToolExecutor for SkillTool {
    fn name(&self) -> &str {
        "skill"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/skill.txt")
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

                // Resolve the skill directory path for variable substitution.
                let skill_dir_str = match &self.base_dir {
                    Some(base) => crate::skill_files::skill_dir_path(&s.id, base)
                        .to_string_lossy()
                        .to_string(),
                    None => s.id.clone(),
                };

                // Substitute variables, then resolve file references
                let content =
                    substitute_variables(&s.content, args.args.as_deref(), &skill_dir_str);
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

                // Prepend a note about the skill's files location on disk.
                let files_note = if !s.files.is_empty() {
                    if let Some(base) = &self.base_dir {
                        let dir = crate::skill_files::skill_dir_path(&s.id, base);
                        format!(
                            "NOTE: This skill's files are at {}/\nPrefix script paths with this directory.\n\n---\n\n",
                            dir.display()
                        )
                    } else {
                        String::new()
                    }
                } else {
                    String::new()
                };

                let source_attr = format!("{:?}", s.source).to_lowercase();
                Ok(format!(
                    "<skill_content name=\"{}\" title=\"{}\" source=\"{}\">\n{}{}\n</skill_content>",
                    s.id, s.title, source_attr, files_note, content
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
