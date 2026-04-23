use super::common::{make_multi_file_skill, make_skills};
use crate::skill_tools::SkillTool;
use crate::ToolExecutor;
use bridge_core::{SkillDefinition, SkillSource};

#[test]
fn description_is_static_from_file() {
    let tool = SkillTool::new(make_skills());
    let desc = tool.description();

    // Description now comes from static file, not dynamically generated
    assert!(desc.contains("Execute a skill within the main conversation"));
    assert!(desc.contains("slash command"));
    assert!(desc.contains("BLOCKING REQUIREMENT"));
    // Skill content should NOT be in the description
    assert!(!desc.contains("You are a code review expert"));
    assert!(!desc.contains("You are a PR summarizer"));
}

#[tokio::test]
async fn execute_returns_full_content_for_valid_skill() {
    let tool = SkillTool::new(make_skills());
    let args = serde_json::json!({ "name": "Code Review" });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("<skill_content"));
    assert!(result.contains("You are a code review expert"));
    assert!(result.contains("Check for bugs"));
}

#[tokio::test]
async fn execute_returns_error_for_unknown_skill() {
    let tool = SkillTool::new(make_skills());
    let args = serde_json::json!({ "name": "nonexistent" });
    let result = tool.execute(args).await;

    assert!(result.is_err());
    let err = result.unwrap_err();
    assert!(err.contains("not found"));
    assert!(err.contains("Code Review"));
    assert!(err.contains("PR Summary"));
}

#[tokio::test]
async fn execute_case_insensitive_matching_by_title() {
    let tool = SkillTool::new(make_skills());
    let args = serde_json::json!({ "name": "code review" });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("You are a code review expert"));
}

#[tokio::test]
async fn execute_matches_by_id() {
    let tool = SkillTool::new(make_skills());
    let args = serde_json::json!({ "name": "pr-summary" });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("You are a PR summarizer"));
}

#[tokio::test]
async fn execute_case_insensitive_matching_by_id() {
    let tool = SkillTool::new(make_skills());
    let args = serde_json::json!({ "name": "PR-SUMMARY" });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("You are a PR summarizer"));
}

#[tokio::test]
async fn execute_with_args_substitutes_template() {
    let skills = vec![SkillDefinition {
        id: "commit".to_string(),
        title: "Commit".to_string(),
        description: "Writes commit messages".to_string(),
        content: "Write a commit message for: {{args}}".to_string(),
        ..Default::default()
    }];
    let tool = SkillTool::new(skills);
    let args = serde_json::json!({
        "name": "commit",
        "args": "fix the login bug"
    });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("<skill_content"));
    assert!(result.contains("Write a commit message for: fix the login bug"));
    // Template variable should be substituted
    assert!(!result.contains("{{args}}"));
}

#[tokio::test]
async fn execute_with_args_no_template_appends_args() {
    let skills = vec![SkillDefinition {
        id: "review".to_string(),
        title: "Review".to_string(),
        description: "Reviews code".to_string(),
        content: "You are a code reviewer. Review the code.".to_string(),
        ..Default::default()
    }];
    let tool = SkillTool::new(skills);
    let args = serde_json::json!({
        "name": "review",
        "args": "PR #123"
    });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("You are a code reviewer. Review the code."));
    assert!(result.contains("Arguments: PR #123"));
}

#[tokio::test]
async fn execute_without_args_ignores_template() {
    let skills = vec![SkillDefinition {
        id: "commit".to_string(),
        title: "Commit".to_string(),
        description: "Writes commit messages".to_string(),
        content: "Write a commit message for: {{args}}".to_string(),
        ..Default::default()
    }];
    let tool = SkillTool::new(skills);
    let args = serde_json::json!({ "name": "commit" });
    let result = tool.execute(args).await.expect("execute");

    // When no args provided, {{args}} remains in content (or skill should handle it)
    assert!(result.contains("Write a commit message for: {{args}}"));
}

#[tokio::test]
async fn execute_with_empty_args_string() {
    let skills = vec![SkillDefinition {
        id: "commit".to_string(),
        title: "Commit".to_string(),
        description: "Writes commit messages".to_string(),
        content: "Write a commit message for: {{args}}".to_string(),
        ..Default::default()
    }];
    let tool = SkillTool::new(skills);
    let args = serde_json::json!({
        "name": "commit",
        "args": ""
    });
    let result = tool.execute(args).await.expect("execute");

    // Empty string should still substitute (removes the placeholder)
    assert!(result.contains("Write a commit message for: "));
    assert!(!result.contains("{{args}}"));
}

#[tokio::test]
async fn execute_resolves_file_references() {
    let tool = SkillTool::new(vec![make_multi_file_skill()]);
    let args = serde_json::json!({ "name": "api-docs" });
    let result = tool.execute(args).await.expect("execute");

    // File references should be inlined
    assert!(result.contains("<skill_file path=\"reference.md\">"));
    assert!(result.contains("GET /users - list users"));
    assert!(result.contains("<skill_file path=\"examples/basic.md\">"));
    assert!(result.contains("curl localhost/users"));
    // Original markdown links should be replaced
    assert!(!result.contains("[reference.md](reference.md)"));
}

#[tokio::test]
async fn execute_file_param_returns_specific_file() {
    let tool = SkillTool::new(vec![make_multi_file_skill()]);
    let args = serde_json::json!({
        "name": "api-docs",
        "file": "reference.md"
    });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("<skill_file"));
    assert!(result.contains("GET /users - list users"));
    // Should NOT contain the main skill content
    assert!(!result.contains("You are an API expert"));
}

#[tokio::test]
async fn execute_file_param_returns_error_for_unknown_file() {
    let tool = SkillTool::new(vec![make_multi_file_skill()]);
    let args = serde_json::json!({
        "name": "api-docs",
        "file": "nonexistent.md"
    });
    let result = tool.execute(args).await;

    assert!(result.is_err());
    let err = result.unwrap_err();
    assert!(err.contains("not found"));
    assert!(err.contains("reference.md"));
}

#[tokio::test]
async fn execute_includes_source_attribute() {
    let mut skill = make_multi_file_skill();
    skill.source = SkillSource::ClaudeCode;
    let tool = SkillTool::new(vec![skill]);
    let args = serde_json::json!({ "name": "api-docs" });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("source=\"claudecode\""));
}

#[tokio::test]
async fn execute_with_dollar_arguments_substitution() {
    let skills = vec![SkillDefinition {
        id: "deploy".to_string(),
        title: "Deploy".to_string(),
        description: "Deploys".to_string(),
        content: "Deploy $ARGUMENTS to production".to_string(),
        ..Default::default()
    }];
    let tool = SkillTool::new(skills);
    let args = serde_json::json!({
        "name": "deploy",
        "args": "my-app"
    });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("Deploy my-app to production"));
    // Should NOT append "Arguments:" since $ARGUMENTS was a placeholder
    assert!(!result.contains("Arguments: my-app"));
}

#[test]
fn skills_getter_returns_all_skills() {
    let skills = make_skills();
    let tool = SkillTool::new(skills.clone());
    assert_eq!(tool.skills().len(), 2);
    assert_eq!(tool.skills()[0].id, "code-review");
}

#[tokio::test]
async fn execute_with_base_dir_prepends_files_note() {
    let tool = SkillTool::with_base_dir(
        vec![make_multi_file_skill()],
        std::path::PathBuf::from("/workspace"),
    );
    let args = serde_json::json!({ "name": "api-docs" });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("NOTE: This skill's files are at /workspace/.skills/api-docs/"));
    assert!(result.contains("Prefix script paths with this directory."));
}

#[tokio::test]
async fn execute_without_base_dir_no_files_note() {
    let tool = SkillTool::new(vec![make_multi_file_skill()]);
    let args = serde_json::json!({ "name": "api-docs" });
    let result = tool.execute(args).await.expect("execute");

    assert!(!result.contains("NOTE: This skill's files are at"));
}

#[tokio::test]
async fn execute_with_base_dir_substitutes_skill_dir() {
    let skill = SkillDefinition {
        id: "deploy".to_string(),
        title: "Deploy".to_string(),
        description: "Deploy helper".to_string(),
        content: "Run ${CLAUDE_SKILL_DIR}/scripts/deploy.sh".to_string(),
        ..Default::default()
    };
    let tool = SkillTool::with_base_dir(vec![skill], std::path::PathBuf::from("/workspace"));
    let args = serde_json::json!({ "name": "deploy" });
    let result = tool.execute(args).await.expect("execute");

    assert!(result.contains("Run /workspace/.skills/deploy/scripts/deploy.sh"));
}
