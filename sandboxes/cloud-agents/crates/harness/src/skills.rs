//! Write skill files into the harness's discovery directory before session start.

use bridge_core::skill::SkillDefinition;
use std::path::{Component, Path, PathBuf};
use tracing::{error, warn};

/// Resolve a relative skill-file key against the skill directory, rejecting
/// anything that would escape it (absolute paths, `..` components, root or
/// prefix components on Windows).
///
/// Returns `None` if the path is unsafe — the caller skips and logs.
/// Empty paths and lone `.` components are also rejected.
fn safe_join(base: &Path, rel: &str) -> Option<PathBuf> {
    if rel.is_empty() {
        return None;
    }
    let mut resolved = PathBuf::new();
    for component in Path::new(rel).components() {
        match component {
            Component::Normal(part) => resolved.push(part),
            // Reject anything that could escape or absolutize the path.
            Component::ParentDir
            | Component::RootDir
            | Component::Prefix(_)
            | Component::CurDir => return None,
        }
    }
    if resolved.as_os_str().is_empty() {
        return None;
    }
    Some(base.join(resolved))
}

/// Write each skill as `<root>/skills/<id>/SKILL.md` with optional supporting files.
///
/// Best-effort: failures are logged, not propagated. The harness will simply
/// not see the skill if the write fails.
pub fn write_skills(root: &Path, skills: &[SkillDefinition]) {
    let skills_dir = root.join("skills");
    if let Err(e) = std::fs::create_dir_all(&skills_dir) {
        warn!(path = %skills_dir.display(), error = %e, "skills root creation failed");
        return;
    }

    for skill in skills {
        let dir = skills_dir.join(&skill.id);
        if let Err(e) = std::fs::create_dir_all(&dir) {
            warn!(skill = %skill.id, error = %e, "skill dir creation failed");
            continue;
        }

        let mut body = String::new();
        body.push_str("---\n");
        // Both Claude Code and OpenCode key the skill by `name`, which
        // by convention matches the directory name (== skill.id). Title
        // goes nowhere structured — fold it into the body if needed.
        body.push_str(&format!("name: {}\n", skill.id));
        body.push_str(&format!("description: {}\n", skill.description));
        if let Some(fm) = &skill.frontmatter {
            if let Some(eff) = &fm.effort {
                body.push_str(&format!("effort: {}\n", eff));
            }
            if let Some(ctx) = &fm.context {
                body.push_str(&format!("context: {}\n", ctx));
            }
            if let Some(tools) = &fm.allowed_tools {
                body.push_str(&format!("allowed-tools: {}\n", tools.join(",")));
            }
        }
        body.push_str("---\n\n");
        body.push_str(&skill.content);

        let skill_md = dir.join("SKILL.md");
        if let Err(e) = std::fs::write(&skill_md, body) {
            warn!(skill = %skill.id, error = %e, "SKILL.md write failed");
        }

        for (rel, contents) in &skill.files {
            let Some(target) = safe_join(&dir, rel) else {
                // Path traversal attempt — surfaced as an error so it
                // shows up in Sentry / on-call alerts.
                error!(
                    skill = %skill.id,
                    file = rel,
                    "rejecting skill file path: traversal or absolute path detected"
                );
                continue;
            };
            if let Some(parent) = target.parent() {
                let _ = std::fs::create_dir_all(parent);
            }
            if let Err(e) = std::fs::write(&target, contents) {
                error!(skill = %skill.id, file = rel, error = %e, "skill file write failed");
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::safe_join;
    use std::path::Path;

    #[test]
    fn accepts_simple_filename() {
        let r = safe_join(Path::new("/skills/foo"), "SKILL.md").unwrap();
        assert_eq!(r, Path::new("/skills/foo/SKILL.md"));
    }

    #[test]
    fn accepts_nested_subdir() {
        let r = safe_join(Path::new("/skills/foo"), "references/issue-taxonomy.md").unwrap();
        assert_eq!(r, Path::new("/skills/foo/references/issue-taxonomy.md"));
    }

    #[test]
    fn rejects_parent_traversal() {
        assert!(safe_join(Path::new("/skills/foo"), "../bar").is_none());
        assert!(safe_join(Path::new("/skills/foo"), "a/../../bar").is_none());
        assert!(safe_join(Path::new("/skills/foo"), "../../../etc/passwd").is_none());
    }

    #[test]
    fn rejects_absolute() {
        assert!(safe_join(Path::new("/skills/foo"), "/etc/passwd").is_none());
        assert!(safe_join(Path::new("/skills/foo"), "/").is_none());
    }

    #[test]
    fn rejects_empty_and_dot() {
        assert!(safe_join(Path::new("/skills/foo"), "").is_none());
        assert!(safe_join(Path::new("/skills/foo"), ".").is_none());
        assert!(safe_join(Path::new("/skills/foo"), "./").is_none());
    }
}
