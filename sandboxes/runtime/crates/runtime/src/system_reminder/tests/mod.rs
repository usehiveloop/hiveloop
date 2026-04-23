mod builder;
mod date;
mod split;

use bridge_core::SkillDefinition;

pub(super) fn make_test_skills() -> Vec<SkillDefinition> {
    vec![
        SkillDefinition {
            id: "code-review".to_string(),
            title: "Code Review".to_string(),
            description: "Reviews code for quality and best practices".to_string(),
            content: "You are a code review expert...".to_string(),
            ..Default::default()
        },
        SkillDefinition {
            id: "commit".to_string(),
            title: "Commit".to_string(),
            description: "Writes conventional commit messages".to_string(),
            content: "Write conventional commits...".to_string(),
            ..Default::default()
        },
    ]
}

pub(super) fn make_test_subagents() -> Vec<(String, String)> {
    vec![
        (
            "researcher".to_string(),
            "Searches and summarizes information".to_string(),
        ),
        ("coder".to_string(), "Writes and reviews code".to_string()),
    ]
}
