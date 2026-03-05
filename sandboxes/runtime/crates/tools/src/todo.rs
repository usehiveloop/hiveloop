use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::sync::RwLock;

use crate::registry::ToolExecutor;

/// Shared state for the todo list, accessible by both TodoWriteTool and TodoReadTool.
#[derive(Clone, Default)]
pub struct TodoState {
    inner: Arc<RwLock<Vec<TodoItemArg>>>,
}

impl TodoState {
    pub fn new() -> Self {
        Self::default()
    }

    /// Replace the entire todo list.
    pub async fn update(&self, todos: Vec<TodoItemArg>) {
        let mut guard = self.inner.write().await;
        *guard = todos;
    }

    /// Get a snapshot of the current todo list.
    pub async fn get(&self) -> Vec<TodoItemArg> {
        self.inner.read().await.clone()
    }
}

/// Arguments for the todowrite tool — replaces the entire todo list on each call.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct TodoWriteArgs {
    /// The complete updated todo list. Each call replaces the entire list.
    pub todos: Vec<TodoItemArg>,
}

/// A single todo item as provided by the LLM.
#[derive(Debug, Clone, Deserialize, Serialize, JsonSchema)]
pub struct TodoItemArg {
    /// Brief description of the task.
    pub content: String,
    /// Current status: pending, in_progress, completed, cancelled.
    pub status: String,
    /// Priority level: high, medium, low.
    pub priority: String,
}

/// Result returned by the todowrite tool (serialized as JSON).
#[derive(Debug, Serialize, Deserialize)]
pub struct TodoWriteResult {
    /// Number of items that are not yet completed.
    pub incomplete_count: usize,
    /// The full todo list.
    pub todos: Vec<TodoItemArg>,
}

/// Result returned by the todoread tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct TodoReadResult {
    /// The full todo list.
    pub todos: Vec<TodoItemArg>,
    /// Total number of items.
    pub total: usize,
    /// Number of items that are not yet completed.
    pub incomplete_count: usize,
}

/// Built-in tool that manages a task list with replace-all semantics.
///
/// The LLM sends the full list on every call — completed items, in-progress
/// items, and new items. The tool validates and returns the list; the
/// [`crate::tool_hook::ToolCallEmitter`] intercepts the result to emit a
/// structured `TodoUpdated` SSE event.
pub struct TodoWriteTool {
    state: TodoState,
}

impl Default for TodoWriteTool {
    fn default() -> Self {
        Self::new()
    }
}

impl TodoWriteTool {
    pub fn new() -> Self {
        Self {
            state: TodoState::new(),
        }
    }

    pub fn with_state(state: TodoState) -> Self {
        Self { state }
    }
}

#[async_trait]
impl ToolExecutor for TodoWriteTool {
    fn name(&self) -> &str {
        "todowrite"
    }

    fn description(&self) -> &str {
        include_str!("instructions/todowrite.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(TodoWriteArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: TodoWriteArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let incomplete_count = args
            .todos
            .iter()
            .filter(|t| t.status != "completed")
            .count();

        // Update shared state
        self.state.update(args.todos.clone()).await;

        let result = TodoWriteResult {
            incomplete_count,
            todos: args.todos,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

/// Built-in tool that reads the current todo list (no parameters).
pub struct TodoReadTool {
    state: TodoState,
}

impl Default for TodoReadTool {
    fn default() -> Self {
        Self::new()
    }
}

impl TodoReadTool {
    pub fn new() -> Self {
        Self {
            state: TodoState::new(),
        }
    }

    pub fn with_state(state: TodoState) -> Self {
        Self { state }
    }
}

/// Empty args for todoread — no parameters needed.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct TodoReadArgs {}

#[async_trait]
impl ToolExecutor for TodoReadTool {
    fn name(&self) -> &str {
        "todoread"
    }

    fn description(&self) -> &str {
        include_str!("instructions/todoread.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(TodoReadArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
        let todos = self.state.get().await;
        let total = todos.len();
        let incomplete_count = todos.iter().filter(|t| t.status != "completed").count();

        let result = TodoReadResult {
            todos,
            total,
            incomplete_count,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_todowrite_returns_correct_incomplete_count() {
        let tool = TodoWriteTool::new();
        let args = serde_json::json!({
            "todos": [
                { "content": "Step 1", "status": "completed", "priority": "high" },
                { "content": "Step 2", "status": "in_progress", "priority": "high" },
                { "content": "Step 3", "status": "pending", "priority": "medium" },
            ]
        });
        let result = tool.execute(args).await.unwrap();
        let parsed: TodoWriteResult = serde_json::from_str(&result).unwrap();
        assert_eq!(parsed.incomplete_count, 2);
        assert_eq!(parsed.todos.len(), 3);
    }

    #[tokio::test]
    async fn test_todowrite_empty_list() {
        let tool = TodoWriteTool::new();
        let args = serde_json::json!({ "todos": [] });
        let result = tool.execute(args).await.unwrap();
        let parsed: TodoWriteResult = serde_json::from_str(&result).unwrap();
        assert_eq!(parsed.incomplete_count, 0);
        assert!(parsed.todos.is_empty());
    }

    #[tokio::test]
    async fn test_todowrite_invalid_args() {
        let tool = TodoWriteTool::new();
        // Missing required 'todos' field
        let args = serde_json::json!({ "items": [] });
        let result = tool.execute(args).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Invalid arguments"));
    }

    #[tokio::test]
    async fn test_todoread_empty() {
        let tool = TodoReadTool::new();
        let result = tool.execute(serde_json::json!({})).await.unwrap();
        let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
        assert!(parsed.todos.is_empty());
        assert_eq!(parsed.total, 0);
        assert_eq!(parsed.incomplete_count, 0);
    }

    #[tokio::test]
    async fn test_todoread_after_write() {
        let state = TodoState::new();
        let write_tool = TodoWriteTool::with_state(state.clone());
        let read_tool = TodoReadTool::with_state(state);

        // Write some todos
        let args = serde_json::json!({
            "todos": [
                { "content": "Task A", "status": "pending", "priority": "high" },
                { "content": "Task B", "status": "completed", "priority": "low" },
            ]
        });
        write_tool.execute(args).await.unwrap();

        // Read them back
        let result = read_tool.execute(serde_json::json!({})).await.unwrap();
        let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
        assert_eq!(parsed.total, 2);
        assert_eq!(parsed.incomplete_count, 1);
        assert_eq!(parsed.todos[0].content, "Task A");
        assert_eq!(parsed.todos[1].content, "Task B");
    }

    #[tokio::test]
    async fn test_todoread_shared_state() {
        let state = TodoState::new();
        let write_tool = TodoWriteTool::with_state(state.clone());
        let read_tool = TodoReadTool::with_state(state);

        // Initially empty
        let result = read_tool.execute(serde_json::json!({})).await.unwrap();
        let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
        assert_eq!(parsed.total, 0);

        // Write
        write_tool
            .execute(serde_json::json!({
                "todos": [{ "content": "X", "status": "pending", "priority": "medium" }]
            }))
            .await
            .unwrap();

        // Read reflects write
        let result = read_tool.execute(serde_json::json!({})).await.unwrap();
        let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
        assert_eq!(parsed.total, 1);

        // Overwrite with empty
        write_tool
            .execute(serde_json::json!({ "todos": [] }))
            .await
            .unwrap();

        // Read reflects empty
        let result = read_tool.execute(serde_json::json!({})).await.unwrap();
        let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
        assert_eq!(parsed.total, 0);
    }
}
