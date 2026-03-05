use crate::protocol::ToolResult;
use std::path::{Path, PathBuf};

/// Execute the Glob tool against the workspace directory.
pub fn glob(workspace: &Path, pattern: &str) -> ToolResult {
    let glob = match globset::Glob::new(pattern) {
        Ok(g) => g.compile_matcher(),
        Err(e) => return ToolResult::error(format!("invalid glob pattern: {e}")),
    };

    let mut matches = Vec::new();
    collect_glob_matches(workspace, workspace, &glob, &mut matches);
    matches.sort();

    if matches.is_empty() {
        ToolResult::text("No files matched the pattern.".into())
    } else {
        ToolResult::text(matches.join("\n"))
    }
}

fn collect_glob_matches(
    root: &Path,
    dir: &Path,
    matcher: &globset::GlobMatcher,
    results: &mut Vec<String>,
) {
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();

        // Skip hidden dirs and target/node_modules
        if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
            if name.starts_with('.') || name == "target" || name == "node_modules" {
                continue;
            }
        }

        if let Ok(rel) = path.strip_prefix(root) {
            let rel_str = rel.to_string_lossy();
            if matcher.is_match(rel_str.as_ref()) {
                results.push(rel_str.into_owned());
            }
        }

        if path.is_dir() {
            collect_glob_matches(root, &path, matcher, results);
        }
    }
}

/// Execute the Grep tool against the workspace directory.
pub fn grep(
    workspace: &Path,
    pattern: &str,
    file_glob: Option<&str>,
    subpath: Option<&str>,
) -> ToolResult {
    let re = match regex::Regex::new(pattern) {
        Ok(r) => r,
        Err(e) => return ToolResult::error(format!("invalid regex: {e}")),
    };

    let glob_matcher =
        file_glob.and_then(|g| globset::Glob::new(g).ok().map(|g| g.compile_matcher()));

    let search_root = match subpath {
        Some(p) => workspace.join(p),
        None => workspace.to_path_buf(),
    };

    let mut results = Vec::new();
    grep_recursive(
        &search_root,
        workspace,
        &re,
        glob_matcher.as_ref(),
        &mut results,
    );

    if results.is_empty() {
        ToolResult::text("No matches found.".into())
    } else {
        // Limit output to avoid huge responses
        let truncated = results.len() > 100;
        let output: Vec<String> = results.into_iter().take(100).collect();
        let mut text = output.join("\n");
        if truncated {
            text.push_str("\n... (results truncated)");
        }
        ToolResult::text(text)
    }
}

fn grep_recursive(
    dir: &Path,
    workspace_root: &Path,
    re: &regex::Regex,
    glob_matcher: Option<&globset::GlobMatcher>,
    results: &mut Vec<String>,
) {
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();

        if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
            if name.starts_with('.') || name == "target" || name == "node_modules" {
                continue;
            }
        }

        if path.is_dir() {
            grep_recursive(&path, workspace_root, re, glob_matcher, results);
            continue;
        }

        if !path.is_file() {
            continue;
        }

        let rel = match path.strip_prefix(workspace_root) {
            Ok(r) => r,
            Err(_) => continue,
        };

        // Check file glob filter
        if let Some(matcher) = glob_matcher {
            if !matcher.is_match(rel.to_string_lossy().as_ref()) {
                // Also try matching just the file name
                if let Some(name) = rel.file_name() {
                    if !matcher.is_match(name.to_string_lossy().as_ref()) {
                        continue;
                    }
                } else {
                    continue;
                }
            }
        }

        // Read and search
        let content = match std::fs::read_to_string(&path) {
            Ok(c) => c,
            Err(_) => continue, // skip binary/unreadable files
        };

        for (line_num, line) in content.lines().enumerate() {
            if re.is_match(line) {
                results.push(format!("{}:{}:{}", rel.display(), line_num + 1, line));
                if results.len() >= 200 {
                    return;
                }
            }
        }
    }
}

/// Execute the Read tool against the workspace directory.
pub fn read(workspace: &Path, file_path: &str) -> ToolResult {
    let target = resolve_path(workspace, file_path);

    if !target.starts_with(workspace) {
        return ToolResult::error("path traversal not allowed".into());
    }

    match std::fs::read_to_string(&target) {
        Ok(content) => {
            // Add line numbers
            let numbered: Vec<String> = content
                .lines()
                .enumerate()
                .map(|(i, line)| format!("{:>4}\t{}", i + 1, line))
                .collect();
            ToolResult::text(numbered.join("\n"))
        }
        Err(e) => ToolResult::error(format!("failed to read {}: {e}", file_path)),
    }
}

fn resolve_path(workspace: &Path, file_path: &str) -> PathBuf {
    let p = Path::new(file_path);
    if p.is_absolute() {
        p.to_path_buf()
    } else {
        workspace.join(p)
    }
}
