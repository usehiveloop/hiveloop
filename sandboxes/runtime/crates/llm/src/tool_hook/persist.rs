//! Incremental persistence of tool call + result pairs to SQLite.

use bridge_core::conversation::{
    ContentBlock, Message, Role, ToolCall, ToolResult as BridgeToolResult,
};

use super::ToolCallEmitter;

impl ToolCallEmitter {
    /// Persist a tool call + result pair incrementally to SQLite.
    ///
    /// This is fire-and-forget: the write is enqueued on the storage channel
    /// and does not block the tool execution path. At turn end, the
    /// authoritative rebuild from rig's enriched_history replaces these
    /// incremental messages, ensuring consistency.
    pub(super) fn persist_tool_interaction(
        &self,
        tool_name: &str,
        tool_call_id: &str,
        args: &serde_json::Value,
        result: &str,
        is_error: bool,
    ) {
        let (Some(storage), Some(shared)) = (&self.storage, &self.persisted_messages) else {
            return;
        };

        let tool_call_msg = Message {
            role: Role::Assistant,
            content: vec![ContentBlock::ToolCall(ToolCall {
                id: tool_call_id.to_string(),
                name: tool_name.to_string(),
                arguments: args.clone(),
            })],
            timestamp: chrono::Utc::now(),
            system_reminder: None,
        };

        let tool_result_msg = Message {
            role: Role::Tool,
            content: vec![ContentBlock::ToolResult(BridgeToolResult {
                tool_call_id: tool_call_id.to_string(),
                content: result.to_string(),
                is_error,
            })],
            timestamp: chrono::Utc::now(),
            system_reminder: None,
        };

        let messages = {
            let mut guard = shared.lock().unwrap();
            guard.push(tool_call_msg);
            guard.push(tool_result_msg);
            guard.clone()
        };

        storage.replace_messages(self.conversation_id.clone(), messages);
    }
}
