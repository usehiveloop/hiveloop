//! Markdown / frontmatter parsers for skill files.

use bridge_core::skill::{SkillDefinition, SkillFrontmatter, SkillSource};
use std::collections::HashMap;

use super::helpers::{extract_frontmatter, first_paragraph, slug_to_title};

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
