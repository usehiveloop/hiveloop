use serde::{Deserialize, Serialize};

/// SSE event types emitted during a conversation response.
#[derive(Debug, Clone, Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum SseEvent {
    /// The assistant started generating a response.
    MessageStart {
        /// Conversation ID
        conversation_id: String,
        /// Provider-assigned message ID
        message_id: String,
    },
    /// A chunk of text content from the assistant.
    ContentDelta {
        /// The text chunk
        delta: String,
        /// Provider-assigned message ID
        message_id: String,
    },
    /// A tool call was initiated by the assistant.
    ToolCallStart {
        /// Tool call ID
        id: String,
        /// Name of the tool being called
        name: String,
        /// Arguments passed to the tool
        arguments: serde_json::Value,
    },
    /// A tool call completed with a result.
    ToolCallResult {
        /// Tool call ID
        id: String,
        /// Result from the tool execution
        result: String,
        /// Whether the tool execution errored
        is_error: bool,
        /// Duration of the tool call in milliseconds.
        #[serde(skip_serializing_if = "Option::is_none")]
        duration_ms: Option<u64>,
    },
    /// The assistant finished generating the response.
    MessageEnd {
        /// Provider-assigned message ID
        message_id: String,
        /// Token usage for this response
        usage: TokenUsage,
    },
    /// An error occurred during generation.
    Error {
        /// Error code
        code: String,
        /// Error message
        message: String,
    },
    /// The todo list was updated.
    TodoUpdated {
        /// The complete current todo list.
        todos: Vec<TodoItem>,
    },
    /// A tool call requires user approval before execution.
    ToolApprovalRequired {
        /// Unique ID for this approval request.
        request_id: String,
        /// Name of the tool being called.
        tool_name: String,
        /// The LLM's tool call ID.
        tool_call_id: String,
        /// Arguments passed to the tool.
        arguments: serde_json::Value,
        /// Integration name if this is an integration tool (e.g., "github").
        #[serde(skip_serializing_if = "Option::is_none")]
        integration_name: Option<String>,
        /// Integration action if this is an integration tool (e.g., "create_pull_request").
        #[serde(skip_serializing_if = "Option::is_none")]
        integration_action: Option<String>,
    },
    /// A tool approval request was resolved.
    ToolApprovalResolved {
        /// The approval request ID that was resolved.
        request_id: String,
        /// The decision: "approve" or "deny".
        decision: String,
    },
    /// The response stream is complete.
    Done,
    /// A background task (bash or subagent) completed.
    BackgroundTaskCompleted {
        /// The task ID that completed.
        task_id: String,
        /// Description of the task.
        description: String,
        /// The result output (JSON string for bash, plain text for subagent).
        output: String,
        /// Whether the task failed.
        is_error: bool,
    },
}

/// A single todo item in the task list.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TodoItem {
    /// Brief description of the task.
    pub content: String,
    /// Current status: pending, in_progress, completed, cancelled.
    pub status: String,
    /// Priority level: high, medium, low.
    pub priority: String,
}

/// Token usage information for a response.
#[derive(Debug, Clone, Serialize, Default)]
pub struct TokenUsage {
    /// Number of input tokens consumed
    pub input_tokens: u64,
    /// Number of output tokens generated
    pub output_tokens: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sse_event_serialization() {
        let event = SseEvent::ContentDelta {
            delta: "Hello".to_string(),
            message_id: "msg_1".to_string(),
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("content_delta"));
        assert!(json.contains("Hello"));
    }

    #[test]
    fn test_message_end_serialization() {
        let event = SseEvent::MessageEnd {
            message_id: "msg_1".to_string(),
            usage: TokenUsage {
                input_tokens: 100,
                output_tokens: 50,
            },
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("message_end"));
        assert!(json.contains("100"));
        assert!(json.contains("50"));
    }

    #[test]
    fn test_todo_updated_serialization() {
        let event = SseEvent::TodoUpdated {
            todos: vec![
                TodoItem {
                    content: "Implement feature".to_string(),
                    status: "in_progress".to_string(),
                    priority: "high".to_string(),
                },
                TodoItem {
                    content: "Write tests".to_string(),
                    status: "pending".to_string(),
                    priority: "medium".to_string(),
                },
            ],
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("todo_updated"));
        assert!(json.contains("Implement feature"));
        assert!(json.contains("in_progress"));
        assert!(json.contains("Write tests"));
        assert!(json.contains("pending"));
    }

    #[test]
    fn test_tool_call_result_with_duration() {
        let event = SseEvent::ToolCallResult {
            id: "tc_1".to_string(),
            result: "ok".to_string(),
            is_error: false,
            duration_ms: Some(42),
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("tool_call_result"));
        assert!(json.contains("\"duration_ms\":42"));
    }

    #[test]
    fn test_tool_call_result_without_duration() {
        let event = SseEvent::ToolCallResult {
            id: "tc_1".to_string(),
            result: "ok".to_string(),
            is_error: false,
            duration_ms: None,
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("tool_call_result"));
        assert!(!json.contains("duration_ms"));
    }

    #[test]
    fn test_done_serialization() {
        let event = SseEvent::Done;
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("done"));
    }

    #[test]
    fn test_background_task_completed_serialization() {
        let event = SseEvent::BackgroundTaskCompleted {
            task_id: "task-123".to_string(),
            description: "Run backend tests".to_string(),
            output: "test result: ok. 5 passed".to_string(),
            is_error: false,
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("background_task_completed"));
        assert!(json.contains("task-123"));
        assert!(json.contains("Run backend tests"));
        assert!(json.contains("test result: ok"));
        assert!(json.contains("\"is_error\":false"));
    }
}
