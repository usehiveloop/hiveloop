//! System reminder injection for conversations.
//!
//! Provides a flexible markdown-based system reminder that is injected
//! before every user message. The reminder can contain multiple sections
//! (skills, date, context, etc.) that help guide the agent's behavior.

mod builder;
mod convenience;
mod date_tracker;
#[cfg(test)]
mod tests;

pub use builder::{SectionStability, SystemReminder};
pub use convenience::{
    create_reminder_with_skills, create_reminder_with_skills_and_date,
    create_reminder_with_skills_todos_and_date,
};
pub use date_tracker::DateTracker;

/// A todo item for display in the system reminder
#[derive(Clone)]
pub struct TodoItem {
    pub content: String,
    pub status: String,
    pub priority: String,
}

impl TodoItem {
    /// Format status for display
    pub(super) fn format_status(&self) -> String {
        self.status.clone()
    }

    /// Format priority for display
    pub(super) fn format_priority(&self) -> String {
        match self.priority.as_str() {
            "high" => "[high]".to_string(),
            "medium" => "[medium]".to_string(),
            "low" => "[low]".to_string(),
            _ => "".to_string(),
        }
    }
}
