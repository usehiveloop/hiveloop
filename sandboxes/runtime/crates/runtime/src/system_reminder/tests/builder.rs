use super::super::*;
use super::{make_test_skills, make_test_subagents};
use bridge_core::SkillDefinition;
use chrono::Utc;

#[test]
fn test_empty_reminder() {
    let reminder = SystemReminder::new();
    assert!(reminder.is_empty());
    assert_eq!(reminder.build(), "");
}

#[test]
fn test_reminder_with_skills() {
    let skills = make_test_skills();
    let reminder = SystemReminder::new().with_skills(&skills);

    let output = reminder.build();

    assert!(!reminder.is_empty());
    assert!(output.contains("<system-reminder>"));
    assert!(output.contains("# System Reminders"));
    assert!(output.contains("## Available skills"));
    assert!(output.contains("The following skills are available for use with the Skill tool:"));
    assert!(output.contains("- **Code Review** - Reviews code for quality and best practices"));
    assert!(output.contains("- **Commit** - Writes conventional commit messages"));
    assert!(output.contains("</system-reminder>"));

    // Skill content should NOT appear (only title + description)
    assert!(!output.contains("You are a code review expert"));
    assert!(!output.contains("Write conventional commits"));
}

#[test]
fn test_reminder_with_current_date() {
    let date = Utc::now();
    let reminder = SystemReminder::new().with_current_date(date);

    let output = reminder.build();

    assert!(!reminder.is_empty());
    assert!(output.contains("<system-reminder>"));
    assert!(output.contains("## Current date"));
    assert!(output.contains("Today is"));
    assert!(output.contains("</system-reminder>"));
}

#[test]
fn test_reminder_with_skills_and_date() {
    let skills = make_test_skills();
    let date = Utc::now();
    let reminder = SystemReminder::new()
        .with_skills(&skills)
        .with_current_date(date);

    let output = reminder.build();

    assert!(output.contains("## Available skills"));
    assert!(output.contains("## Current date"));
    assert!(output.contains("Today is"));
    assert!(output.contains("Code Review"));
}

#[test]
fn test_reminder_empty_skills() {
    let skills: Vec<SkillDefinition> = vec![];
    let reminder = SystemReminder::new().with_skills(&skills);

    assert!(reminder.is_empty());
    assert_eq!(reminder.build(), "");
}

#[test]
fn test_reminder_with_subagents() {
    let subagents = make_test_subagents();
    let reminder = SystemReminder::new().with_subagents(&subagents);

    let output = reminder.build();

    assert!(!reminder.is_empty());
    assert!(output.contains("<system-reminder>"));
    assert!(output.contains("## Available sub-agents"));
    assert!(output.contains("The following sub-agents are available for use with the Agent tool:"));
    assert!(output.contains("- **researcher** - Searches and summarizes information"));
    assert!(output.contains("- **coder** - Writes and reviews code"));
    assert!(output.contains("</system-reminder>"));
}

#[test]
fn test_reminder_with_empty_subagents() {
    let subagents: Vec<(String, String)> = vec![];
    let reminder = SystemReminder::new().with_subagents(&subagents);

    assert!(reminder.is_empty());
    assert_eq!(reminder.build(), "");
}

#[test]
fn test_reminder_with_skills_and_subagents() {
    let skills = make_test_skills();
    let subagents = make_test_subagents();
    let reminder = SystemReminder::new()
        .with_skills(&skills)
        .with_subagents(&subagents);

    let output = reminder.build();

    assert!(output.contains("## Available skills"));
    assert!(output.contains("## Available sub-agents"));
    assert!(output.contains("Code Review"));
    assert!(output.contains("researcher"));
}

#[test]
fn test_convenience_function() {
    let skills = make_test_skills();
    let output = create_reminder_with_skills(&skills, &[]);

    assert!(output.contains("<system-reminder>"));
    assert!(output.contains("Available skills"));
    assert!(output.contains("Code Review"));
    assert!(output.contains("Commit"));
}

#[test]
fn test_convenience_function_empty() {
    let skills: Vec<SkillDefinition> = vec![];
    let output = create_reminder_with_skills(&skills, &[]);

    assert_eq!(output, "");
}

#[test]
fn test_convenience_function_with_date() {
    let skills = make_test_skills();
    let date = Utc::now();
    let output = create_reminder_with_skills_and_date(&skills, &[], date);

    assert!(output.contains("Available skills"));
    assert!(output.contains("Current date"));
    assert!(output.contains("Today is"));
}

#[test]
fn test_local_and_remote_skills_shown_equally() {
    let skills = vec![
        SkillDefinition {
            id: "remote".to_string(),
            title: "Remote Skill".to_string(),
            description: "From control plane".to_string(),
            content: String::new(),
            ..Default::default()
        },
        SkillDefinition {
            id: "local".to_string(),
            title: "Local Skill".to_string(),
            description: "From filesystem".to_string(),
            content: String::new(),
            source: bridge_core::SkillSource::ClaudeCode,
            ..Default::default()
        },
    ];
    let output = SystemReminder::new().with_skills(&skills).build();

    // Both show up identically — source doesn't matter to the model
    assert!(output.contains("- **Remote Skill** - From control plane"));
    assert!(output.contains("- **Local Skill** - From filesystem"));
}

#[test]
fn test_model_disabled_skills_filtered() {
    let skills = vec![
        SkillDefinition {
            id: "visible".to_string(),
            title: "Visible".to_string(),
            description: "Should appear".to_string(),
            content: String::new(),
            ..Default::default()
        },
        SkillDefinition {
            id: "hidden".to_string(),
            title: "Hidden".to_string(),
            description: "Should not appear".to_string(),
            content: String::new(),
            frontmatter: Some(bridge_core::SkillFrontmatter {
                disable_model_invocation: Some(true),
                ..Default::default()
            }),
            ..Default::default()
        },
    ];
    let output = SystemReminder::new().with_skills(&skills).build();

    assert!(output.contains("Visible"));
    assert!(!output.contains("Hidden"));
}

#[test]
fn test_non_user_invocable_skills_filtered() {
    let skills = vec![SkillDefinition {
        id: "model-only".to_string(),
        title: "Model Only".to_string(),
        description: "Not user invocable".to_string(),
        content: String::new(),
        frontmatter: Some(bridge_core::SkillFrontmatter {
            user_invocable: Some(false),
            ..Default::default()
        }),
        ..Default::default()
    }];
    let output = SystemReminder::new().with_skills(&skills).build();

    // When all skills are filtered, the section should be empty
    assert!(output.is_empty());
}
