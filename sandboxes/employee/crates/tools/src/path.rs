use std::path::{Path, PathBuf};

use globset::{Glob, GlobSet, GlobSetBuilder};

#[derive(Debug, thiserror::Error)]
#[allow(dead_code)]
pub enum PathPolicyError {
    #[error("path is required but empty")]
    Empty,
    #[error("path `{0}` is not within any allowed root")]
    OutsideAllowedRoots(String),
    #[error("path `{0}` matches a deny glob")]
    DeniedByGlob(String),
    #[error("path `{0}` could not be resolved: {1}")]
    Unresolvable(String, String),
}

pub fn expand_user_path(raw: &str) -> PathBuf {
    let trimmed = raw.trim();
    if let Some(rest) = trimmed.strip_prefix("~/") {
        if let Ok(home) = std::env::var("HOME") {
            return PathBuf::from(home).join(rest);
        }
    }
    if trimmed == "~" {
        if let Ok(home) = std::env::var("HOME") {
            return PathBuf::from(home);
        }
    }
    PathBuf::from(trimmed)
}

pub fn resolve_relative_to(base: &Path, raw: &str) -> PathBuf {
    if raw.trim().is_empty() {
        return base.to_path_buf();
    }
    let expanded = expand_user_path(raw);
    if expanded.is_absolute() {
        expanded
    } else {
        base.join(expanded)
    }
}

pub fn resolve_within_workspace(
    workspace_root: &Path,
    raw: &str,
    allowed_roots: &[String],
) -> Result<PathBuf, PathPolicyError> {
    if raw.trim().is_empty() {
        return Err(PathPolicyError::Empty);
    }
    let resolved = resolve_relative_to(workspace_root, raw);

    let mut effective_roots: Vec<PathBuf> = allowed_roots
        .iter()
        .map(|s| expand_user_path(s))
        .collect();
    if effective_roots.is_empty() {
        effective_roots.push(workspace_root.to_path_buf());
    }

    let canonical = canonicalize_best_effort(&resolved);
    let canonical_roots: Vec<PathBuf> = effective_roots
        .iter()
        .map(|root| canonicalize_best_effort(root))
        .collect();

    let allowed = canonical_roots
        .iter()
        .any(|root| canonical.starts_with(root));
    if !allowed {
        return Err(PathPolicyError::OutsideAllowedRoots(
            canonical.display().to_string(),
        ));
    }
    Ok(canonical)
}

pub fn canonicalize_best_effort(path: &Path) -> PathBuf {
    if let Ok(canonical) = std::fs::canonicalize(path) {
        return canonical;
    }
    if let Some(parent) = path.parent() {
        if let Ok(canonical_parent) = std::fs::canonicalize(parent) {
            if let Some(file_name) = path.file_name() {
                return canonical_parent.join(file_name);
            }
        }
    }
    path.to_path_buf()
}

pub fn build_glob_set(patterns: &[String]) -> GlobSet {
    let mut builder = GlobSetBuilder::new();
    for pattern in patterns {
        if let Ok(glob) = Glob::new(pattern) {
            builder.add(glob);
        }
    }
    builder.build().unwrap_or_else(|_| GlobSet::empty())
}

pub fn enforce_deny_globs(path: &Path, deny_globs: &GlobSet) -> Result<(), PathPolicyError> {
    if deny_globs.is_match(path) {
        return Err(PathPolicyError::DeniedByGlob(path.display().to_string()));
    }
    Ok(())
}
