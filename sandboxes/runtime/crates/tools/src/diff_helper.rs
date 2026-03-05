use similar::TextDiff;

/// Generate a unified diff between old and new file content.
///
/// Returns an empty string if the contents are identical.
pub fn generate_diff(file_path: &str, old: &str, new: &str) -> String {
    if old == new {
        return String::new();
    }

    let diff = TextDiff::from_lines(old, new);
    let unified = diff
        .unified_diff()
        .context_radius(3)
        .header(&format!("a/{file_path}"), &format!("b/{file_path}"))
        .to_string();

    unified
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_diff_basic() {
        let old = "line one\nline two\nline three\n";
        let new = "line one\nline TWO\nline three\n";
        let diff = generate_diff("test.txt", old, new);
        assert!(diff.contains("--- a/test.txt"));
        assert!(diff.contains("+++ b/test.txt"));
        assert!(diff.contains("-line two"));
        assert!(diff.contains("+line TWO"));
    }

    #[test]
    fn test_generate_diff_identical() {
        let content = "same content\n";
        let diff = generate_diff("test.txt", content, content);
        assert!(diff.is_empty());
    }

    #[test]
    fn test_generate_diff_addition() {
        let old = "line one\n";
        let new = "line one\nline two\n";
        let diff = generate_diff("test.txt", old, new);
        assert!(diff.contains("+line two"));
    }

    #[test]
    fn test_generate_diff_deletion() {
        let old = "line one\nline two\n";
        let new = "line one\n";
        let diff = generate_diff("test.txt", old, new);
        assert!(diff.contains("-line two"));
    }
}
