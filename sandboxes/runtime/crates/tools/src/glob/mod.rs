use async_trait::async_trait;
use globset::GlobBuilder;
use ignore::WalkBuilder;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::time::SystemTime;

use crate::boundary::ProjectBoundary;
use crate::ToolExecutor;

#[cfg(test)]
mod tests;

/// Arguments for the Glob tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct GlobArgs {
    /// Glob pattern to match files. Example: '**/*.rs', 'src/**/*.ts'
    #[schemars(description = "Glob pattern to match files. Example: '**/*.rs', 'src/**/*.ts'")]
    pub pattern: String,
    /// The directory to search in. Defaults to the current working directory.
    #[schemars(description = "Directory to search in. Defaults to the current working directory")]
    pub path: Option<String>,
}

/// A single matched file entry.
#[derive(Debug, Serialize, Deserialize)]
pub struct GlobFileEntry {
    pub path: String,
    pub modified: Option<String>,
}

/// Result returned by the Glob tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct GlobResult {
    pub files: Vec<GlobFileEntry>,
    pub total_matches: usize,
    pub truncated: bool,
    /// Steer for the agent when `truncated == true`: narrow the pattern,
    /// switch to RipGrep, or spawn self_agent instead of paginating blindly.
    /// Absent when the full result set fits.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hint: Option<String>,
}

/// Maximum number of results to return.
const MAX_RESULTS: usize = 1000;

pub struct GlobTool {
    boundary: Option<ProjectBoundary>,
}

impl GlobTool {
    pub fn new() -> Self {
        Self { boundary: None }
    }

    pub fn with_boundary(mut self, boundary: ProjectBoundary) -> Self {
        self.boundary = Some(boundary);
        self
    }
}

impl Default for GlobTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for GlobTool {
    fn name(&self) -> &str {
        "Glob"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/glob.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(GlobArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: GlobArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let pattern = args.pattern.clone();
        let search_path = args.path.clone().unwrap_or_else(|| ".".to_string());

        // Check project boundary for the search path
        if let Some(ref boundary) = self.boundary {
            if search_path != "." {
                boundary.check(&search_path)?;
            }
        }

        let result = tokio::task::spawn_blocking(move || execute_glob(&pattern, &search_path))
            .await
            .map_err(|e| format!("Task join error: {e}"))??;

        let serialized = serde_json::to_string(&result)
            .map_err(|e| format!("Failed to serialize result: {e}"))?;

        // Apply shared truncation for large results
        let truncated = crate::truncation::truncate_output(
            &serialized,
            crate::truncation::MAX_LINES,
            crate::truncation::MAX_BYTES,
        );
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

fn execute_glob(pattern: &str, search_path: &str) -> Result<GlobResult, String> {
    let glob = GlobBuilder::new(pattern)
        .literal_separator(false)
        .build()
        .map_err(|e| format!("Invalid glob pattern: {e}"))?;
    let matcher = glob.compile_matcher();

    let root = PathBuf::from(search_path);
    if !root.exists() {
        return Err(format!("Path does not exist: {search_path}"));
    }

    let walker = WalkBuilder::new(&root)
        .hidden(false)
        .git_ignore(true)
        .build();

    let mut matched_files: Vec<(String, Option<SystemTime>)> = Vec::new();
    let mut truncated = false;

    for entry in walker {
        let entry = match entry {
            Ok(e) => e,
            Err(_) => continue,
        };

        // Only match files, not directories
        let path = entry.path();
        if !path.is_file() {
            continue;
        }

        // Match against the relative path from the root
        let relative = path.strip_prefix(&root).unwrap_or(path);

        if matcher.is_match(relative) {
            // Early termination once we have enough results
            if matched_files.len() >= MAX_RESULTS {
                truncated = true;
                break;
            }

            let abs_path = if path.is_absolute() {
                path.to_string_lossy().to_string()
            } else {
                path.canonicalize()
                    .map(|p| p.to_string_lossy().to_string())
                    .unwrap_or_else(|_| path.to_string_lossy().to_string())
            };

            let modified = path.metadata().ok().and_then(|m| m.modified().ok());

            matched_files.push((abs_path, modified));
        }
    }

    // Sort by modification time, newest first
    matched_files.sort_by(|a, b| {
        let time_a = a.1.unwrap_or(SystemTime::UNIX_EPOCH);
        let time_b = b.1.unwrap_or(SystemTime::UNIX_EPOCH);
        time_b.cmp(&time_a)
    });

    let total_matches = matched_files.len();

    let files: Vec<GlobFileEntry> = matched_files
        .into_iter()
        .take(MAX_RESULTS)
        .map(|(path, modified)| {
            let modified_str = modified
                .and_then(|t| t.duration_since(SystemTime::UNIX_EPOCH).ok())
                .map(|d| {
                    chrono::DateTime::from_timestamp(d.as_secs() as i64, d.subsec_nanos())
                        .map(|dt| dt.to_rfc3339())
                        .unwrap_or_default()
                });
            GlobFileEntry {
                path,
                modified: modified_str,
            }
        })
        .collect();

    let hint = if truncated {
        Some(format!(
            "Result capped at {MAX_RESULTS} matches. Choose one: narrow the pattern to match fewer files, call RipGrep with a regex to find specific content instead of listing files, or spawn self_agent with a focused question."
        ))
    } else {
        None
    };

    Ok(GlobResult {
        files,
        total_matches,
        truncated,
        hint,
    })
}
