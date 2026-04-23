//! Date tracking / change detection for the reminder pipeline.

use chrono::{DateTime, Utc};

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
