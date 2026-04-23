use super::super::substitute::{resolve_file_references, substitute_variables};
use std::collections::HashMap;

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

#[test]
fn substitute_variables_with_base_dir_skill_dir() {
    let result = substitute_variables(
        "Run ${CLAUDE_SKILL_DIR}/script.sh",
        None,
        ".skills/my-skill",
    );
    assert_eq!(result, "Run .skills/my-skill/script.sh");
}
