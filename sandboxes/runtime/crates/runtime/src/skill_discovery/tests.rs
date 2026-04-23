use super::helpers::extract_frontmatter;
use super::*;
use std::fs as std_fs;
use tempfile::TempDir;

#[test]
fn extract_frontmatter_with_yaml() {
    let raw = "---\nname: deploy\ndescription: Deploy app\n---\n\nDeploy instructions here.";
    let (fm, body) = extract_frontmatter(raw);
    assert!(fm.is_some());
    assert!(fm.unwrap().contains("name: deploy"));
    assert_eq!(body, "Deploy instructions here.");
}

#[test]
fn extract_frontmatter_without_yaml() {
    let raw = "Just some content without frontmatter.";
    let (fm, body) = extract_frontmatter(raw);
    assert!(fm.is_none());
    assert_eq!(body, raw);
}

#[test]
fn extract_frontmatter_no_closing_delimiter() {
    let raw = "---\nname: broken\nNo closing delimiter";
    let (fm, body) = extract_frontmatter(raw);
    assert!(fm.is_none());
    assert_eq!(body, raw);
}

#[test]
fn parse_skill_md_with_frontmatter() {
    let raw =
        "---\nname: Code Review\ndescription: Reviews code\n---\n\nYou are a code reviewer.";
    let skill = parse_skill_md(raw, "code-review", SkillSource::ClaudeCode);

    assert_eq!(skill.id, "code-review");
    assert_eq!(skill.title, "Code Review");
    assert_eq!(skill.description, "Reviews code");
    assert_eq!(skill.content, "You are a code reviewer.");
    assert_eq!(skill.source, SkillSource::ClaudeCode);
    assert!(skill.frontmatter.is_some());
}

#[test]
fn parse_skill_md_without_frontmatter() {
    let raw = "You are a code reviewer.\n\nCheck for bugs.";
    let skill = parse_skill_md(raw, "code-review", SkillSource::ClaudeCode);

    assert_eq!(skill.id, "code-review");
    assert_eq!(skill.title, "Code Review");
    assert_eq!(skill.description, "You are a code reviewer.");
    assert_eq!(skill.content, raw);
}

#[test]
fn parse_skill_md_with_full_frontmatter() {
    let raw = "---\nname: Deploy\ndescription: Deploy to prod\nallowed_tools:\n  - bash\n  - read\neffort: high\ncontext: fork\n---\n\nDeploy content.";
    let skill = parse_skill_md(raw, "deploy", SkillSource::ClaudeCode);
    let fm = skill.frontmatter.unwrap();

    assert_eq!(
        fm.allowed_tools,
        Some(vec!["bash".to_string(), "read".to_string()])
    );
    assert_eq!(fm.effort, Some("high".to_string()));
    assert_eq!(fm.context, Some("fork".to_string()));
}

#[test]
fn parse_plain_md_basic() {
    let raw = "Use TypeScript with strict mode.\n\nAlways write tests.";
    let skill = parse_plain_md(
        raw,
        "typescript-rules",
        "TypeScript Rules",
        SkillSource::CursorRules,
    );

    assert_eq!(skill.id, "typescript-rules");
    assert_eq!(skill.title, "TypeScript Rules");
    assert_eq!(skill.description, "Use TypeScript with strict mode.");
    assert_eq!(skill.content, raw);
    assert!(skill.files.is_empty());
    assert!(skill.frontmatter.is_none());
}

#[test]
fn slug_to_title_basic() {
    use super::helpers::slug_to_title;
    assert_eq!(slug_to_title("code-review"), "Code Review");
    assert_eq!(slug_to_title("my_cool_skill"), "My Cool Skill");
    assert_eq!(slug_to_title("deploy"), "Deploy");
}

#[test]
fn first_paragraph_basic() {
    use super::helpers::first_paragraph;
    assert_eq!(
        first_paragraph("Hello world.\n\nSecond para."),
        "Hello world."
    );
}

#[test]
fn first_paragraph_skips_heading() {
    use super::helpers::first_paragraph;
    assert_eq!(
        first_paragraph("# Title\n\nFirst real paragraph."),
        "First real paragraph."
    );
}

#[test]
fn first_paragraph_truncates_long() {
    use super::helpers::first_paragraph;
    let long = "A".repeat(250);
    let result = first_paragraph(&long);
    assert_eq!(result.len(), 200);
    assert!(result.ends_with("..."));
}

#[tokio::test]
async fn discover_skills_from_claude_skills_dir() {
    let tmp = TempDir::new().unwrap();
    let skill_dir = tmp.path().join(".claude/skills/deploy");
    std_fs::create_dir_all(&skill_dir).unwrap();
    std_fs::write(
        skill_dir.join("SKILL.md"),
        "---\nname: Deploy\ndescription: Deploy the app\n---\n\nDeploy instructions.",
    )
    .unwrap();
    std_fs::write(skill_dir.join("runbook.md"), "# Runbook\nStep 1").unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 1);
    assert_eq!(skills[0].id, "deploy");
    assert_eq!(skills[0].title, "Deploy");
    assert_eq!(skills[0].source, SkillSource::ClaudeCode);
    assert_eq!(skills[0].files.len(), 1);
    assert!(skills[0].files.contains_key("runbook.md"));
}

#[tokio::test]
async fn discover_skills_from_cursor_rules() {
    let tmp = TempDir::new().unwrap();
    let rules_dir = tmp.path().join(".cursor/rules");
    std_fs::create_dir_all(&rules_dir).unwrap();
    std_fs::write(rules_dir.join("no-any.md"), "Never use `any` type.").unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 1);
    assert_eq!(skills[0].id, "no-any");
    assert_eq!(skills[0].source, SkillSource::CursorRules);
}

#[tokio::test]
async fn discover_skills_from_cursorrules_file() {
    let tmp = TempDir::new().unwrap();
    std_fs::write(
        tmp.path().join(".cursorrules"),
        "Use functional components.",
    )
    .unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 1);
    assert_eq!(skills[0].id, "cursorrules");
    assert_eq!(skills[0].title, "Cursor Rules");
}

#[tokio::test]
async fn discover_skills_from_copilot_instructions() {
    let tmp = TempDir::new().unwrap();
    let github_dir = tmp.path().join(".github");
    std_fs::create_dir_all(&github_dir).unwrap();
    std_fs::write(
        github_dir.join("copilot-instructions.md"),
        "Use TypeScript strict mode.",
    )
    .unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 1);
    assert_eq!(skills[0].id, "copilot-instructions");
    assert_eq!(skills[0].source, SkillSource::GitHubCopilot);
}

#[tokio::test]
async fn discover_skills_deduplicates_by_id() {
    let tmp = TempDir::new().unwrap();

    // Create same skill id in both .claude/skills/ and .cursor/rules/
    let claude_dir = tmp.path().join(".claude/skills/deploy");
    std_fs::create_dir_all(&claude_dir).unwrap();
    std_fs::write(
        claude_dir.join("SKILL.md"),
        "---\nname: Deploy\ndescription: Claude deploy\n---\n\nClaude version.",
    )
    .unwrap();

    let cursor_dir = tmp.path().join(".cursor/rules");
    std_fs::create_dir_all(&cursor_dir).unwrap();
    std_fs::write(cursor_dir.join("deploy.md"), "Cursor version.").unwrap();

    let skills = discover_skills(tmp.path()).await;
    // Only one "deploy" skill should exist (Claude wins due to higher priority)
    let deploy_skills: Vec<_> = skills.iter().filter(|s| s.id == "deploy").collect();
    assert_eq!(deploy_skills.len(), 1);
    assert_eq!(deploy_skills[0].source, SkillSource::ClaudeCode);
    assert!(deploy_skills[0].content.contains("Claude version"));
}

#[tokio::test]
async fn discover_skills_multi_source() {
    let tmp = TempDir::new().unwrap();

    // Claude skill
    let claude_dir = tmp.path().join(".claude/skills/review");
    std_fs::create_dir_all(&claude_dir).unwrap();
    std_fs::write(claude_dir.join("SKILL.md"), "Review code.").unwrap();

    // Cursor rule
    let cursor_dir = tmp.path().join(".cursor/rules");
    std_fs::create_dir_all(&cursor_dir).unwrap();
    std_fs::write(cursor_dir.join("style.md"), "Use consistent style.").unwrap();

    // Copilot
    let github_dir = tmp.path().join(".github");
    std_fs::create_dir_all(&github_dir).unwrap();
    std_fs::write(github_dir.join("copilot-instructions.md"), "Be helpful.").unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 3);

    let ids: HashSet<_> = skills.iter().map(|s| s.id.as_str()).collect();
    assert!(ids.contains("review"));
    assert!(ids.contains("style"));
    assert!(ids.contains("copilot-instructions"));
}

#[tokio::test]
async fn discover_skills_empty_dir() {
    let tmp = TempDir::new().unwrap();
    let skills = discover_skills(tmp.path()).await;
    assert!(skills.is_empty());
}

#[tokio::test]
async fn discover_skills_from_claude_commands() {
    let tmp = TempDir::new().unwrap();
    let cmd_dir = tmp.path().join(".claude/commands");
    std_fs::create_dir_all(&cmd_dir).unwrap();
    std_fs::write(
        cmd_dir.join("commit.md"),
        "---\nname: Commit\ndescription: Write commits\n---\n\nWrite a commit message.",
    )
    .unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 1);
    assert_eq!(skills[0].id, "commit");
    assert_eq!(skills[0].title, "Commit");
    assert_eq!(skills[0].source, SkillSource::ClaudeCode);
}

#[tokio::test]
async fn discover_skills_from_agent_skills() {
    let tmp = TempDir::new().unwrap();
    let agent_dir = tmp.path().join(".agent/skills/analyze");
    std_fs::create_dir_all(&agent_dir).unwrap();
    std_fs::write(agent_dir.join("SKILL.md"), "Analyze the data.").unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 1);
    assert_eq!(skills[0].id, "analyze");
    assert_eq!(skills[0].source, SkillSource::AgentSkills);
}

#[tokio::test]
async fn discover_skills_from_windsurf_rules() {
    let tmp = TempDir::new().unwrap();
    std_fs::write(tmp.path().join(".windsurfrules"), "Windsurf rules here.").unwrap();

    let skills = discover_skills(tmp.path()).await;
    assert_eq!(skills.len(), 1);
    assert_eq!(skills[0].id, "windsurfrules");
    assert_eq!(skills[0].source, SkillSource::WindsurfRules);
}
