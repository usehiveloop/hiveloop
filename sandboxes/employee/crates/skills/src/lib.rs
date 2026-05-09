use std::collections::{BTreeMap, BTreeSet, HashSet};
use std::fs;
use std::path::{Component, Path, PathBuf};

use domain::SkillSpec;
use serde_json::{json, Value};
use tracing::{info, warn};
use walkdir::WalkDir;

const ALLOWED_FILE_DIRS: &[&str] = &["references", "templates", "scripts", "assets"];

pub struct SkillWriter {
    root: PathBuf,
}

impl SkillWriter {
    pub fn new(root: impl Into<PathBuf>) -> Self {
        Self { root: root.into() }
    }

    /// Upsert skills supplied by /config into the filesystem skill root.
    ///
    /// This intentionally does not prune missing skills. Runtime skills may be
    /// created by `skill_manage`; a later config push must not wipe learned
    /// procedural memory just because it sends an empty or partial skills array.
    pub fn sync(&self, specs: &[SkillSpec]) {
        let skills_dir = self.root.join(".skills");
        self.write_skills(specs, &skills_dir);
    }

    fn write_skills(&self, specs: &[SkillSpec], skills_dir: &Path) -> HashSet<String> {
        let mut known = HashSet::new();
        for spec in specs {
            if let Err(e) = validate_skill_name(&spec.name) {
                warn!(skill = %spec.name, error = %e, "invalid skill name");
                continue;
            }

            let dir = skills_dir.join(&spec.name);
            if let Err(e) = fs::create_dir_all(&dir) {
                warn!(skill = %spec.name, error = %e, "failed to create skill dir");
                continue;
            }

            let frontmatter = render_skill(spec);
            let skill_md = dir.join("SKILL.md");
            if let Err(e) = atomic_write(&skill_md, &frontmatter) {
                warn!(skill = %spec.name, error = %e, "failed to write SKILL.md");
                continue;
            }

            for (rel_path, content) in &spec.files {
                let Ok(rel) = validate_supporting_path(rel_path) else {
                    warn!(skill = %spec.name, path = %rel_path, "invalid supporting file path");
                    continue;
                };
                let dest = dir.join(rel);
                if let Err(e) = atomic_write(&dest, content) {
                    warn!(skill = %spec.name, path = %rel_path, error = %e, "failed to write skill file");
                }
            }

            known.insert(spec.name.clone());
            info!(skill = %spec.name, "skill upserted");
        }
        known
    }
}

#[derive(Clone)]
pub struct SkillStore {
    root: PathBuf,
}

impl SkillStore {
    pub fn new(workspace_root: impl Into<PathBuf>) -> Self {
        Self {
            root: workspace_root.into().join(".skills"),
        }
    }

    pub fn list(&self, category: Option<&str>) -> Value {
        let mut skills = Vec::new();
        let mut categories = BTreeSet::new();

        for dir in self.skill_dirs() {
            let skill_md = dir.join("SKILL.md");
            let Ok(content) = fs::read_to_string(&skill_md) else {
                continue;
            };
            let meta = parse_frontmatter(&content);
            let Some(name) = meta
                .get("name")
                .cloned()
                .or_else(|| dir.file_name().map(|s| s.to_string_lossy().to_string()))
            else {
                continue;
            };
            let skill_category = meta.get("category").cloned();
            if let Some(cat) = skill_category.as_deref() {
                categories.insert(cat.to_string());
            }
            if let Some(filter) = category {
                if skill_category.as_deref() != Some(filter) {
                    continue;
                }
            }
            skills.push(json!({
                "name": name,
                "description": meta.get("description").cloned().unwrap_or_default(),
                "category": skill_category,
            }));
        }

        skills.sort_by(|a, b| {
            a.get("name")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .cmp(b.get("name").and_then(Value::as_str).unwrap_or_default())
        });

        json!({
            "success": true,
            "skills": skills,
            "categories": categories.into_iter().collect::<Vec<_>>(),
            "count": skills.len(),
            "hint": "Use skill_view(name) to see full content, tags, and linked files"
        })
    }

    pub fn view(&self, name: &str, file_path: Option<&str>) -> anyhow::Result<Value> {
        validate_skill_name(name)?;
        let dir = self.skill_dir(name);
        if !dir.join("SKILL.md").exists() {
            anyhow::bail!("skill '{name}' not found");
        }

        if let Some(file_path) = file_path {
            let rel = validate_supporting_path(file_path)?;
            let path = dir.join(&rel);
            ensure_inside_existing(&dir, &path)?;
            let content = fs::read_to_string(&path)?;
            return Ok(json!({
                "success": true,
                "name": name,
                "file": rel.to_string_lossy(),
                "content": content,
                "file_type": path.extension().and_then(|e| e.to_str()).map(|e| format!(".{e}")).unwrap_or_default()
            }));
        }

        let skill_md = dir.join("SKILL.md");
        let content = fs::read_to_string(&skill_md)?;
        let meta = parse_frontmatter(&content);
        let linked_files = linked_files(&dir);
        let required_env = csv_or_json_array(meta.get("required_environment_variables"));
        let required_credentials = csv_or_json_array(meta.get("required_credential_files"));
        let missing_env: Vec<String> = required_env
            .iter()
            .filter(|key| std::env::var(key.as_str()).is_err())
            .cloned()
            .collect();
        let missing_credentials: Vec<String> = required_credentials
            .iter()
            .filter(|path| !Path::new(path.as_str()).exists())
            .cloned()
            .collect();

        Ok(json!({
            "success": true,
            "name": meta.get("name").cloned().unwrap_or_else(|| name.to_string()),
            "description": meta.get("description").cloned().unwrap_or_default(),
            "category": meta.get("category").cloned(),
            "tags": csv_or_json_array(meta.get("tags")),
            "related_skills": csv_or_json_array(meta.get("related_skills")),
            "content": content,
            "path": format!("{name}/SKILL.md"),
            "skill_dir": dir.to_string_lossy(),
            "linked_files": linked_files,
            "usage_hint": "To view linked files, call skill_view(name, file_path) where file_path is e.g. 'references/api.md' or 'assets/config.yaml'",
            "required_environment_variables": required_env,
            "missing_required_environment_variables": missing_env,
            "missing_credential_files": missing_credentials,
            "setup_needed": !missing_env.is_empty() || !missing_credentials.is_empty(),
            "setup_skipped": false,
            "readiness_status": if missing_env.is_empty() && missing_credentials.is_empty() { "available" } else { "missing_requirements" }
        }))
    }

    pub fn manage(&self, args: SkillManageArgs) -> anyhow::Result<Value> {
        validate_skill_name(&args.name)?;
        match args.action.as_str() {
            "create" => self.create(args),
            "patch" => self.patch(args),
            "edit" => self.edit(args),
            "delete" => self.delete(args),
            "write_file" => self.write_file(args),
            "remove_file" => self.remove_file(args),
            other => anyhow::bail!("unsupported skill_manage action '{other}'"),
        }
    }

    fn create(&self, args: SkillManageArgs) -> anyhow::Result<Value> {
        let dir = self.skill_dir(&args.name);
        if dir.exists() {
            anyhow::bail!("skill '{}' already exists", args.name);
        }
        let content = args
            .content
            .ok_or_else(|| anyhow::anyhow!("content required for create"))?;
        fs::create_dir_all(&dir)?;
        let content = ensure_frontmatter(&args.name, args.category.as_deref(), &content);
        atomic_write(&dir.join("SKILL.md"), &content)?;
        Ok(json!({"success": true, "message": format!("Created skill '{}'.", args.name)}))
    }

    fn patch(&self, args: SkillManageArgs) -> anyhow::Result<Value> {
        self.refuse_pinned(&args.name)?;
        let path = self.target_file(&args.name, args.file_path.as_deref())?;
        let old = args
            .old_string
            .ok_or_else(|| anyhow::anyhow!("old_string required for patch"))?;
        let new = args.new_string.unwrap_or_default();
        let content = fs::read_to_string(&path)?;
        let matches = content.matches(&old).count();
        if matches == 0 {
            anyhow::bail!("old_string not found");
        }
        if matches > 1 && !args.replace_all {
            anyhow::bail!(
                "old_string matched {matches} times; pass replace_all=true to replace all"
            );
        }
        let updated = if args.replace_all {
            content.replace(&old, &new)
        } else {
            content.replacen(&old, &new, 1)
        };
        atomic_write(&path, &updated)?;
        Ok(
            json!({"success": true, "message": format!("Patched skill '{}' ({} replacement{}).", args.name, if args.replace_all { matches } else { 1 }, if matches == 1 { "" } else { "s" })}),
        )
    }

    fn edit(&self, args: SkillManageArgs) -> anyhow::Result<Value> {
        self.refuse_pinned(&args.name)?;
        let content = args
            .content
            .ok_or_else(|| anyhow::anyhow!("content required for edit"))?;
        let path = self.skill_dir(&args.name).join("SKILL.md");
        if !path.exists() {
            anyhow::bail!("skill '{}' not found", args.name);
        }
        atomic_write(&path, &content)?;
        Ok(
            json!({"success": true, "message": format!("Edited SKILL.md in skill '{}'.", args.name)}),
        )
    }

    fn delete(&self, args: SkillManageArgs) -> anyhow::Result<Value> {
        self.refuse_pinned(&args.name)?;
        let dir = self.skill_dir(&args.name);
        if !dir.exists() {
            anyhow::bail!("skill '{}' not found", args.name);
        }
        if let Some(target) = args.absorbed_into.as_deref().filter(|s| !s.is_empty()) {
            validate_skill_name(target)?;
            if !self.skill_dir(target).join("SKILL.md").exists() {
                anyhow::bail!("absorbed_into target '{target}' does not exist");
            }
        }
        fs::remove_dir_all(&dir)?;
        Ok(json!({"success": true, "message": format!("Deleted skill '{}'.", args.name)}))
    }

    fn write_file(&self, args: SkillManageArgs) -> anyhow::Result<Value> {
        self.refuse_pinned(&args.name)?;
        let rel = validate_supporting_path(
            args.file_path
                .as_deref()
                .ok_or_else(|| anyhow::anyhow!("file_path required for write_file"))?,
        )?;
        let content = args
            .file_content
            .ok_or_else(|| anyhow::anyhow!("file_content required for write_file"))?;
        let path = self.skill_dir(&args.name).join(&rel);
        if !self.skill_dir(&args.name).join("SKILL.md").exists() {
            anyhow::bail!("skill '{}' not found", args.name);
        }
        atomic_write(&path, &content)?;
        Ok(
            json!({"success": true, "message": format!("Wrote {} in skill '{}'.", rel.to_string_lossy(), args.name)}),
        )
    }

    fn remove_file(&self, args: SkillManageArgs) -> anyhow::Result<Value> {
        self.refuse_pinned(&args.name)?;
        let rel = validate_supporting_path(
            args.file_path
                .as_deref()
                .ok_or_else(|| anyhow::anyhow!("file_path required for remove_file"))?,
        )?;
        let path = self.skill_dir(&args.name).join(&rel);
        ensure_inside_existing(&self.skill_dir(&args.name), &path)?;
        fs::remove_file(&path)?;
        Ok(
            json!({"success": true, "message": format!("Removed {} from skill '{}'.", rel.to_string_lossy(), args.name)}),
        )
    }

    fn target_file(&self, name: &str, file_path: Option<&str>) -> anyhow::Result<PathBuf> {
        let dir = self.skill_dir(name);
        if !dir.join("SKILL.md").exists() {
            anyhow::bail!("skill '{name}' not found");
        }
        match file_path {
            Some(file_path) => {
                let rel = validate_supporting_path(file_path)?;
                let path = dir.join(rel);
                ensure_inside_existing(&dir, &path)?;
                Ok(path)
            }
            None => Ok(dir.join("SKILL.md")),
        }
    }

    fn refuse_pinned(&self, name: &str) -> anyhow::Result<()> {
        let path = self.skill_dir(name).join("SKILL.md");
        let content = fs::read_to_string(&path)?;
        if parse_bool(parse_frontmatter(&content).get("pinned")) {
            anyhow::bail!("skill '{name}' is pinned and cannot be modified");
        }
        Ok(())
    }

    fn skill_dir(&self, name: &str) -> PathBuf {
        self.root.join(name)
    }

    fn skill_dirs(&self) -> Vec<PathBuf> {
        let Ok(entries) = fs::read_dir(&self.root) else {
            return Vec::new();
        };
        entries
            .flatten()
            .map(|e| e.path())
            .filter(|p| p.is_dir())
            .collect()
    }
}

#[derive(Debug, Default)]
pub struct SkillManageArgs {
    pub action: String,
    pub name: String,
    pub content: Option<String>,
    pub category: Option<String>,
    pub file_path: Option<String>,
    pub file_content: Option<String>,
    pub old_string: Option<String>,
    pub new_string: Option<String>,
    pub replace_all: bool,
    pub absorbed_into: Option<String>,
}

fn render_skill(spec: &SkillSpec) -> String {
    let mut lines = vec![
        "---".to_string(),
        format!("name: {}", spec.name),
        format!("description: {}", spec.description),
        format!("trigger: {}", trigger_value(&spec.trigger)),
    ];
    if let Some(category) = &spec.category {
        lines.push(format!("category: {category}"));
    }
    if !spec.tags.is_empty() {
        lines.push(format!("tags: [{}]", spec.tags.join(", ")));
    }
    if !spec.related_skills.is_empty() {
        lines.push(format!(
            "related_skills: [{}]",
            spec.related_skills.join(", ")
        ));
    }
    if !spec.required_environment_variables.is_empty() {
        lines.push(format!(
            "required_environment_variables: [{}]",
            spec.required_environment_variables.join(", ")
        ));
    }
    if !spec.required_credential_files.is_empty() {
        lines.push(format!(
            "required_credential_files: [{}]",
            spec.required_credential_files.join(", ")
        ));
    }
    if spec.pinned {
        lines.push("pinned: true".to_string());
    }
    lines.push("---".to_string());
    lines.push(String::new());
    lines.push(spec.instructions.clone());
    lines.join("\n")
}

fn trigger_value(trigger: &domain::SkillTrigger) -> &str {
    match trigger {
        domain::SkillTrigger::Always => "always",
        domain::SkillTrigger::Keyword { .. } => "keyword",
    }
}

fn parse_frontmatter(content: &str) -> BTreeMap<String, String> {
    let mut map = BTreeMap::new();
    let mut lines = content.lines();
    if lines.next() != Some("---") {
        return map;
    }
    for line in lines {
        if line == "---" {
            break;
        }
        if let Some((key, value)) = line.split_once(':') {
            map.insert(
                key.trim().to_string(),
                value.trim().trim_matches('"').to_string(),
            );
        }
    }
    map
}

fn linked_files(dir: &Path) -> Value {
    let mut groups: BTreeMap<String, Vec<String>> = BTreeMap::new();
    for allowed in ALLOWED_FILE_DIRS {
        let root = dir.join(allowed);
        if !root.exists() {
            continue;
        }
        for entry in WalkDir::new(&root).into_iter().flatten() {
            if !entry.file_type().is_file() {
                continue;
            }
            if let Ok(rel) = entry.path().strip_prefix(dir) {
                groups
                    .entry((*allowed).to_string())
                    .or_default()
                    .push(rel.to_string_lossy().to_string());
            }
        }
    }
    for files in groups.values_mut() {
        files.sort();
    }
    json!(groups)
}

fn csv_or_json_array(value: Option<&String>) -> Vec<String> {
    let Some(value) = value else {
        return Vec::new();
    };
    let trimmed = value.trim();
    if trimmed.starts_with('[') && trimmed.ends_with(']') {
        return trimmed
            .trim_start_matches('[')
            .trim_end_matches(']')
            .split(',')
            .map(|v| v.trim().trim_matches('"').to_string())
            .filter(|v| !v.is_empty())
            .collect();
    }
    trimmed
        .split(',')
        .map(|v| v.trim().to_string())
        .filter(|v| !v.is_empty())
        .collect()
}

fn ensure_frontmatter(name: &str, category: Option<&str>, content: &str) -> String {
    if content.trim_start().starts_with("---") {
        return content.to_string();
    }
    let mut lines = vec![
        "---".to_string(),
        format!("name: {name}"),
        "description: ".to_string(),
    ];
    if let Some(category) = category {
        lines.push(format!("category: {category}"));
    }
    lines.push("---".to_string());
    lines.push(String::new());
    lines.push(content.to_string());
    lines.join("\n")
}

fn parse_bool(value: Option<&String>) -> bool {
    value
        .map(|v| matches!(v.trim(), "true" | "yes" | "1"))
        .unwrap_or(false)
}

fn validate_skill_name(name: &str) -> anyhow::Result<()> {
    if name.is_empty() || name.len() > 64 {
        anyhow::bail!("skill name must be 1-64 characters");
    }
    if !name
        .chars()
        .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-' || c == '_')
    {
        anyhow::bail!("skill name must use lowercase letters, numbers, hyphens, or underscores");
    }
    Ok(())
}

fn validate_supporting_path(path: &str) -> anyhow::Result<PathBuf> {
    let rel = PathBuf::from(path);
    if rel.is_absolute() {
        anyhow::bail!("file_path must be relative");
    }
    let mut components = rel.components();
    let Some(first_component) = components.next() else {
        anyhow::bail!("file_path is empty");
    };
    let Component::Normal(first) = first_component else {
        anyhow::bail!("file_path must not contain traversal or special components");
    };
    let first = first.to_string_lossy();
    if !ALLOWED_FILE_DIRS.iter().any(|allowed| *allowed == first) {
        anyhow::bail!("file_path must be under references/, templates/, scripts/, or assets/");
    }
    for component in rel.components() {
        if !matches!(component, Component::Normal(_)) {
            anyhow::bail!("file_path must not contain traversal or special components");
        }
    }
    Ok(rel)
}

fn ensure_inside_existing(base: &Path, path: &Path) -> anyhow::Result<()> {
    let base = base.canonicalize()?;
    let path = path.canonicalize()?;
    if !path.starts_with(base) {
        anyhow::bail!("file_path escapes skill directory");
    }
    Ok(())
}

fn atomic_write(path: &Path, content: &str) -> anyhow::Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    let tmp = path.with_extension(format!(
        "{}tmp",
        path.extension()
            .and_then(|e| e.to_str())
            .map(|e| format!("{e}."))
            .unwrap_or_default()
    ));
    fs::write(&tmp, content)?;
    fs::rename(tmp, path)?;
    Ok(())
}
