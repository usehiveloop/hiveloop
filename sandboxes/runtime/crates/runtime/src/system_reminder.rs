//! System reminder injection for conversations.
//!
//! Provides a flexible markdown-based system reminder that is injected
//! before every user message. The reminder can contain multiple sections
//! (skills, context, etc.) that help guide the agent's behavior.

use bridge_core::SkillDefinition;

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

/// Create a system reminder with skills section.
pub fn create_reminder_with_skills(skills: &[SkillDefinition]) -> String {
    if skills.is_empty() {
        return String::new();
    }

    SystemReminder::new().with_skills(skills).build()
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
            },
            SkillDefinition {
                id: "commit".to_string(),
                title: "Commit".to_string(),
                description: "Writes conventional commit messages".to_string(),
                content: "Write conventional commits...".to_string(),
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
    fn test_reminder_empty_skills() {
        let skills: Vec<SkillDefinition> = vec![];
        let reminder = SystemReminder::new().with_skills(&skills);

        assert!(reminder.is_empty());
        assert_eq!(reminder.build(), "");
    }

    #[test]
    fn test_convenience_function() {
        let skills = make_test_skills();
        let output = create_reminder_with_skills(&skills);

        assert!(output.contains("<system-reminder>"));
        assert!(output.contains("Available skills"));
        assert!(output.contains("Code Review"));
        assert!(output.contains("Commit"));
    }

    #[test]
    fn test_convenience_function_empty() {
        let skills: Vec<SkillDefinition> = vec![];
        let output = create_reminder_with_skills(&skills);

        assert_eq!(output, "");
    }
}
