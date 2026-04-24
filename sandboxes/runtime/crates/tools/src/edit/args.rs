use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// Arguments for the Edit tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct EditArgs {
    /// Absolute path to the file to modify.
    #[schemars(description = "Absolute path to the file to modify")]
    pub file_path: String,
    /// The exact text to find and replace. Must match uniquely in the file unless replace_all is true.
    #[schemars(
        description = "The exact text to find and replace. Must match uniquely in the file unless replace_all is true"
    )]
    pub old_string: String,
    /// The replacement text. Must differ from old_string.
    #[schemars(description = "The replacement text. Must differ from old_string")]
    pub new_string: String,
    /// If true, replace all occurrences of old_string. Defaults to false.
    #[schemars(description = "If true, replace all occurrences of old_string. Defaults to false")]
    pub replace_all: Option<bool>,
}

/// Result returned by the Edit tool.
///
/// `path`, `old_content_snippet`, and `new_content_snippet` are intentionally
/// omitted from the serialized output. The model already sent `file_path`,
/// `old_string`, and `new_string` in its args; echoing them back costs ~300
/// bytes per edit × N later turns of carried context. The `diff` field
/// already captures the meaningful change. Fields kept on the struct with
/// `#[serde(skip)]` so internal callers that construct/read `EditResult`
/// in-process aren't broken.
#[derive(Debug, Serialize, Deserialize)]
pub struct EditResult {
    #[serde(skip)]
    pub path: String,
    #[serde(skip)]
    pub old_content_snippet: String,
    #[serde(skip)]
    pub new_content_snippet: String,
    pub replacements_made: usize,
    pub lines_added: usize,
    pub lines_removed: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diff: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diagnostics: Option<String>,
}
