//! Top-level helpers that assemble a `SystemReminder` from common inputs.

use bridge_core::SkillDefinition;
use chrono::{DateTime, Utc};

use super::{SystemReminder, TodoItem};

/// Create a system reminder with skills and sub-agents sections.
pub fn create_reminder_with_skills(
    skills: &[SkillDefinition],
    subagents: &[(String, String)],
) -> String {
    if skills.is_empty() && subagents.is_empty() {
        return String::new();
    }

    SystemReminder::new()
        .with_skills(skills)
        .with_subagents(subagents)
        .build()
}

/// Create a system reminder with skills, sub-agents, and current date.
pub fn create_reminder_with_skills_and_date(
    skills: &[SkillDefinition],
    subagents: &[(String, String)],
    date: DateTime<Utc>,
) -> String {
    SystemReminder::new()
        .with_skills(skills)
        .with_subagents(subagents)
        .with_current_date(date)
        .build()
}

/// Create a system reminder with skills, sub-agents, todos, and current date.
pub fn create_reminder_with_skills_todos_and_date(
    skills: &[SkillDefinition],
    subagents: &[(String, String)],
    todos: Option<&[TodoItem]>,
    date: DateTime<Utc>,
) -> String {
    let mut reminder = SystemReminder::new();

    if !skills.is_empty() {
        reminder = reminder.with_skills(skills);
    }

    reminder = reminder.with_subagents(subagents);

    if let Some(todo_list) = todos {
        if !todo_list.is_empty() {
            reminder = reminder.with_todos(todo_list);
        }
    }

    reminder = reminder.with_current_date(date);

    reminder.build()
}
