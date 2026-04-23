use super::*;
use bridge_core::{SkillDefinition, SkillSource};
use std::collections::HashMap;
use std::path::{Path, PathBuf};

fn skill_with_files(id: &str, files: HashMap<String, String>) -> SkillDefinition {
    SkillDefinition {
        id: id.to_string(),
        title: id.to_string(),
        description: format!("Test skill {id}"),
        content: "skill content".to_string(),
        files,
        source: SkillSource::ControlPlane,
        ..Default::default()
    }
}

#[tokio::test]
async fn write_creates_directory_structure() {
    let tmp = tempfile::tempdir().unwrap();
    let mut files = HashMap::new();
    files.insert(
        "scripts/run.sh".to_string(),
        "#!/bin/bash\necho hi".to_string(),
    );
    files.insert("references/guide.md".to_string(), "# Guide".to_string());

    let skills = vec![skill_with_files("my-skill", files)];
    write_skill_files(&skills, tmp.path()).await;

    let script = tmp.path().join(".skills/my-skill/scripts/run.sh");
    let guide = tmp.path().join(".skills/my-skill/references/guide.md");
    assert!(script.exists(), "script should exist");
    assert!(guide.exists(), "guide should exist");
    assert_eq!(
        std::fs::read_to_string(&script).unwrap(),
        "#!/bin/bash\necho hi"
    );
    assert_eq!(std::fs::read_to_string(&guide).unwrap(), "# Guide");
}

#[cfg(unix)]
#[tokio::test]
async fn write_sets_executable_permissions() {
    use std::os::unix::fs::PermissionsExt;

    let tmp = tempfile::tempdir().unwrap();
    let mut files = HashMap::new();
    files.insert("run.sh".to_string(), "#!/bin/bash".to_string());
    files.insert(
        "analyze.py".to_string(),
        "#!/usr/bin/env python3".to_string(),
    );
    files.insert("deploy.rb".to_string(), "#!/usr/bin/env ruby".to_string());
    files.insert("readme.md".to_string(), "# Readme".to_string());
    files.insert("data.json".to_string(), "{}".to_string());

    let skills = vec![skill_with_files("perms-test", files)];
    write_skill_files(&skills, tmp.path()).await;

    let base = tmp.path().join(".skills/perms-test");

    for name in &["run.sh", "analyze.py", "deploy.rb"] {
        let mode = std::fs::metadata(base.join(name))
            .unwrap()
            .permissions()
            .mode();
        assert_eq!(mode & 0o111, 0o111, "{name} should be executable");
    }
    for name in &["readme.md", "data.json"] {
        let mode = std::fs::metadata(base.join(name))
            .unwrap()
            .permissions()
            .mode();
        assert_eq!(mode & 0o111, 0, "{name} should NOT be executable");
    }
}

#[tokio::test]
async fn write_handles_nested_paths() {
    let tmp = tempfile::tempdir().unwrap();
    let mut files = HashMap::new();
    files.insert(
        "scripts/deploy/run.sh".to_string(),
        "#!/bin/bash\necho deploy".to_string(),
    );

    let skills = vec![skill_with_files("nested", files)];
    write_skill_files(&skills, tmp.path()).await;

    let script = tmp.path().join(".skills/nested/scripts/deploy/run.sh");
    assert!(script.exists());
}

#[tokio::test]
async fn write_skips_skills_with_empty_files() {
    let tmp = tempfile::tempdir().unwrap();
    let skills = vec![skill_with_files("empty", HashMap::new())];
    write_skill_files(&skills, tmp.path()).await;

    assert!(!tmp.path().join(".skills/empty").exists());
}

#[tokio::test]
async fn write_rejects_path_traversal() {
    let tmp = tempfile::tempdir().unwrap();
    let mut files = HashMap::new();
    files.insert("../../etc/passwd".to_string(), "bad content".to_string());
    files.insert("scripts/good.sh".to_string(), "#!/bin/bash".to_string());

    let skills = vec![skill_with_files("traversal", files)];
    write_skill_files(&skills, tmp.path()).await;

    // The traversal path should be rejected.
    assert!(!tmp.path().join("etc/passwd").exists());
    // The good file should still be written.
    assert!(tmp
        .path()
        .join(".skills/traversal/scripts/good.sh")
        .exists());
}

#[tokio::test]
async fn cleanup_removes_skill_directories() {
    let tmp = tempfile::tempdir().unwrap();
    let mut files = HashMap::new();
    files.insert("script.sh".to_string(), "#!/bin/bash".to_string());

    let skills = vec![
        skill_with_files("skill-a", files.clone()),
        skill_with_files("skill-b", files),
    ];
    write_skill_files(&skills, tmp.path()).await;

    assert!(tmp.path().join(".skills/skill-a").exists());
    assert!(tmp.path().join(".skills/skill-b").exists());

    // Clean up only skill-a.
    cleanup_skill_files(&["skill-a"], tmp.path()).await;
    assert!(!tmp.path().join(".skills/skill-a").exists());
    assert!(tmp.path().join(".skills/skill-b").exists());
    // .skills/ root still exists because skill-b is there.
    assert!(tmp.path().join(".skills").exists());
}

#[tokio::test]
async fn cleanup_removes_skills_root_when_empty() {
    let tmp = tempfile::tempdir().unwrap();
    let mut files = HashMap::new();
    files.insert("script.sh".to_string(), "#!/bin/bash".to_string());

    let skills = vec![skill_with_files("only-skill", files)];
    write_skill_files(&skills, tmp.path()).await;
    assert!(tmp.path().join(".skills").exists());

    cleanup_skill_files(&["only-skill"], tmp.path()).await;
    assert!(!tmp.path().join(".skills").exists());
}

#[tokio::test]
async fn cleanup_is_noop_for_nonexistent_ids() {
    let tmp = tempfile::tempdir().unwrap();
    // Should not panic or error.
    cleanup_skill_files(&["nonexistent"], tmp.path()).await;
}

#[test]
fn skill_dir_path_returns_expected_path() {
    let base = Path::new("/workspace");
    assert_eq!(
        skill_dir_path("use-railway", base),
        PathBuf::from("/workspace/.skills/use-railway")
    );
}
