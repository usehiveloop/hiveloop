use std::fs;
use std::path::PathBuf;

use domain::{SkillSpec, SkillTrigger};
use skills::SkillWriter;

// SCENARIO: Control plane sends a skill definition for issue analysis.
// The skill writer creates a .skills/issue-analyzer/SKILL.md with proper frontmatter.
#[test]
fn skill_writer_creates_frontmatter_and_instructions() {
    let root = setup_temp_dir("skill-write-1");
    let writer = SkillWriter::new(root.clone());

    let specs = vec![SkillSpec {
        name: "issue-analyzer".into(),
        description: "Analyze Linear issues".into(),
        trigger: SkillTrigger::Keyword {
            patterns: vec!["issue".into(), "SEA-".into()],
        },
        instructions: "Group by status. Highlight urgent ones.".into(),
        files: Default::default(),
    }];

    writer.sync(&specs);

    let skill_md = root.join(".skills/issue-analyzer/SKILL.md");
    assert!(skill_md.exists(), "SKILL.md must exist");
    let content = fs::read_to_string(&skill_md).unwrap();
    assert!(
        content.contains("name: issue-analyzer"),
        "must have name in frontmatter"
    );
    assert!(
        content.contains("description: Analyze Linear issues"),
        "must have description"
    );
    assert!(
        content.contains("trigger: keyword"),
        "must have trigger type"
    );
    assert!(
        content.contains("Group by status"),
        "must include instructions"
    );
}

// SCENARIO: Control plane updates a skill definition. Old files are replaced.
// The writer overwrites existing skill files with new content.
#[test]
fn skill_writer_overwrites_existing_skill() {
    let root = setup_temp_dir("skill-write-2");
    let writer = SkillWriter::new(root.clone());

    // Create initial skill
    let specs = vec![SkillSpec {
        name: "reporter".into(),
        description: "Old description".into(),
        trigger: SkillTrigger::Always,
        instructions: "Old instructions.".into(),
        files: Default::default(),
    }];
    writer.sync(&specs);

    // Update with new description
    let updated = vec![SkillSpec {
        name: "reporter".into(),
        description: "New description - weekly summary generator".into(),
        trigger: SkillTrigger::Always,
        instructions: "New instructions for weekly reports.".into(),
        files: Default::default(),
    }];
    writer.sync(&updated);

    let skill_md = root.join(".skills/reporter/SKILL.md");
    let content = fs::read_to_string(&skill_md).unwrap();
    assert!(
        content.contains("New description"),
        "must have new description"
    );
    assert!(
        !content.contains("Old description"),
        "old description must be gone"
    );
}

// SCENARIO: Control plane removes a skill that's no longer needed.
// The writer purges the .skills/ directory for any skill not in the new spec list.
#[test]
fn purges_stale_skills_not_in_spec_list() {
    let root = setup_temp_dir("skill-purge-1");
    let writer = SkillWriter::new(root.clone());

    // Create two skills
    writer.sync(&[
        SkillSpec {
            name: "issue-analyzer".into(),
            description: "Analyze".into(),
            trigger: SkillTrigger::Always,
            instructions: "Instructions.".into(),
            files: Default::default(),
        },
        SkillSpec {
            name: "code-reviewer".into(),
            description: "Review".into(),
            trigger: SkillTrigger::Always,
            instructions: "Instructions.".into(),
            files: Default::default(),
        },
    ]);

    assert!(root.join(".skills/issue-analyzer/SKILL.md").exists());
    assert!(root.join(".skills/code-reviewer/SKILL.md").exists());

    // Now only keep issue-analyzer - code-reviewer should be purged
    writer.sync(&[SkillSpec {
        name: "issue-analyzer".into(),
        description: "Analyze".into(),
        trigger: SkillTrigger::Always,
        instructions: "Updated.".into(),
        files: Default::default(),
    }]);

    assert!(root.join(".skills/issue-analyzer/SKILL.md").exists());
    assert!(
        !root.join(".skills/code-reviewer").exists(),
        "stale skill must be purged"
    );
}

// SCENARIO: Control plane sends a multi-file skill with scripts and references.
// The writer creates the SKILL.md AND all supporting files in the right locations.
#[test]
fn multi_file_skill_creates_supporting_files() {
    let root = setup_temp_dir("skill-multi-1");
    let writer = SkillWriter::new(root.clone());

    let mut files = std::collections::HashMap::new();
    files.insert(
        "scripts/fetch.sh".into(),
        "#!/bin/bash\necho fetching...".into(),
    );
    files.insert("references/data.json".into(), "{\"version\": 1}".into());

    writer.sync(&[SkillSpec {
        name: "data-fetcher".into(),
        description: "Fetch and process data".into(),
        trigger: SkillTrigger::Keyword {
            patterns: vec!["fetch".into()],
        },
        instructions: "Run fetch.sh to get data.".into(),
        files,
    }]);

    assert!(root.join(".skills/data-fetcher/SKILL.md").exists());
    assert!(
        root.join(".skills/data-fetcher/scripts/fetch.sh").exists(),
        "supporting script must be created"
    );
    assert!(
        root.join(".skills/data-fetcher/references/data.json")
            .exists(),
        "reference file must be created"
    );

    let script = fs::read_to_string(root.join(".skills/data-fetcher/scripts/fetch.sh")).unwrap();
    assert!(
        script.contains("#!/bin/bash"),
        "script content must be preserved"
    );
}

// SCENARIO: Control plane sends empty skills array.
// All previously written skills are purged.
#[test]
fn empty_specs_purges_all_skills() {
    let root = setup_temp_dir("skill-empty-1");
    let writer = SkillWriter::new(root.clone());

    writer.sync(&[SkillSpec {
        name: "temp-skill".into(),
        description: "Temp".into(),
        trigger: SkillTrigger::Always,
        instructions: "Temp.".into(),
        files: Default::default(),
    }]);

    assert!(root.join(".skills/temp-skill/SKILL.md").exists());

    // Send empty array - all purged
    writer.sync(&[]);

    assert!(
        !root.join(".skills/temp-skill").exists(),
        "all skills must be purged"
    );
}

fn setup_temp_dir(suffix: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("skills-test-{}", suffix));
    let _ = fs::remove_dir_all(&dir);
    fs::create_dir_all(&dir).unwrap();
    dir
}
