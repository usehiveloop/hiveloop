use std::collections::HashMap;

#[derive(Debug, Clone)]
pub struct ToolErrorTracker {
    failures: HashMap<String, u32>,
    max_per_tool: u32,
}

impl ToolErrorTracker {
    pub fn new(max_per_tool: u32) -> Self {
        Self {
            failures: HashMap::new(),
            max_per_tool,
        }
    }

    pub fn record_failure(&mut self, tool_name: &str) -> u32 {
        let count = self.failures.entry(tool_name.to_string()).or_insert(0);
        *count += 1;
        *count
    }

    pub fn remaining_attempts(&self, tool_name: &str) -> u32 {
        let used = self.failures.get(tool_name).copied().unwrap_or(0);
        self.max_per_tool.saturating_sub(used)
    }

    pub fn is_exhausted(&self, tool_name: &str) -> bool {
        self.remaining_attempts(tool_name) == 0
    }

    pub fn reset(&mut self, tool_name: &str) {
        self.failures.remove(tool_name);
    }

    pub fn format_retry_hint(&self, tool_name: &str, error: &str) -> String {
        let remaining = self.remaining_attempts(tool_name);
        if remaining > 0 {
            format!(
                "{error}\n\nAttempts remaining for '{tool_name}': {remaining}. \
                 Analyze the error, identify the root cause, and adjust your approach before retrying."
            )
        } else {
            format!(
                "{error}\n\nTool '{tool_name}' has exhausted all {} attempts. \
                 Try a different tool or approach.",
                self.max_per_tool
            )
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn tracks_failures_per_tool() {
        let mut tracker = ToolErrorTracker::new(3);
        assert_eq!(tracker.record_failure("bash"), 1);
        assert_eq!(tracker.record_failure("bash"), 2);
        assert_eq!(tracker.record_failure("bash"), 3);
        assert!(tracker.is_exhausted("bash"));
        assert_eq!(tracker.remaining_attempts("bash"), 0);
    }

    #[test]
    fn resets_on_success() {
        let mut tracker = ToolErrorTracker::new(3);
        tracker.record_failure("bash");
        tracker.record_failure("bash");
        tracker.reset("bash");
        assert_eq!(tracker.remaining_attempts("bash"), 3);
        assert!(!tracker.is_exhausted("bash"));
    }

    #[test]
    fn tracks_different_tools_separately() {
        let mut tracker = ToolErrorTracker::new(3);
        tracker.record_failure("bash");
        tracker.record_failure("bash");
        tracker.record_failure("read_file");
        assert_eq!(tracker.remaining_attempts("bash"), 1);
        assert_eq!(tracker.remaining_attempts("read_file"), 2);
    }

    #[test]
    fn format_retry_hint_with_remaining() {
        let mut tracker = ToolErrorTracker::new(3);
        tracker.record_failure("bash");
        let hint = tracker.format_retry_hint("bash", "command not found");
        assert!(hint.contains("Attempts remaining for 'bash': 2"));
        assert!(hint.contains("command not found"));
    }

    #[test]
    fn format_retry_hint_exhausted() {
        let mut tracker = ToolErrorTracker::new(1);
        tracker.record_failure("bash");
        let hint = tracker.format_retry_hint("bash", "command not found");
        assert!(hint.contains("exhausted all 1 attempts"));
    }
}
