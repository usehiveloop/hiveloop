use regex::Regex;
use std::collections::HashMap;

/// Substitute template variables in skill content.
///
/// Supported variables:
/// - `{{args}}` and `$ARGUMENTS` → full args string
/// - `$1`, `$2`, ... → positional args (whitespace-split)
/// - `${CLAUDE_SKILL_DIR}` → skill directory path (e.g. `.skills/my-skill`)
pub(super) fn substitute_variables(content: &str, args: Option<&str>, skill_dir: &str) -> String {
    let mut result = content.to_string();

    // Always substitute ${CLAUDE_SKILL_DIR}
    result = result.replace("${CLAUDE_SKILL_DIR}", skill_dir);

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
pub(super) fn resolve_file_references(content: &str, files: &HashMap<String, String>) -> String {
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
