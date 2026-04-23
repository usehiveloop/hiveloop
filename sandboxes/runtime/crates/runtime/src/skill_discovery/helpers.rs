//! Small string & filesystem helpers shared across discoverers.

use std::collections::HashMap;
use std::path::Path;
use tokio::fs;

/// Maximum file size we'll read for a supporting file (1 MB).
pub(super) const MAX_FILE_SIZE: u64 = 1_048_576;

/// Extract YAML frontmatter and body from a raw string.
///
/// Returns `(Some(yaml_str), body)` if frontmatter is found, `(None, raw)` otherwise.
pub(super) fn extract_frontmatter(raw: &str) -> (Option<&str>, &str) {
    let trimmed = raw.trim_start();
    if !trimmed.starts_with("---") {
        return (None, raw);
    }

    // Find the closing ---
    let after_first = &trimmed[3..];
    let rest = after_first.trim_start_matches(['\r', '\n']);
    if let Some(end) = rest.find("\n---") {
        let yaml = &rest[..end];
        let body_start = end + 4; // "\n---".len()
        let body = rest[body_start..].trim_start_matches(['\r', '\n']);
        (Some(yaml), body)
    } else {
        // No closing ---, treat entire content as body
        (None, raw)
    }
}

/// Extract the first paragraph of text (up to ~200 chars) for use as a description.
pub(super) fn first_paragraph(text: &str) -> String {
    let trimmed = text.trim();
    // Skip leading headings
    let content = if trimmed.starts_with('#') {
        trimmed
            .lines()
            .skip_while(|l| l.starts_with('#') || l.trim().is_empty())
            .collect::<Vec<&str>>()
            .join("\n")
    } else {
        trimmed.to_string()
    };

    let para: String = content
        .lines()
        .take_while(|l| !l.trim().is_empty())
        .collect::<Vec<&str>>()
        .join(" ");

    if para.len() > 200 {
        format!("{}...", &para[..197])
    } else {
        para
    }
}

/// Convert a slug like "code-review" to a title like "Code Review".
pub(super) fn slug_to_title(slug: &str) -> String {
    slug.split(['-', '_'])
        .map(|word| {
            let mut chars = word.chars();
            match chars.next() {
                Some(c) => {
                    let mut s = c.to_uppercase().to_string();
                    s.extend(chars);
                    s
                }
                None => String::new(),
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}

/// Read all sibling files in a skill directory (excluding SKILL.md itself).
///
/// Skips files larger than MAX_FILE_SIZE and files that aren't valid UTF-8.
pub(super) async fn read_sibling_files(dir: &Path) -> HashMap<String, String> {
    let mut files = HashMap::new();
    let Ok(mut entries) = fs::read_dir(dir).await else {
        return files;
    };

    while let Ok(Some(entry)) = entries.next_entry().await {
        let path = entry.path();

        // Skip SKILL.md itself
        if path.file_name().map(|n| n == "SKILL.md").unwrap_or(false) {
            continue;
        }

        // Skip directories (no recursion)
        if path.is_dir() {
            continue;
        }

        // Skip files that are too large
        if let Ok(meta) = fs::metadata(&path).await {
            if meta.len() > MAX_FILE_SIZE {
                continue;
            }
        }

        // Try reading as UTF-8 text (skip binary files)
        if let Ok(content) = fs::read_to_string(&path).await {
            let relative = path
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or("unknown");
            files.insert(relative.to_string(), content);
        }
    }

    files
}
