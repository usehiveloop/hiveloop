//! Filesystem scanners for each skill-source layout.

use bridge_core::skill::{SkillDefinition, SkillSource};
use std::collections::HashSet;
use std::path::Path;
use tokio::fs;
use tracing::debug;

use super::helpers::{read_sibling_files, slug_to_title};
use super::parsers::{parse_plain_md, parse_skill_md};

/// Discover directory-based skills (e.g., `.claude/skills/deploy/SKILL.md`).
///
/// Each subdirectory with a `SKILL.md` becomes a skill. Sibling files are
/// read into the `files` map for lazy-loading.
pub(super) async fn discover_directory_skills(
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
pub(super) async fn discover_file_skills_with_frontmatter(
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
pub(super) async fn discover_plain_md_files(
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
pub(super) async fn discover_single_file_skill(
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
