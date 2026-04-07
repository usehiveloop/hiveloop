//! Multi-source skill discovery from the working directory.
//!
//! Scans well-known directories for skill definitions from multiple AI tools:
//! - `.claude/skills/*/SKILL.md` and `.claude/commands/*.md` (Claude Code)
//! - `.agent/skills/*/SKILL.md` (Agent Skills)
//! - `.cursor/rules/*.md` and `.cursorrules` (Cursor)
//! - `.github/copilot-instructions.md` (GitHub Copilot)
//! - `.windsurf/rules/*.md` and `.windsurfrules` (Windsurf)
//!
//! Skills from higher-priority sources take precedence when ids collide.

use bridge_core::skill::{SkillDefinition, SkillFrontmatter, SkillSource};
use std::collections::{HashMap, HashSet};
use std::path::Path;
use tokio::fs;
use tracing::debug;

/// Maximum file size we'll read for a supporting file (1 MB).
const MAX_FILE_SIZE: u64 = 1_048_576;

/// Discover skills from all known sources in the working directory.
///
/// Returns a deduplicated list of skills. When the same skill id appears in
/// multiple sources, the earlier (higher-priority) source wins.
pub async fn discover_skills(working_dir: &Path) -> Vec<SkillDefinition> {
    let mut skills = Vec::new();
    let mut seen_ids: HashSet<String> = HashSet::new();

    // 1. .claude/skills/*/SKILL.md (multi-file, frontmatter)
    discover_directory_skills(
        working_dir,
        ".claude/skills",
        SkillSource::ClaudeCode,
        &mut skills,
        &mut seen_ids,
    )
    .await;

    // 2. .claude/commands/*.md (single-file, frontmatter)
    discover_file_skills_with_frontmatter(
        working_dir,
        ".claude/commands",
        SkillSource::ClaudeCode,
        &mut skills,
        &mut seen_ids,
    )
    .await;

    // 3. .agent/skills/*/SKILL.md (multi-file, frontmatter)
    discover_directory_skills(
        working_dir,
        ".agent/skills",
        SkillSource::AgentSkills,
        &mut skills,
        &mut seen_ids,
    )
    .await;

    // 4. .cursor/rules/*.md + .cursorrules (plain markdown)
    discover_plain_md_files(
        working_dir,
        ".cursor/rules",
        SkillSource::CursorRules,
        &mut skills,
        &mut seen_ids,
    )
    .await;
    discover_single_file_skill(
        working_dir,
        ".cursorrules",
        "cursorrules",
        "Cursor Rules",
        SkillSource::CursorRules,
        &mut skills,
        &mut seen_ids,
    )
    .await;

    // 5. .github/copilot-instructions.md (single file)
    discover_single_file_skill(
        working_dir,
        ".github/copilot-instructions.md",
        "copilot-instructions",
        "Copilot Instructions",
        SkillSource::GitHubCopilot,
        &mut skills,
        &mut seen_ids,
    )
    .await;

    // 6. .windsurf/rules/*.md + .windsurfrules (plain markdown)
    discover_plain_md_files(
        working_dir,
        ".windsurf/rules",
        SkillSource::WindsurfRules,
        &mut skills,
        &mut seen_ids,
    )
    .await;
    discover_single_file_skill(
        working_dir,
        ".windsurfrules",
        "windsurfrules",
        "Windsurf Rules",
        SkillSource::WindsurfRules,
        &mut skills,
        &mut seen_ids,
    )
    .await;

    skills
}

/// Discover directory-based skills (e.g., `.claude/skills/deploy/SKILL.md`).
///
/// Each subdirectory with a `SKILL.md` becomes a skill. Sibling files are
/// read into the `files` map for lazy-loading.
async fn discover_directory_skills(
    working_dir: &Path,
    relative_dir: &str,
    source: SkillSource,
    skills: &mut Vec<SkillDefinition>,
    seen_ids: &mut HashSet<String>,
) {
    let skills_dir = working_dir.join(relative_dir);
    let Ok(mut entries) = fs::read_dir(&skills_dir).await else {
        return;
    };

    while let Ok(Some(entry)) = entries.next_entry().await {
        let path = entry.path();
        if !path.is_dir() {
            continue;
        }

        let skill_md_path = path.join("SKILL.md");
        let Ok(raw) = fs::read_to_string(&skill_md_path).await else {
            continue;
        };

        let dir_name = path
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("unknown");

        if seen_ids.contains(dir_name) {
            debug!(skill_id = dir_name, source = ?source, "skipping duplicate skill");
            continue;
        }

        let mut skill = parse_skill_md(&raw, dir_name, source.clone());

        // Read sibling files into the files map
        skill.files = read_sibling_files(&path).await;

        seen_ids.insert(skill.id.clone());
        skills.push(skill);
    }
}

/// Discover single-file skills with YAML frontmatter (e.g., `.claude/commands/*.md`).
async fn discover_file_skills_with_frontmatter(
    working_dir: &Path,
    relative_dir: &str,
    source: SkillSource,
    skills: &mut Vec<SkillDefinition>,
    seen_ids: &mut HashSet<String>,
) {
    let dir = working_dir.join(relative_dir);
    let Ok(mut entries) = fs::read_dir(&dir).await else {
        return;
    };

    while let Ok(Some(entry)) = entries.next_entry().await {
        let path = entry.path();
        let is_md = path
            .extension()
            .map(|e| e == "md" || e == "markdown")
            .unwrap_or(false);
        if !is_md {
            continue;
        }

        let Ok(raw) = fs::read_to_string(&path).await else {
            continue;
        };

        let id = path
            .file_stem()
            .and_then(|n| n.to_str())
            .unwrap_or("unknown");

        if seen_ids.contains(id) {
            continue;
        }

        let skill = parse_skill_md(&raw, id, source.clone());
        seen_ids.insert(skill.id.clone());
        skills.push(skill);
    }
}

/// Discover plain markdown files without frontmatter (e.g., `.cursor/rules/*.md`).
async fn discover_plain_md_files(
    working_dir: &Path,
    relative_dir: &str,
    source: SkillSource,
    skills: &mut Vec<SkillDefinition>,
    seen_ids: &mut HashSet<String>,
) {
    let dir = working_dir.join(relative_dir);
    let Ok(mut entries) = fs::read_dir(&dir).await else {
        return;
    };

    while let Ok(Some(entry)) = entries.next_entry().await {
        let path = entry.path();
        let is_md = path
            .extension()
            .map(|e| e == "md" || e == "markdown")
            .unwrap_or(false);
        if !is_md {
            continue;
        }

        let Ok(raw) = fs::read_to_string(&path).await else {
            continue;
        };

        let id = path
            .file_stem()
            .and_then(|n| n.to_str())
            .unwrap_or("unknown");

        if seen_ids.contains(id) {
            continue;
        }

        let title = slug_to_title(id);
        let skill = parse_plain_md(&raw, id, &title, source.clone());
        seen_ids.insert(skill.id.clone());
        skills.push(skill);
    }
}

/// Discover a single file as a skill (e.g., `.github/copilot-instructions.md`).
async fn discover_single_file_skill(
    working_dir: &Path,
    relative_path: &str,
    id: &str,
    title: &str,
    source: SkillSource,
    skills: &mut Vec<SkillDefinition>,
    seen_ids: &mut HashSet<String>,
) {
    if seen_ids.contains(id) {
        return;
    }

    let path = working_dir.join(relative_path);
    let Ok(raw) = fs::read_to_string(&path).await else {
        return;
    };

    let skill = parse_plain_md(&raw, id, title, source);
    seen_ids.insert(skill.id.clone());
    skills.push(skill);
}

// ---------------------------------------------------------------------------
// Parsers
// ---------------------------------------------------------------------------

/// Parse a SKILL.md file with optional YAML frontmatter.
///
/// Frontmatter is delimited by `---` at the start of the file. If no
/// frontmatter is found, the entire content is treated as the body.
pub fn parse_skill_md(raw: &str, dir_name: &str, source: SkillSource) -> SkillDefinition {
    let (frontmatter, body) = extract_frontmatter(raw);

    // Parse frontmatter YAML
    let fm: SkillFrontmatter = frontmatter
        .and_then(|yaml| serde_yaml::from_str(yaml).ok())
        .unwrap_or_default();

    // Extract name/description from frontmatter, falling back to dir_name / first paragraph
    let raw_fm: HashMap<String, serde_yaml::Value> = frontmatter
        .and_then(|yaml| serde_yaml::from_str(yaml).ok())
        .unwrap_or_default();

    let title = raw_fm
        .get("name")
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
        .unwrap_or_else(|| slug_to_title(dir_name));

    let description = raw_fm
        .get("description")
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
        .unwrap_or_else(|| first_paragraph(body));

    SkillDefinition {
        id: dir_name.to_string(),
        title,
        description,
        content: body.to_string(),
        frontmatter: Some(fm),
        source,
        ..Default::default()
    }
}

/// Parse a plain markdown file (no frontmatter) as a skill.
pub fn parse_plain_md(raw: &str, id: &str, title: &str, source: SkillSource) -> SkillDefinition {
    SkillDefinition {
        id: id.to_string(),
        title: title.to_string(),
        description: first_paragraph(raw),
        content: raw.to_string(),
        source,
        ..Default::default()
    }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Extract YAML frontmatter and body from a raw string.
///
/// Returns `(Some(yaml_str), body)` if frontmatter is found, `(None, raw)` otherwise.
fn extract_frontmatter(raw: &str) -> (Option<&str>, &str) {
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
fn first_paragraph(text: &str) -> String {
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
fn slug_to_title(slug: &str) -> String {
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
async fn read_sibling_files(dir: &Path) -> HashMap<String, String> {
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
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
        assert_eq!(slug_to_title("code-review"), "Code Review");
        assert_eq!(slug_to_title("my_cool_skill"), "My Cool Skill");
        assert_eq!(slug_to_title("deploy"), "Deploy");
    }

    #[test]
    fn first_paragraph_basic() {
        assert_eq!(
            first_paragraph("Hello world.\n\nSecond para."),
            "Hello world."
        );
    }

    #[test]
    fn first_paragraph_skips_heading() {
        assert_eq!(
            first_paragraph("# Title\n\nFirst real paragraph."),
            "First real paragraph."
        );
    }

    #[test]
    fn first_paragraph_truncates_long() {
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
}
