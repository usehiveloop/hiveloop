use bridge_core::{SkillDefinition, SkillSource};
use std::collections::HashMap;

pub(super) fn make_skills() -> Vec<SkillDefinition> {
    vec![
        SkillDefinition {
            id: "code-review".to_string(),
            title: "Code Review".to_string(),
            description: "Reviews code for quality and best practices".to_string(),
            content: "You are a code review expert.\n\n## Guidelines\n- Check for bugs\n- Suggest improvements".to_string(),
            ..Default::default()
        },
        SkillDefinition {
            id: "pr-summary".to_string(),
            title: "PR Summary".to_string(),
            description: "Summarizes pull requests concisely".to_string(),
            content: "You are a PR summarizer.\n\nCreate a concise summary of the changes.".to_string(),
            ..Default::default()
        },
    ]
}

pub(super) fn make_multi_file_skill() -> SkillDefinition {
    let mut files = HashMap::new();
    files.insert(
        "reference.md".to_string(),
        "# API Reference\n\nGET /users - list users".to_string(),
    );
    files.insert(
        "examples/basic.md".to_string(),
        "# Basic Example\n\ncurl localhost/users".to_string(),
    );

    SkillDefinition {
        id: "api-docs".to_string(),
        title: "API Docs".to_string(),
        description: "API documentation skill".to_string(),
        content: "You are an API expert.\n\nSee [reference.md](reference.md) for the API.\nSee [examples](examples/basic.md) for usage.".to_string(),
        files,
        source: SkillSource::ControlPlane,
        ..Default::default()
    }
}
