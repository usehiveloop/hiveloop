use async_trait::async_trait;
use grep_regex::RegexMatcherBuilder;
use grep_searcher::{Searcher, SearcherBuilder, Sink, SinkContext, SinkContextKind, SinkMatch};
use ignore::overrides::OverrideBuilder;
use ignore::types::TypesBuilder;
use ignore::WalkBuilder;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::SystemTime;

use crate::boundary::ProjectBoundary;
use crate::ToolExecutor;

/// Maximum length for a single match line before truncation.
const MAX_MATCH_LINE_LENGTH: usize = 2000;

fn truncate_match_line(line: &str) -> String {
    if line.len() > MAX_MATCH_LINE_LENGTH {
        format!("{}...", &line[..MAX_MATCH_LINE_LENGTH])
    } else {
        line.to_string()
    }
}

/// Output mode for grep results.
#[derive(Debug, Deserialize, JsonSchema, Clone, Default)]
#[serde(rename_all = "snake_case")]
pub enum OutputMode {
    /// Show matching lines with context.
    Content,
    /// Show only file paths that contain matches (default).
    #[default]
    FilesWithMatches,
    /// Show match counts per file.
    Count,
}

/// Arguments for the Grep tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct GrepArgs {
    /// Regular expression pattern to search for. Example: 'fn\\s+main'
    #[schemars(description = "Regular expression pattern to search for. Example: 'fn\\s+main'")]
    pub pattern: String,
    /// File or directory to search in. Defaults to current working directory.
    #[schemars(
        description = "File or directory to search in. Defaults to current working directory"
    )]
    pub path: Option<String>,
    /// Glob pattern to filter files (e.g., "*.js", "*.{ts,tsx}").
    #[schemars(description = "Glob pattern to filter files. Example: '*.rs', '*.{ts,tsx}'")]
    pub glob: Option<String>,
    /// File type to search (e.g., "js", "py", "rust"). More efficient than glob for standard types.
    #[schemars(
        description = "File type to search. Example: 'js', 'py', 'rust'. More efficient than glob for standard types"
    )]
    pub file_type: Option<String>,
    /// Case insensitive search.
    #[schemars(description = "Set to true for case-insensitive matching")]
    pub case_insensitive: Option<bool>,
    /// Number of lines to show before each match.
    #[schemars(description = "Number of context lines to show before each match")]
    pub context_before: Option<usize>,
    /// Number of lines to show after each match.
    #[schemars(description = "Number of context lines to show after each match")]
    pub context_after: Option<usize>,
    /// Number of lines to show before and after each match (overrides context_before/after).
    #[schemars(
        description = "Number of context lines before and after each match. Overrides context_before/context_after"
    )]
    pub context: Option<usize>,
    /// Output mode: "content", "files_with_matches", or "count".
    #[schemars(
        description = "Output mode: 'content' shows matching lines, 'files_with_matches' shows file paths (default), 'count' shows match counts"
    )]
    pub output_mode: Option<OutputMode>,
    /// Maximum number of results to return. Default: 100. Use a higher value when you need comprehensive search results, or lower for quick lookups.
    #[schemars(
        description = "Maximum number of results to return. Default: 100. Use a higher value for comprehensive results, lower for quick lookups"
    )]
    pub max_results: Option<usize>,
}

/// A single match in content mode.
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct GrepMatch {
    pub path: String,
    pub line_number: Option<u64>,
    pub content: String,
    pub is_context: bool,
}

/// A count result for count mode.
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct GrepCountEntry {
    pub path: String,
    pub count: usize,
}

/// Result returned by the Grep tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct GrepResult {
    pub matches: Vec<serde_json::Value>,
    pub total_matches: usize,
    pub files_searched: usize,
    pub truncated: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub note: Option<String>,
}

/// Default max results.
const DEFAULT_MAX_RESULTS: usize = 100;

pub struct GrepTool {
    boundary: Option<ProjectBoundary>,
}

impl GrepTool {
    pub fn new() -> Self {
        Self { boundary: None }
    }

    pub fn with_boundary(mut self, boundary: ProjectBoundary) -> Self {
        self.boundary = Some(boundary);
        self
    }
}

impl Default for GrepTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for GrepTool {
    fn name(&self) -> &str {
        "Grep"
    }

    fn description(&self) -> &str {
        include_str!("instructions/grep.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(GrepArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: GrepArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        // Check project boundary for the search path
        if let Some(ref boundary) = self.boundary {
            if let Some(ref path) = args.path {
                if path != "." {
                    boundary.check(path)?;
                }
            }
        }

        let result = tokio::task::spawn_blocking(move || execute_grep(args))
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

/// Custom sink for collecting grep matches.
struct CollectorSink {
    path: PathBuf,
    matches: Arc<Mutex<Vec<GrepMatch>>>,
    match_count: usize,
}

impl Sink for CollectorSink {
    type Error = std::io::Error;

    fn matched(&mut self, _searcher: &Searcher, mat: &SinkMatch<'_>) -> Result<bool, Self::Error> {
        let content = truncate_match_line(String::from_utf8_lossy(mat.bytes()).trim_end());
        let line_number = mat.line_number();

        let entry = GrepMatch {
            path: self.path.to_string_lossy().to_string(),
            line_number,
            content,
            is_context: false,
        };

        if let Ok(mut matches) = self.matches.lock() {
            matches.push(entry);
        }
        self.match_count += 1;

        Ok(true)
    }

    fn context(
        &mut self,
        _searcher: &Searcher,
        ctx: &SinkContext<'_>,
    ) -> Result<bool, Self::Error> {
        // Only include before/after context, not other context types
        match ctx.kind() {
            &SinkContextKind::Before | &SinkContextKind::After => {}
            _ => return Ok(true),
        }

        let content = truncate_match_line(String::from_utf8_lossy(ctx.bytes()).trim_end());
        let line_number = ctx.line_number();

        let entry = GrepMatch {
            path: self.path.to_string_lossy().to_string(),
            line_number,
            content,
            is_context: true,
        };

        if let Ok(mut matches) = self.matches.lock() {
            matches.push(entry);
        }

        Ok(true)
    }
}

fn execute_grep(args: GrepArgs) -> Result<GrepResult, String> {
    let search_path = args.path.clone().unwrap_or_else(|| ".".to_string());
    let output_mode = args.output_mode.clone().unwrap_or_default();
    let max_results = args.max_results.unwrap_or(DEFAULT_MAX_RESULTS);

    // Build the regex matcher
    let mut matcher_builder = RegexMatcherBuilder::new();
    if args.case_insensitive.unwrap_or(false) {
        matcher_builder.case_insensitive(true);
    }
    let matcher = matcher_builder
        .build(&args.pattern)
        .map_err(|e| format!("Invalid regex pattern: {e}"))?;

    // Determine context lines
    let context_before = args.context.unwrap_or(args.context_before.unwrap_or(0));
    let context_after = args.context.unwrap_or(args.context_after.unwrap_or(0));

    // Build the searcher
    let searcher_builder = SearcherBuilder::new()
        .line_number(true)
        .before_context(context_before)
        .after_context(context_after)
        .build();

    let root = Path::new(&search_path);
    if !root.exists() {
        return Err(format!("Path does not exist: {search_path}"));
    }

    // Build walker with optional file type and glob filtering
    let mut walk_builder = WalkBuilder::new(root);
    walk_builder.hidden(false).git_ignore(true);

    // Apply file type filter
    if let Some(ref ft) = args.file_type {
        let mut types_builder = TypesBuilder::new();
        types_builder.add_defaults();
        types_builder.select(ft);
        let types = types_builder
            .build()
            .map_err(|e| format!("Failed to build file types: {e}"))?;
        walk_builder.types(types);
    }

    // Apply glob filter
    if let Some(ref glob_pattern) = args.glob {
        let mut override_builder = OverrideBuilder::new(root);
        override_builder
            .add(glob_pattern)
            .map_err(|e| format!("Invalid glob pattern: {e}"))?;
        let overrides = override_builder
            .build()
            .map_err(|e| format!("Failed to build glob override: {e}"))?;
        walk_builder.overrides(overrides);
    }

    // If root is a file, search just that file
    let is_single_file = root.is_file();

    let all_matches: Arc<Mutex<Vec<GrepMatch>>> = Arc::new(Mutex::new(Vec::new()));
    let mut files_searched: usize = 0;
    let mut files_with_matches: Vec<String> = Vec::new();
    let mut count_entries: Vec<GrepCountEntry> = Vec::new();
    let mut file_mtimes: HashMap<String, SystemTime> = HashMap::new();
    let mut inaccessible_count: usize = 0;

    if is_single_file {
        files_searched = 1;
        let mut searcher = searcher_builder.clone();
        let mut sink = CollectorSink {
            path: root.to_path_buf(),
            matches: Arc::clone(&all_matches),
            match_count: 0,
        };

        searcher
            .search_path(&matcher, root, &mut sink)
            .map_err(|e| format!("Search error: {e}"))?;

        if sink.match_count > 0 {
            let path_str = root.to_string_lossy().to_string();
            if let Ok(meta) = std::fs::metadata(root) {
                if let Ok(mtime) = meta.modified() {
                    file_mtimes.insert(path_str.clone(), mtime);
                }
            }
            files_with_matches.push(path_str.clone());
            count_entries.push(GrepCountEntry {
                path: path_str,
                count: sink.match_count,
            });
        }
    } else {
        let walker = walk_builder.build();

        for entry in walker {
            let entry = match entry {
                Ok(e) => e,
                Err(_) => {
                    inaccessible_count += 1;
                    continue;
                }
            };

            let path = entry.path();
            if !path.is_file() {
                continue;
            }

            files_searched += 1;

            let mut searcher = searcher_builder.clone();
            let mut sink = CollectorSink {
                path: path.to_path_buf(),
                matches: Arc::clone(&all_matches),
                match_count: 0,
            };

            // Silently skip files that can't be searched (binary, etc.)
            if searcher.search_path(&matcher, path, &mut sink).is_ok() && sink.match_count > 0 {
                let path_str = path.to_string_lossy().to_string();
                if let Ok(meta) = std::fs::metadata(path) {
                    if let Ok(mtime) = meta.modified() {
                        file_mtimes.insert(path_str.clone(), mtime);
                    }
                }
                files_with_matches.push(path_str.clone());
                count_entries.push(GrepCountEntry {
                    path: path_str,
                    count: sink.match_count,
                });
            }
        }
    }

    // Sort files_with_matches by mtime descending (newest first)
    files_with_matches.sort_by(|a, b| {
        let time_a = file_mtimes
            .get(a)
            .copied()
            .unwrap_or(SystemTime::UNIX_EPOCH);
        let time_b = file_mtimes
            .get(b)
            .copied()
            .unwrap_or(SystemTime::UNIX_EPOCH);
        time_b.cmp(&time_a)
    });

    // Sort count_entries by mtime descending
    count_entries.sort_by(|a, b| {
        let time_a = file_mtimes
            .get(&a.path)
            .copied()
            .unwrap_or(SystemTime::UNIX_EPOCH);
        let time_b = file_mtimes
            .get(&b.path)
            .copied()
            .unwrap_or(SystemTime::UNIX_EPOCH);
        time_b.cmp(&time_a)
    });

    // Sort content matches by file mtime descending
    if let Ok(mut matches) = all_matches.lock() {
        matches.sort_by(|a, b| {
            let time_a = file_mtimes
                .get(&a.path)
                .copied()
                .unwrap_or(SystemTime::UNIX_EPOCH);
            let time_b = file_mtimes
                .get(&b.path)
                .copied()
                .unwrap_or(SystemTime::UNIX_EPOCH);
            let time_cmp = time_b.cmp(&time_a);
            if time_cmp == std::cmp::Ordering::Equal {
                a.line_number.cmp(&b.line_number)
            } else {
                time_cmp
            }
        });
    }

    // Build result based on output mode
    let (matches, total_matches, truncated) = match output_mode {
        OutputMode::Content => {
            let collected = all_matches
                .lock()
                .map_err(|e| format!("Lock error: {e}"))?
                .clone();
            let total = collected.len();
            let truncated = total > max_results;
            let items: Vec<serde_json::Value> = collected
                .into_iter()
                .take(max_results)
                .map(|m| serde_json::to_value(m).unwrap_or(serde_json::Value::Null))
                .collect();
            (items, total, truncated)
        }
        OutputMode::FilesWithMatches => {
            let total = files_with_matches.len();
            let truncated = total > max_results;
            let items: Vec<serde_json::Value> = files_with_matches
                .into_iter()
                .take(max_results)
                .map(|p| serde_json::json!(p))
                .collect();
            (items, total, truncated)
        }
        OutputMode::Count => {
            let total = count_entries.len();
            let truncated = total > max_results;
            let items: Vec<serde_json::Value> = count_entries
                .into_iter()
                .take(max_results)
                .map(|c| serde_json::to_value(c).unwrap_or(serde_json::Value::Null))
                .collect();
            (items, total, truncated)
        }
    };

    let note = if inaccessible_count > 0 {
        Some("(Some paths were inaccessible and skipped)".to_string())
    } else {
        None
    };

    Ok(GrepResult {
        matches,
        total_matches,
        files_searched,
        truncated,
        note,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    #[test]
    fn test_grep_description_is_rich() {
        let tool = GrepTool::new();
        let desc = tool.description();
        assert!(!desc.is_empty());
        assert!(desc.contains("regex"), "should mention regex support");
        assert!(
            desc.contains("modification time"),
            "should mention sort order"
        );
        assert!(desc.contains("Agent"), "should mention cross-tool guidance");
    }

    #[tokio::test]
    async fn test_grep_files_with_matches() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("a.txt"), "hello world\nfoo bar\n").expect("write");
        fs::write(dir_path.join("b.txt"), "goodbye world\nbaz qux\n").expect("write");
        fs::write(dir_path.join("c.txt"), "no match here\n").expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "world",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "files_with_matches"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 2);
        assert_eq!(parsed.files_searched, 3);
        assert!(!parsed.truncated);
    }

    #[tokio::test]
    async fn test_grep_content_mode() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(
            dir_path.join("test.txt"),
            "line one\nline two\nline three\n",
        )
        .expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "two",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "content"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 1);
        let first = &parsed.matches[0];
        assert_eq!(first["content"], "line two");
        assert_eq!(first["line_number"], 2);
    }

    #[tokio::test]
    async fn test_grep_count_mode() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(
            dir_path.join("test.txt"),
            "apple\nbanana\napple pie\napple sauce\n",
        )
        .expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "apple",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "count"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 1); // 1 file has matches
        let first = &parsed.matches[0];
        assert_eq!(first["count"], 3);
    }

    #[tokio::test]
    async fn test_grep_case_insensitive() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("test.txt"), "Hello\nhello\nHELLO\n").expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "hello",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "content",
            "case_insensitive": true
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 3);
    }

    #[tokio::test]
    async fn test_grep_with_context() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(
            dir_path.join("test.txt"),
            "line 1\nline 2\nmatch here\nline 4\nline 5\n",
        )
        .expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "match here",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "content",
            "context": 1
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        // Should have: 1 context before, 1 match, 1 context after = 3 entries
        assert_eq!(parsed.total_matches, 3);
        // The match itself
        let match_entries: Vec<_> = parsed
            .matches
            .iter()
            .filter(|m| !m["is_context"].as_bool().unwrap_or(true))
            .collect();
        assert_eq!(match_entries.len(), 1);
        assert_eq!(match_entries[0]["content"], "match here");
    }

    #[tokio::test]
    async fn test_grep_with_glob_filter() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("test.rs"), "fn main() {}\n").expect("write");
        fs::write(dir_path.join("test.txt"), "fn main() {}\n").expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "fn main",
            "path": dir_path.to_str().unwrap(),
            "glob": "*.rs",
            "output_mode": "files_with_matches"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 1);
        assert!(parsed.matches[0]
            .as_str()
            .unwrap_or("")
            .ends_with("test.rs"));
    }

    #[tokio::test]
    async fn test_grep_invalid_regex() {
        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "[invalid",
            "path": "/tmp"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Invalid regex pattern"));
    }

    #[tokio::test]
    async fn test_grep_nonexistent_path() {
        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "test",
            "path": "/tmp/nonexistent_grep_test_xyz"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("does not exist"));
    }

    #[tokio::test]
    async fn test_grep_no_matches() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("test.txt"), "hello world\n").expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "zzzznotfound",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "files_with_matches"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 0);
        assert!(parsed.matches.is_empty());
    }

    #[tokio::test]
    async fn test_grep_single_file() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("single.txt");
        fs::write(&file_path, "alpha\nbeta\ngamma\n").expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "beta",
            "path": file_path.to_str().unwrap(),
            "output_mode": "content"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 1);
        assert_eq!(parsed.files_searched, 1);
        assert_eq!(parsed.matches[0]["content"], "beta");
    }

    #[tokio::test]
    async fn test_grep_results_sorted_by_mtime() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        // Create files with different mtimes
        fs::write(dir_path.join("old.txt"), "match_word here\n").expect("write");
        // Sleep to ensure different mtime
        std::thread::sleep(std::time::Duration::from_millis(50));
        fs::write(dir_path.join("new.txt"), "match_word here\n").expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "match_word",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "files_with_matches"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 2);
        // Newest file should appear first
        let first = parsed.matches[0].as_str().unwrap();
        assert!(
            first.ends_with("new.txt"),
            "newest file should be first, got: {first}"
        );
    }

    #[tokio::test]
    async fn test_grep_long_line_truncation() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        let long_line = format!("prefix_{}_suffix", "x".repeat(3000));
        fs::write(dir_path.join("long.txt"), &long_line).expect("write");

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "prefix_",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "content"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 1);
        let content = parsed.matches[0]["content"].as_str().unwrap();
        assert!(
            content.ends_with("..."),
            "long line should be truncated with ..."
        );
        // Should be 2000 + 3 for "..."
        assert!(content.len() <= 2003 + 10);
    }

    #[tokio::test]
    async fn test_grep_default_max_is_100() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        // Create 150 files each with a match
        for i in 0..150 {
            fs::write(dir_path.join(format!("f{i:03}.txt")), "findme\n").expect("write");
        }

        let tool = GrepTool::new();
        let args = serde_json::json!({
            "pattern": "findme",
            "path": dir_path.to_str().unwrap(),
            "output_mode": "files_with_matches"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert!(parsed.truncated, "should be truncated at default 100");
        assert_eq!(parsed.matches.len(), 100);
        assert!(parsed.total_matches > 100);
    }

    #[tokio::test]
    async fn test_grep_default_output_mode() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("test.txt"), "hello\n").expect("write");

        let tool = GrepTool::new();
        // No output_mode specified - should default to files_with_matches
        let args = serde_json::json!({
            "pattern": "hello",
            "path": dir_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: GrepResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_matches, 1);
        // In files_with_matches mode, matches are file paths
        assert!(parsed.matches[0]
            .as_str()
            .unwrap_or("")
            .ends_with("test.txt"));
    }
}
