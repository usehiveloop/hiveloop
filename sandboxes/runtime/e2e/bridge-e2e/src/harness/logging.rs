use std::path::Path;

use super::types::{format_sse_for_log, now_str};
use super::TestHarness;

impl TestHarness {
    /// Returns the log directory path.
    pub fn log_dir(&self) -> &Path {
        &self.log_dir
    }

    /// Register a conversation_id → agent_id mapping and log the conversation
    /// header (agent info, system prompt, tools).
    pub async fn register_conversation(&self, conv_id: &str, agent_id: &str) {
        self.conversation_agents
            .lock()
            .unwrap()
            .insert(conv_id.to_string(), agent_id.to_string());

        let mut log = format!(
            "================================================================================\n\
             CONVERSATION STARTED\n\
             ================================================================================\n\
             Timestamp:       {}\n\
             Agent ID:        {}\n\
             Conversation ID: {}\n\n",
            now_str(),
            agent_id,
            conv_id
        );

        // Fetch agent info to log system prompt and tools
        if let Ok(resp) = self.get_agent(agent_id).await {
            if resp.status().is_success() {
                if let Ok(body) = resp.json::<serde_json::Value>().await {
                    if let Some(prompt) = body.get("system_prompt").and_then(|v| v.as_str()) {
                        log.push_str(&format!(
                            "================================================================================\n\
                             SYSTEM PROMPT\n\
                             ================================================================================\n\
                             {}\n\n",
                            prompt
                        ));
                    }
                    if let Some(tools) = body.get("tools").and_then(|v| v.as_array()) {
                        let tool_names: Vec<&str> = tools
                            .iter()
                            .filter_map(|t| t.get("name").and_then(|n| n.as_str()))
                            .collect();
                        if !tool_names.is_empty() {
                            log.push_str(&format!("Built-in Tools: {}\n\n", tool_names.join(", ")));
                        }
                    }
                    if let Some(mcp) = body.get("mcp_servers").and_then(|v| v.as_array()) {
                        let server_names: Vec<&str> = mcp
                            .iter()
                            .filter_map(|s| s.get("name").and_then(|n| n.as_str()))
                            .collect();
                        if !server_names.is_empty() {
                            log.push_str(&format!("MCP Servers: {}\n\n", server_names.join(", ")));
                        }
                    }
                    if let Some(subagents) = body.get("subagents").and_then(|v| v.as_array()) {
                        if !subagents.is_empty() {
                            let sub_ids: Vec<&str> = subagents
                                .iter()
                                .filter_map(|s| s.get("id").and_then(|n| n.as_str()))
                                .collect();
                            log.push_str(&format!("Subagents: {}\n\n", sub_ids.join(", ")));
                        }
                    }
                }
            }
        }

        self.append_log(agent_id, &log);
    }

    /// Get the log label (agent_id) for a conversation, or fall back to conv_id.
    pub(super) fn log_label(&self, conv_id: &str) -> String {
        self.conversation_agents
            .lock()
            .unwrap()
            .get(conv_id)
            .cloned()
            .unwrap_or_else(|| conv_id.to_string())
    }

    /// Stream log content for the given label (agent_id) to stderr so it
    /// appears in real-time during test runs.
    pub(super) fn append_log(&self, label: &str, content: &str) {
        for line in content.lines() {
            eprintln!("[{}] {}", label, line);
        }
    }

    /// Log a single SSE event to the appropriate log file.
    pub(super) fn log_sse_event(&self, conv_id: &str, event_type: &str, data: &serde_json::Value) {
        let label = self.log_label(conv_id);
        let formatted = format_sse_for_log(event_type, data);
        self.append_log(
            &label,
            &format!(
                "[{}] --- SSE: {} ---\n{}\n\n",
                now_str(),
                event_type,
                formatted
            ),
        );
    }
}
