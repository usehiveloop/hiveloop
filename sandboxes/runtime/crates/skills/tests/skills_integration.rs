use std::fs;
use std::path::PathBuf;

use domain::{SkillSpec, SkillTrigger};
use skills::{SkillManageArgs, SkillStore, SkillWriter};

// SCENARIO: Control plane pushes a skill definition during deployment.
// The system materializes it into /workspace/.skills without requiring DB reads at runtime.
#[test]
fn config_skill_import_creates_frontmatter_and_instructions() {
    let root = setup_temp_dir("skill-import-1");
    let writer = SkillWriter::new(root.clone());

    writer.sync(&[skill(
        "issue-analyzer",
        "Analyze Linear issues",
        "Group by status. Highlight urgent ones.",
    )]);

    let skill_md = root.join(".skills/issue-analyzer/SKILL.md");
    assert!(skill_md.exists(), "SKILL.md must exist");
    let content = fs::read_to_string(&skill_md).unwrap();
    assert!(content.contains("name: issue-analyzer"));
    assert!(content.contains("description: Analyze Linear issues"));
    assert!(content.contains("Group by status"));
}

// SCENARIO: A later /config push contains no skills because the operator only changed model config.
// Learned filesystem skills must survive; empty skills arrays are not destructive.
#[test]
fn empty_config_skills_does_not_prune_filesystem_memory() {
    let root = setup_temp_dir("skill-empty-non-destructive");
    let writer = SkillWriter::new(root.clone());

    writer.sync(&[skill("learned-debugger", "Debug", "Use logs first.")]);
    assert!(root.join(".skills/learned-debugger/SKILL.md").exists());

    writer.sync(&[]);

    assert!(
        root.join(".skills/learned-debugger/SKILL.md").exists(),
        "empty config skills array must not delete learned procedural memory"
    );
}

// SCENARIO: Control plane sends a partial skills array.
// Existing local skills not mentioned by config stay available, while mentioned skills are updated.
#[test]
fn partial_config_skills_upserts_without_deleting_other_skills() {
    let root = setup_temp_dir("skill-upsert-1");
    let writer = SkillWriter::new(root.clone());

    writer.sync(&[
        skill("railway-debugger", "Old Railway", "Old instructions."),
        skill("support-helper", "Support", "Support instructions."),
    ]);
    writer.sync(&[skill(
        "railway-debugger",
        "Railway deploy debugger",
        "New instructions: check deployments first.",
    )]);

    let railway = fs::read_to_string(root.join(".skills/railway-debugger/SKILL.md")).unwrap();
    let support = fs::read_to_string(root.join(".skills/support-helper/SKILL.md")).unwrap();
    assert!(railway.contains("New instructions"));
    assert!(support.contains("Support instructions"));
}

// SCENARIO: A skill has supporting references and scripts.
// The importer writes all linked files to normal filesystem paths for inspectability.
#[test]
fn multi_file_skill_creates_supporting_files() {
    let root = setup_temp_dir("skill-multi-1");
    let writer = SkillWriter::new(root.clone());

    let mut spec = skill("data-fetcher", "Fetch and process data", "Run fetch.sh.");
    spec.files.insert(
        "scripts/fetch.sh".into(),
        "#!/bin/bash\necho fetching...".into(),
    );
    spec.files
        .insert("references/data.json".into(), "{\"version\": 1}".into());
    writer.sync(&[spec]);

    assert!(root.join(".skills/data-fetcher/SKILL.md").exists());
    assert!(root.join(".skills/data-fetcher/scripts/fetch.sh").exists());
    assert!(root
        .join(".skills/data-fetcher/references/data.json")
        .exists());
}

// SCENARIO: The agent asks what it can remember.
// skills_list returns cheap metadata only and does not leak full skill bodies into context.
#[test]
fn skills_list_returns_metadata_without_full_content() {
    let root = setup_temp_dir("skill-list-1");
    SkillWriter::new(root.clone()).sync(&[skill(
        "railway-debugger",
        "Debug Railway deploy failures",
        "Very long private instructions that should not appear in list.",
    )]);

    let result = SkillStore::new(root).list(None);
    assert_eq!(result["success"], true);
    assert_eq!(result["count"], 1);
    assert_eq!(result["skills"][0]["name"], "railway-debugger");
    assert!(
        !result
            .to_string()
            .contains("Very long private instructions"),
        "list must not include full procedural content"
    );
}

// SCENARIO: The agent selects one relevant skill.
// skill_view returns full SKILL.md plus linked file inventory, but not linked file contents.
#[test]
fn skill_view_returns_full_skill_and_linked_file_inventory() {
    let root = setup_temp_dir("skill-view-1");
    let mut spec = skill(
        "railway-debugger",
        "Debug Railway",
        "Check failed deployment logs.",
    );
    spec.category = Some("devops".into());
    spec.tags = vec!["railway".into(), "deploy".into()];
    spec.files.insert(
        "references/common-errors.md".into(),
        "# Common Errors".into(),
    );
    SkillWriter::new(root.clone()).sync(&[spec]);

    let result = SkillStore::new(root)
        .view("railway-debugger", None)
        .unwrap();
    assert_eq!(result["success"], true);
    assert_eq!(result["category"], "devops");
    assert!(result["content"]
        .as_str()
        .unwrap()
        .contains("Check failed deployment logs."));
    assert_eq!(
        result["linked_files"]["references"][0],
        "references/common-errors.md"
    );
    assert!(
        !result.to_string().contains("# Common Errors"),
        "main skill view should list linked files, not inline their contents"
    );
}

// SCENARIO: The relevant workflow needs one reference file.
// skill_view(name, file_path) loads exactly that linked file from allowed folders.
#[test]
fn skill_view_loads_specific_linked_file() {
    let root = setup_temp_dir("skill-view-file-1");
    let mut spec = skill(
        "railway-debugger",
        "Debug Railway",
        "Read reference when needed.",
    );
    spec.files.insert(
        "references/common-errors.md".into(),
        "# Common Errors".into(),
    );
    SkillWriter::new(root.clone()).sync(&[spec]);

    let result = SkillStore::new(root)
        .view("railway-debugger", Some("references/common-errors.md"))
        .unwrap();
    assert_eq!(result["success"], true);
    assert_eq!(result["file"], "references/common-errors.md");
    assert_eq!(result["content"], "# Common Errors");
}

// SCENARIO: A malicious or confused model attempts path traversal.
// Skill reads/writes must never escape /workspace/.skills.
#[test]
fn skill_view_rejects_path_traversal() {
    let root = setup_temp_dir("skill-path-1");
    SkillWriter::new(root.clone()).sync(&[skill("safe-skill", "Safe", "Stay inside.")]);

    let err = SkillStore::new(root)
        .view("safe-skill", Some("../secret.txt"))
        .unwrap_err()
        .to_string();
    assert!(err.contains("under references/") || err.contains("traversal"));
}

// SCENARIO: The user asks the agent to remember a workflow.
// skill_manage(create) creates a normal filesystem skill.
#[test]
fn skill_manage_create_creates_skill() {
    let root = setup_temp_dir("skill-manage-create-1");
    let result = SkillStore::new(root.clone())
        .manage(SkillManageArgs {
            action: "create".into(),
            name: "debug-railway-deploys".into(),
            category: Some("devops".into()),
            content: Some("# Debug Railway Deploys\nCheck logs first.".into()),
            ..Default::default()
        })
        .unwrap();

    assert_eq!(result["success"], true);
    let content = fs::read_to_string(root.join(".skills/debug-railway-deploys/SKILL.md")).unwrap();
    assert!(content.contains("category: devops"));
    assert!(content.contains("Check logs first."));
}

// SCENARIO: The agent patches stale instructions after user approval.
// Ambiguous patches are refused unless replace_all=true to avoid corrupting skills.
#[test]
fn skill_manage_patch_requires_unique_match_by_default() {
    let root = setup_temp_dir("skill-manage-patch-1");
    SkillWriter::new(root.clone()).sync(&[skill(
        "debugger",
        "Debug",
        "Run logs. Then run logs again.",
    )]);

    let err = SkillStore::new(root)
        .manage(SkillManageArgs {
            action: "patch".into(),
            name: "debugger".into(),
            old_string: Some("logs".into()),
            new_string: Some("deployment logs".into()),
            ..Default::default()
        })
        .unwrap_err()
        .to_string();
    assert!(err.contains("matched 2 times"));
}

// SCENARIO: The agent stores supporting material for a skill.
// skill_manage(write_file) allows approved folders and refuses unrelated paths.
#[test]
fn skill_manage_write_file_is_limited_to_skill_support_dirs() {
    let root = setup_temp_dir("skill-manage-write-1");
    SkillWriter::new(root.clone()).sync(&[skill("debugger", "Debug", "Use references.")]);

    let ok = SkillStore::new(root.clone())
        .manage(SkillManageArgs {
            action: "write_file".into(),
            name: "debugger".into(),
            file_path: Some("references/errors.md".into()),
            file_content: Some("# Errors".into()),
            ..Default::default()
        })
        .unwrap();
    assert_eq!(ok["success"], true);

    let err = SkillStore::new(root)
        .manage(SkillManageArgs {
            action: "write_file".into(),
            name: "debugger".into(),
            file_path: Some("notes/errors.md".into()),
            file_content: Some("# Errors".into()),
            ..Default::default()
        })
        .unwrap_err()
        .to_string();
    assert!(err.contains("references/"));
}

// SCENARIO: A locally authored skill must sync back to control plane with full content.
// The snapshot contains SKILL.md plus every supported file body.
#[test]
fn skill_sync_snapshot_includes_content_metadata_and_files() {
    let root = setup_temp_dir("skill-sync-snapshot-1");
    let store = SkillStore::new(root.clone());
    store
        .manage(SkillManageArgs {
            action: "create".into(),
            name: "debugger".into(),
            content: Some(
                "---\nname: debugger\ndescription: Debug production issues\ntags: [debug, prod]\n---\n\n# Debug\nCheck logs.".into(),
            ),
            ..Default::default()
        })
        .unwrap();
    store
        .manage(SkillManageArgs {
            action: "write_file".into(),
            name: "debugger".into(),
            file_path: Some("references/errors.md".into()),
            file_content: Some("# Errors".into()),
            ..Default::default()
        })
        .unwrap();

    let snapshot = store.sync_snapshot("debugger").unwrap();
    assert_eq!(snapshot["name"], "debugger");
    assert_eq!(snapshot["description"], "Debug production issues");
    assert!(snapshot["content"].as_str().unwrap().contains("# Debug"));
    assert_eq!(snapshot["files"]["references/errors.md"], "# Errors");
    assert_eq!(snapshot["tags"][0], "debug");
}

// SCENARIO: Curated skills are pinned.
// The agent cannot patch, edit, or delete pinned skills.
#[test]
fn pinned_skills_refuse_mutations() {
    let root = setup_temp_dir("skill-pinned-1");
    let mut spec = skill("curated-debugger", "Curated", "Do not modify.");
    spec.pinned = true;
    SkillWriter::new(root.clone()).sync(&[spec]);

    let err = SkillStore::new(root)
        .manage(SkillManageArgs {
            action: "patch".into(),
            name: "curated-debugger".into(),
            old_string: Some("modify".into()),
            new_string: Some("change".into()),
            ..Default::default()
        })
        .unwrap_err()
        .to_string();
    assert!(err.contains("pinned"));
}

fn skill(name: &str, description: &str, instructions: &str) -> SkillSpec {
    SkillSpec {
        name: name.into(),
        description: description.into(),
        trigger: SkillTrigger::Always,
        instructions: instructions.into(),
        files: Default::default(),
        category: None,
        tags: Vec::new(),
        related_skills: Vec::new(),
        required_environment_variables: Vec::new(),
        required_credential_files: Vec::new(),
        pinned: false,
    }
}

fn setup_temp_dir(suffix: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("skills-test-{suffix}"));
    let _ = fs::remove_dir_all(&dir);
    fs::create_dir_all(&dir).unwrap();
    dir
}
