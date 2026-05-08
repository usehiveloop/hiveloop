use std::collections::HashSet;
use std::fs;
use std::path::{Path, PathBuf};

use domain::SkillSpec;
use tracing::{info, warn};

pub struct SkillWriter {
    root: PathBuf,
}

impl SkillWriter {
    pub fn new(root: impl Into<PathBuf>) -> Self {
        Self { root: root.into() }
    }

    pub fn sync(&self, specs: &[SkillSpec]) {
        let skills_dir = self.root.join(".skills");
        let known: HashSet<String> = self.write_skills(specs, &skills_dir);
        self.purge_stale(&skills_dir, &known);
    }

    fn write_skills(&self, specs: &[SkillSpec], skills_dir: &Path) -> HashSet<String> {
        let mut known = HashSet::new();
        for spec in specs {
            let dir = skills_dir.join(&spec.name);
            if let Err(e) = fs::create_dir_all(&dir) {
                warn!(skill = %spec.name, error = %e, "failed to create skill dir");
                continue;
            }

            let frontmatter = format!(
                "---\nname: {}\ndescription: {}\ntrigger: {}\n---\n\n{}",
                spec.name,
                spec.description,
                trigger_value(&spec.trigger),
                spec.instructions,
            );

            let skill_md = dir.join("SKILL.md");
            if let Err(e) = fs::write(&skill_md, &frontmatter) {
                warn!(skill = %spec.name, error = %e, "failed to write SKILL.md");
                continue;
            }

            for (rel_path, content) in &spec.files {
                let dest = dir.join(rel_path);
                if let Some(parent) = dest.parent() {
                    let _ = fs::create_dir_all(parent);
                }
                if let Err(e) = fs::write(&dest, content) {
                    warn!(skill = %spec.name, path = %rel_path, error = %e, "failed to write skill file");
                }
            }

            known.insert(spec.name.clone());
            info!(skill = %spec.name, "skill written");
        }
        known
    }

    fn purge_stale(&self, skills_dir: &Path, known: &HashSet<String>) {
        let Ok(entries) = fs::read_dir(skills_dir) else {
            return;
        };
        for entry in entries.flatten() {
            let name = entry.file_name().to_string_lossy().to_string();
            if known.contains(&name) {
                continue;
            }
            if let Err(e) = fs::remove_dir_all(entry.path()) {
                warn!(dir = %name, error = %e, "failed to purge stale skill");
            } else {
                info!(dir = %name, "purged stale skill");
            }
        }
    }
}

fn trigger_value(trigger: &domain::SkillTrigger) -> &str {
    match trigger {
        domain::SkillTrigger::Always => "always",
        domain::SkillTrigger::Keyword { .. } => "keyword",
    }
}
