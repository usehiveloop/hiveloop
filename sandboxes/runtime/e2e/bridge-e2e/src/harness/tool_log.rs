use anyhow::{anyhow, Result};

use super::types::ToolCallLogEntry;
use super::TestHarness;

impl TestHarness {
    /// Read the mock Portal MCP tool call log files.
    pub fn read_tool_call_log(&self) -> Result<Vec<ToolCallLogEntry>> {
        let log_dir = self
            .tool_log_dir
            .as_ref()
            .ok_or_else(|| anyhow!("no tool log directory configured (not using real agents?)"))?;

        let mut entries = Vec::new();

        if let Ok(dir_entries) = std::fs::read_dir(log_dir) {
            for entry in dir_entries.flatten() {
                let path = entry.path();
                if path.extension().is_some_and(|e| e == "jsonl") {
                    if let Ok(content) = std::fs::read_to_string(&path) {
                        for line in content.lines() {
                            let line = line.trim();
                            if line.is_empty() {
                                continue;
                            }
                            if let Ok(entry) = serde_json::from_str::<ToolCallLogEntry>(line) {
                                entries.push(entry);
                            }
                        }
                    }
                }
            }
        }

        Ok(entries)
    }

    /// Assert that a specific tool was called at least once.
    pub fn assert_tool_called(&self, tool_name: &str) -> Result<()> {
        let entries = self.read_tool_call_log()?;
        if entries.iter().any(|e| e.tool_name == tool_name) {
            Ok(())
        } else {
            let called_tools: Vec<&str> = entries.iter().map(|e| e.tool_name.as_str()).collect();
            Err(anyhow!(
                "tool '{}' was never called. Tools called: {:?}",
                tool_name,
                called_tools
            ))
        }
    }

    /// Assert that at least one of the given tools was called.
    pub fn assert_any_tool_called(&self, tool_names: &[&str]) -> Result<()> {
        let entries = self.read_tool_call_log()?;
        if entries
            .iter()
            .any(|e| tool_names.contains(&e.tool_name.as_str()))
        {
            Ok(())
        } else {
            let called_tools: Vec<&str> = entries.iter().map(|e| e.tool_name.as_str()).collect();
            Err(anyhow!(
                "none of {:?} were called. Tools called: {:?}",
                tool_names,
                called_tools
            ))
        }
    }

    /// Assert tool called with args matching a predicate.
    pub fn assert_tool_called_with(
        &self,
        tool_name: &str,
        predicate: impl Fn(&serde_json::Value) -> bool,
    ) -> Result<()> {
        let entries = self.read_tool_call_log()?;
        let matching: Vec<_> = entries
            .iter()
            .filter(|e| e.tool_name == tool_name)
            .collect();

        if matching.is_empty() {
            let called_tools: Vec<&str> = entries.iter().map(|e| e.tool_name.as_str()).collect();
            return Err(anyhow!(
                "tool '{}' was never called. Tools called: {:?}",
                tool_name,
                called_tools
            ));
        }

        if matching.iter().any(|e| predicate(&e.arguments)) {
            Ok(())
        } else {
            Err(anyhow!(
                "tool '{}' was called {} times but no call matched the predicate",
                tool_name,
                matching.len()
            ))
        }
    }
}
