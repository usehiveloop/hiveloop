use std::path::{Path, PathBuf};

use bridge_core::SkillDefinition;
use tracing::warn;

/// File extensions that should be marked executable after writing.
const EXECUTABLE_EXTENSIONS: &[&str] = &["sh", "py", "rb"];

/// Return the on-disk directory for a skill: `<base_dir>/.skills/<skill_id>`.
pub fn skill_dir_path(skill_id: &str, base_dir: &Path) -> PathBuf {
    base_dir.join(".skills").join(skill_id)
}

/// Write every skill's `files` map to disk under `.skills/<skill-id>/`.
///
/// Skips skills with no files. Logs warnings on individual failures but never
/// blocks skill loading.
pub async fn write_skill_files(skills: &[SkillDefinition], base_dir: &Path) {
    for skill in skills {
        if skill.files.is_empty() {
            continue;
        }
        let skill_dir = skill_dir_path(&skill.id, base_dir);

        for (relative_path, content) in &skill.files {
            let file_path = skill_dir.join(relative_path);

            // Path-traversal guard: the resolved path must stay inside skill_dir.
            match file_path.canonicalize().ok().or_else(|| {
                // File doesn't exist yet — normalize manually by checking the
                // joined path starts with the skill dir prefix.
                let normalized = skill_dir.join(relative_path);
                // Reject paths that contain `..` components.
                if relative_path.contains("..") {
                    None
                } else {
                    Some(normalized)
                }
            }) {
                Some(resolved) if resolved.starts_with(&skill_dir) => {}
                Some(_) | None => {
                    warn!(
                        skill_id = %skill.id,
                        path = %relative_path,
                        "skill file path escapes skill directory — skipping"
                    );
                    continue;
                }
            }

            // Create parent directories.
            if let Some(parent) = file_path.parent() {
                if let Err(e) = tokio::fs::create_dir_all(parent).await {
                    warn!(
                        skill_id = %skill.id,
                        path = %relative_path,
                        error = %e,
                        "failed to create directory for skill file"
                    );
                    continue;
                }
            }

            // Write the file content.
            if let Err(e) = tokio::fs::write(&file_path, content).await {
                warn!(
                    skill_id = %skill.id,
                    path = %relative_path,
                    error = %e,
                    "failed to write skill file"
                );
                continue;
            }

            // Mark scripts executable (Unix only).
            #[cfg(unix)]
            {
                let is_executable = Path::new(relative_path)
                    .extension()
                    .and_then(|e| e.to_str())
                    .map(|ext| EXECUTABLE_EXTENSIONS.contains(&ext))
                    .unwrap_or(false);

                if is_executable {
                    use std::os::unix::fs::PermissionsExt;
                    let perms = std::fs::Permissions::from_mode(0o755);
                    if let Err(e) = tokio::fs::set_permissions(&file_path, perms).await {
                        warn!(
                            skill_id = %skill.id,
                            path = %relative_path,
                            error = %e,
                            "failed to set executable permission on skill file"
                        );
                    }
                }
            }
        }
    }
}

/// Remove `.skills/<skill-id>/` directories for the given skill IDs.
///
/// If `.skills/` is empty afterwards, removes it too.
/// Logs warnings on failures but never propagates errors.
pub async fn cleanup_skill_files(skill_ids: &[&str], base_dir: &Path) {
    let skills_root = base_dir.join(".skills");

    for skill_id in skill_ids {
        let dir = skills_root.join(skill_id);
        if dir.exists() {
            if let Err(e) = tokio::fs::remove_dir_all(&dir).await {
                warn!(
                    skill_id = %skill_id,
                    error = %e,
                    "failed to remove skill directory"
                );
            }
        }
    }

    // Clean up the .skills/ root if it's now empty.
    if skills_root.exists() {
        if let Ok(mut entries) = tokio::fs::read_dir(&skills_root).await {
            if entries.next_entry().await.ok().flatten().is_none() {
                let _ = tokio::fs::remove_dir(&skills_root).await;
            }
        }
    }
}
