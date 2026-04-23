//! `SystemReminder` builder and section model.

use bridge_core::SkillDefinition;
use chrono::{DateTime, Utc};

use super::TodoItem;

/// A flexible system reminder builder that generates markdown content
/// to be injected before user messages.
pub struct SystemReminder {
    sections: Vec<Section>,
}

/// Classifies a reminder section for prompt-cache layout.
///
/// The cache prefix is byte-sensitive: any drift near the top of the
/// prompt busts cache hits for the entire prefix. Bridge places stable
/// sections (skills, subagents — fixed for an agent's lifetime) in the
/// cacheable region and keeps volatile sections (date, todos, environment
/// snapshot) at the tail of the current user turn so they never
/// invalidate prior-turn caches.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum SectionStability {
    /// Bytes are stable across turns within an agent lifetime. Safe to
    /// place in the cacheable prefix.
    Stable,
    /// Bytes change turn-to-turn (timestamps, counters, live environment).
    /// Must live at the tail of the current user message.
    Volatile,
}

/// A section within the system reminder.
pub(super) struct Section {
    pub(super) title: String,
    pub(super) content: String,
    pub(super) stability: SectionStability,
}

impl SystemReminder {
    /// Create a new empty system reminder.
    pub fn new() -> Self {
        Self {
            sections: Vec::new(),
        }
    }

    /// Add the available skills section.
    ///
    /// Filters out skills that should not be visible to the model
    /// (disable_model_invocation=true or user_invocable=false).
    /// Annotates locally-discovered skills with a `(local)` suffix.
    pub fn with_skills(mut self, skills: &[SkillDefinition]) -> Self {
        // Filter skills the model should not see
        let visible: Vec<&SkillDefinition> = skills
            .iter()
            .filter(|s| {
                let fm = s.frontmatter.as_ref();
                let model_disabled = fm.and_then(|f| f.disable_model_invocation).unwrap_or(false);
                let not_user_invocable = fm.and_then(|f| f.user_invocable) == Some(false);
                !model_disabled && !not_user_invocable
            })
            .collect();

        if visible.is_empty() {
            return self;
        }

        let mut content = String::new();
        content.push_str("The following skills are available for use with the Skill tool:\n\n");

        for skill in &visible {
            content.push_str(&format!("- **{}** - {}\n", skill.title, skill.description));
        }

        self.sections.push(Section {
            title: "Available skills".to_string(),
            content,
            stability: SectionStability::Stable,
        });

        self
    }

    /// Add the available sub-agents section.
    pub fn with_subagents(mut self, subagents: &[(String, String)]) -> Self {
        if subagents.is_empty() {
            return self;
        }

        let mut content = String::new();
        content.push_str("The following sub-agents are available for use with the Agent tool:\n\n");

        for (name, description) in subagents {
            content.push_str(&format!("- **{}** - {}\n", name, description));
        }

        self.sections.push(Section {
            title: "Available sub-agents".to_string(),
            content,
            stability: SectionStability::Stable,
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
            stability: SectionStability::Volatile,
        });

        self
    }

    /// Build the final markdown string.
    pub fn build(&self) -> String {
        build_from_sections(self.sections.iter().collect::<Vec<_>>().as_slice())
    }

    /// Build the reminder split by stability for cache-aware placement.
    ///
    /// - `stable`: markdown containing only sections whose bytes are stable
    ///   across turns within an agent lifetime (skills, subagents). Place
    ///   this BEFORE the user's current message so it joins the cacheable
    ///   prefix.
    /// - `volatile`: markdown containing sections with turn-to-turn drift
    ///   (date, todos, immortal counters). Place this at the TAIL of the
    ///   current user message so it never invalidates prior-turn caches.
    ///
    /// Either half may be empty. Both are either empty or a full
    /// `<system-reminder>…</system-reminder>` block.
    pub fn build_split(&self) -> (String, String) {
        let stable: Vec<&Section> = self
            .sections
            .iter()
            .filter(|s| s.stability == SectionStability::Stable)
            .collect();
        let volatile: Vec<&Section> = self
            .sections
            .iter()
            .filter(|s| s.stability == SectionStability::Volatile)
            .collect();
        (build_from_sections(&stable), build_from_sections(&volatile))
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
            stability: SectionStability::Volatile,
        });

        self
    }

    /// Add immortal conversation context to the reminder.
    ///
    /// Informs the agent about the journal tools and current chain state so it
    /// knows to journal important information and understands context resets.
    pub fn with_immortal_context(mut self, chain_index: u32, journal_entry_count: usize) -> Self {
        let mut content = String::new();

        content.push_str(
            "This is an **immortal conversation** — your context window may be refreshed \
             to keep the conversation going indefinitely. When that happens, your journal \
             entries and a structured checkpoint carry forward into the fresh context.\n\n",
        );

        content.push_str(
            "You have two journal tools available:\n\
             - `journal_write`: Record key decisions, discoveries, user preferences, or \
               constraints. Only write high-signal entries — not routine actions.\n\
             - `journal_read`: Review your journal entries at any time.\n\n",
        );

        if chain_index > 0 {
            content.push_str(&format!(
                "**Current chain**: {} (context has been refreshed {} time{}). ",
                chain_index,
                chain_index,
                if chain_index == 1 { "" } else { "s" }
            ));
        }

        if journal_entry_count > 0 {
            content.push_str(&format!("**Journal entries**: {}.\n", journal_entry_count));
        } else {
            content.push_str("Your journal is empty — consider writing key decisions as you go.\n");
        }

        self.sections.push(Section {
            title: "Immortal Conversation".to_string(),
            content,
            // chain_index and journal_entry_count grow over the conversation,
            // so the block is not byte-stable — must go in the volatile tail.
            stability: SectionStability::Volatile,
        });

        self
    }

    /// Check if the reminder has any content.
    pub fn is_empty(&self) -> bool {
        self.sections.is_empty()
    }
}

/// Render a list of sections into the `<system-reminder>` envelope. Used by
/// both `build` and `build_split`.
pub(super) fn build_from_sections(sections: &[&Section]) -> String {
    if sections.is_empty() {
        return String::new();
    }
    let mut output = String::new();
    output.push_str("<system-reminder>\n\n");
    output.push_str("# System Reminders\n\n");
    for (i, section) in sections.iter().enumerate() {
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

impl Default for SystemReminder {
    fn default() -> Self {
        Self::new()
    }
}
