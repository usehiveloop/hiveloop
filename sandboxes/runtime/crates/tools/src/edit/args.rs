use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// Arguments for the Edit tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct EditArgs {
    /// Absolute path to the file to modify.
    #[schemars(description = "Absolute path to the file to modify")]
    pub file_path: String,
    /// The exact text to find and replace. Must match uniquely in the file unless replaceAll is true.
    #[schemars(
        description = "The exact text to find and replace. Must match uniquely in the file unless replaceAll is true"
    )]
    pub old_string: String,
    /// The replacement text. Must differ from oldString.
    #[schemars(description = "The replacement text. Must differ from oldString")]
    pub new_string: String,
    /// If true, replace all occurrences of oldString. Defaults to false.
    #[schemars(description = "If true, replace all occurrences of oldString. Defaults to false")]
    pub replace_all: Option<bool>,
}

/// Result returned by the Edit tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct EditResult {
    pub path: String,
    pub old_content_snippet: String,
    pub new_content_snippet: String,
    pub replacements_made: usize,
    pub lines_added: usize,
    pub lines_removed: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diff: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diagnostics: Option<String>,
}
