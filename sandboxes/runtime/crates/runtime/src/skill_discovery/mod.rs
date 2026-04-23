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

use bridge_core::skill::{SkillDefinition, SkillSource};
use std::collections::HashSet;
use std::path::Path;

mod discoverers;
mod helpers;
mod parsers;
#[cfg(test)]
mod tests;

pub use parsers::{parse_plain_md, parse_skill_md};

use discoverers::{
    discover_directory_skills, discover_file_skills_with_frontmatter, discover_plain_md_files,
    discover_single_file_skill,
};

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
