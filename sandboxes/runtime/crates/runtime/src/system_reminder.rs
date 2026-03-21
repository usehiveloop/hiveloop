//! System reminder injection for conversations.
//!
//! Provides a flexible markdown-based system reminder that is injected
//! before every user message. The reminder can contain multiple sections
//! (skills, date, context, etc.) that help guide the agent's behavior.

use bridge_core::SkillDefinition;
use chrono::{DateTime, Utc};

/// A todo item for display in the system reminder
#[derive(Clone)]
pub struct TodoItem {
    pub content: String,
    pub status: String,
    pub priority: String,
}

impl TodoItem {
    /// Format status for display
    fn format_status(&self) -> String {
        self.status.clone()
    }

    /// Format priority for display
    fn format_priority(&self) -> String {
        match self.priority.as_str() {
            "high" => "[high]".to_string(),
            "medium" => "[medium]".to_string(),
            "low" => "[low]".to_string(),
            _ => "".to_string(),
        }
    }
}

/// A flexible system reminder builder that generates markdown content
/// to be injected before user messages.
pub struct SystemReminder {
    sections: Vec<Section>,
}

/// A section within the system reminder.
struct Section {
    title: String,
    content: String,
}

impl SystemReminder {
    /// Create a new empty system reminder.
    pub fn new() -> Self {
        Self {
            sections: Vec::new(),
        }
    }

    /// Add the available skills section.
    pub fn with_skills(mut self, skills: &[SkillDefinition]) -> Self {
        if skills.is_empty() {
            return self;
        }

        let mut content = String::new();
        content.push_str("The following skills are available for use with the Skill tool:\n\n");

        for skill in skills {
            content.push_str(&format!("- **{}** - {}\n", skill.title, skill.description));
        }

        self.sections.push(Section {
            title: "Available skills".to_string(),
            content,
        });

        self
    }

    /// Add the available sub-agents section.
    pub fn with_subagents(mut self, subagents: &[(String, String)]) -> Self {
        if subagents.is_empty() {
            return self;
        }

        let mut content = String::new();
        content.push_str(
            "The following sub-agents are available for use with the Agent tool:\n\n",
        );

        for (name, description) in subagents {
            content.push_str(&format!("- **{}** - {}\n", name, description));
        }

        self.sections.push(Section {
            title: "Available sub-agents".to_string(),
            content,
        });

        self
    }

    /// Add the current date section.
    pub fn with_current_date(mut self, date: DateTime<Utc>) -> Self {
        let formatted_date = date.format("%A, %B %d, %Y").to_string();
        let content = format!("Today is {}.", formatted_date);

        self.sections.push(Section {
            title: "Current date".to_string(),
            content,
        });

        self
    }

    /// Build the final markdown string.
    pub fn build(&self) -> String {
        if self.sections.is_empty() {
            return String::new();
        }

        let mut output = String::new();
        output.push_str("<system-reminder>\n\n");
        output.push_str("# System Reminders\n\n");

        for (i, section) in self.sections.iter().enumerate() {
            if i > 0 {
                output.push('\n');
            }
            output.push_str(&format!("## {}\n\n", section.title));
            output.push_str(&section.content);
            output.push('\n');
        }

        output.push_str("\n</system-reminder>");
        output
    }

    /// Add the todo list section.
    pub fn with_todos(mut self, todos: &[TodoItem]) -> Self {
        if todos.is_empty() {
            return self;
        }

        let mut content = String::new();

        // Count incomplete todos
        let incomplete_count = todos
            .iter()
            .filter(|t| t.status != "completed" && t.status != "cancelled")
            .count();

        if incomplete_count == 0 {
            content.push_str("All tasks are complete!\n\n");
        } else {
            content.push_str(&format!(
                "You have {} task(s) in progress.\n\n",
                incomplete_count
            ));
        }

        // List all todos with status
        for (i, todo) in todos.iter().enumerate() {
            let priority = todo.format_priority();
            let status = todo.format_status();
            content.push_str(&format!(
                "{}. {} [{}] {}\n",
                i + 1,
                priority,
                status,
                todo.content
            ));
        }

        content.push_str("\n**Important**: Please update your progress with todos as soon as there's an update, rather than waiting until the end.");

        self.sections.push(Section {
            title: "Todo List".to_string(),
            content,
        });

        self
    }

    /// Check if the reminder has any content.
    pub fn is_empty(&self) -> bool {
        self.sections.is_empty()
    }
}

impl Default for SystemReminder {
    fn default() -> Self {
        Self::new()
    }
}

/// Tracks conversation date and detects changes.
pub struct DateTracker {
    current_date: DateTime<Utc>,
}

impl DateTracker {
    /// Create a new date tracker with the current time.
    pub fn new() -> Self {
        Self {
            current_date: Utc::now(),
        }
    }

    /// Create a date tracker with a specific starting date.
    pub fn with_date(date: DateTime<Utc>) -> Self {
        Self { current_date: date }
    }

    /// Check if the date has changed and update the tracker.
    /// Returns the date change message if the date changed, None otherwise.
    pub fn check_date_change(&mut self) -> Option<String> {
        let now = Utc::now();
        let old_date = self.current_date.date_naive();
        let new_date = now.date_naive();

        if old_date != new_date {
            let days_diff = new_date.signed_duration_since(old_date).num_days();
            self.current_date = now;

            let old_formatted = old_date.format("%B %d, %Y");
            let new_formatted = new_date.format("%B %d, %Y");

            let message = if days_diff == 1 {
                format!(
                    "<system-reminder>\n\n# Date Change\n\nNote: The date has changed. It is now {} (was {}).\n\n</system-reminder>",
                    new_formatted, old_formatted
                )
            } else {
                format!(
                    "<system-reminder>\n\n# Date Change\n\nNote: The date has changed by {} days. It is now {} (was {}).\n\n</system-reminder>",
                    days_diff.abs(), new_formatted, old_formatted
                )
            };

            Some(message)
        } else {
            None
        }
    }

    /// Get the current tracked date.
    pub fn current_date(&self) -> DateTime<Utc> {
        self.current_date
    }
}

impl Default for DateTracker {
    fn default() -> Self {
        Self::new()
    }
}

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

#[cfg(test)]
mod tests {
    use super::*;

    fn make_test_skills() -> Vec<SkillDefinition> {
        vec![
            SkillDefinition {
                id: "code-review".to_string(),
                title: "Code Review".to_string(),
                description: "Reviews code for quality and best practices".to_string(),
                content: "You are a code review expert...".to_string(),
                parameters_schema: None,
            },
            SkillDefinition {
                id: "commit".to_string(),
                title: "Commit".to_string(),
                description: "Writes conventional commit messages".to_string(),
                content: "Write conventional commits...".to_string(),
                parameters_schema: None,
            },
        ]
    }

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

    fn make_test_subagents() -> Vec<(String, String)> {
        vec![
            (
                "researcher".to_string(),
                "Searches and summarizes information".to_string(),
            ),
            (
                "coder".to_string(),
                "Writes and reviews code".to_string(),
            ),
        ]
    }

    #[test]
    fn test_reminder_with_subagents() {
        let subagents = make_test_subagents();
        let reminder = SystemReminder::new().with_subagents(&subagents);

        let output = reminder.build();

        assert!(!reminder.is_empty());
        assert!(output.contains("<system-reminder>"));
        assert!(output.contains("## Available sub-agents"));
        assert!(output.contains(
            "The following sub-agents are available for use with the Agent tool:"
        ));
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
    fn test_date_tracker_no_change_same_day() {
        let now = Utc::now();
        let mut tracker = DateTracker::with_date(now);

        // Should return None when no time has passed
        let result = tracker.check_date_change();
        assert!(result.is_none());
    }

    #[test]
    fn test_date_tracker_detects_change_next_day() {
        // Create a tracker for yesterday
        let yesterday = Utc::now() - chrono::Duration::days(1);
        let mut tracker = DateTracker::with_date(yesterday);

        // Check should detect the 1-day change
        let result = tracker.check_date_change();
        assert!(result.is_some());

        let message = result.unwrap();
        assert!(message.contains("<system-reminder>"));
        assert!(message.contains("# Date Change"));
        assert!(message.contains("The date has changed"));
        assert!(message.contains("(was "));
        assert!(message.contains("</system-reminder>"));
    }

    #[test]
    fn test_date_tracker_detects_multi_day_change() {
        // Create a tracker for 3 days ago
        let three_days_ago = Utc::now() - chrono::Duration::days(3);
        let mut tracker = DateTracker::with_date(three_days_ago);

        let result = tracker.check_date_change();
        assert!(result.is_some());

        let message = result.unwrap();
        assert!(message.contains("3 days"));
        assert!(message.contains("The date has changed by"));
    }

    #[test]
    fn test_date_tracker_updates_internal_date() {
        let yesterday = Utc::now() - chrono::Duration::days(1);
        let mut tracker = DateTracker::with_date(yesterday);

        let old_date = tracker.current_date();
        tracker.check_date_change();
        let new_date = tracker.current_date();

        // The tracker should have updated to today
        assert_ne!(old_date.date_naive(), new_date.date_naive());
        assert_eq!(new_date.date_naive(), Utc::now().date_naive());
    }

    #[test]
    fn test_date_tracker_no_false_positive_after_update() {
        let yesterday = Utc::now() - chrono::Duration::days(1);
        let mut tracker = DateTracker::with_date(yesterday);

        // First check detects change
        assert!(tracker.check_date_change().is_some());

        // Second check (same day) should not detect change
        assert!(tracker.check_date_change().is_none());
    }
}
