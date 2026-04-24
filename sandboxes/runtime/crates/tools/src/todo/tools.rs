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
///
/// Intentionally scalar: we do NOT echo the todos the caller just sent. The
/// model already has them in its own tool_call arguments, and bridge's
/// `SystemReminder::with_todos` injects the live list into every user turn
/// so the model can always see the latest state. Echoing the full list back
/// on every call was adding ~1 KB per write × N later turns of carried
/// context — the dominant non-bash cost on long agentic runs.
#[derive(Debug, Serialize, Deserialize)]
pub struct TodoWriteResult {
    /// Always `true` on success — just an ack so the model sees a positive
    /// confirmation in the tool-role response rather than an empty object.
    pub ok: bool,
    /// Number of items that are not yet completed (status != "completed" /
    /// "cancelled"). The model usually only needs this scalar to decide
    /// what to work on next.
    pub incomplete_count: usize,
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

    /// Get access to the todo state
    pub fn state(&self) -> &TodoState {
        &self.state
    }
}

#[async_trait]
impl ToolExecutor for TodoWriteTool {
    fn name(&self) -> &str {
        "todowrite"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/todowrite.txt")
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
            .filter(|t| t.status != "completed" && t.status != "cancelled")
            .count();

        // Update shared state — the canonical place the volatile system
        // reminder reads from when injecting the current list into the next
        // user turn.
        self.state.update(args.todos).await;

        let result = TodoWriteResult {
            ok: true,
            incomplete_count,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
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

    /// Get access to the todo state
    pub fn state(&self) -> &TodoState {
        &self.state
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
        include_str!("../instructions/todoread.txt")
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

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
