//! Structured intermediate representation for compacted history.
//!
//! Mirrors forgecode's `ContextSummary` exactly: extraction is a pure function
//! from `&[Message]` to this structured tree, then a separate render step
//! emits the final markdown. The two-step design lets transformers
//! (dedupe consecutive ops, strip working dir, drop system messages) operate
//! on the structured form before rendering.

use serde::{Deserialize, Serialize};

/// Top-level structured summary of a contiguous slice of conversation.
#[derive(Default, Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ContextSummary {
    pub messages: Vec<SummaryBlock>,
}

/// One role-grouped block of consecutive messages from the same role.
/// Forgecode groups consecutive same-role messages into a single block so the
/// rendered markdown reads as `### N. User` / `### N. Assistant` sections,
/// not one section per message.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SummaryBlock {
    pub role: SummaryRole,
    pub contents: Vec<SummaryMessage>,
}

/// Role label used in the rendered output. Mirrors forgecode's `Role`.
#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub enum SummaryRole {
    User,
    Assistant,
}

impl SummaryRole {
    pub fn label(&self) -> &'static str {
        match self {
            SummaryRole::User => "User",
            SummaryRole::Assistant => "Assistant",
        }
    }
}

/// One content block within a `SummaryBlock`. Either plain text or a
/// summarised tool call (with success/failure marker linked from the result).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum SummaryMessage {
    Text(String),
    ToolCall(SummaryToolCall),
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SummaryToolCall {
    /// Tool call identifier (used to link the call to its result).
    pub id: Option<String>,
    pub tool: SummaryTool,
    /// Set to `true` once the matching `ToolResult` is found and the result
    /// payload doesn't look like an error. Defaults to `true` when no result
    /// exists in the compacted range (e.g. a tool call that didn't complete
    /// before cancellation — rare).
    pub is_success: bool,
}

/// Per-tool structured payload. Mirrors forgecode's `SummaryTool` enum but
/// only includes the tools bridge actually has. Unknown tools fall through
/// to `Mcp { name }`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum SummaryTool {
    /// `Read` tool.
    FileRead { path: String },
    /// `write` / `edit` / `multiedit` — all unified as "Update".
    FileUpdate { path: String },
    /// Shell command (`bash`).
    Shell { command: String },
    /// File-pattern search (`Glob`).
    Glob { pattern: String },
    /// Grep-style search (`RipGrep`).
    Search {
        pattern: String,
        path: Option<String>,
    },
    /// Directory listing (`LS`).
    List { path: String },
    /// Todo list write — captures the diff against the previous snapshot.
    TodoWrite { changes: Vec<TodoChange> },
    /// Todo list read.
    TodoRead,
    /// LSP query (action + target).
    Lsp {
        action: String,
        target: Option<String>,
    },
    /// Generic fallback for unknown tools (MCP, custom integrations).
    /// Renders as `**{name}**` only — the args are intentionally dropped
    /// because they're often huge and the call's result is what matters.
    Mcp { name: String },
}

/// One change to a todo list, computed by diffing the new `todowrite` payload
/// against the snapshot of the previous list state.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct TodoChange {
    pub content: String,
    pub status: TodoStatus,
    pub kind: TodoChangeKind,
}

#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub enum TodoStatus {
    Pending,
    InProgress,
    Completed,
    Cancelled,
}

#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub enum TodoChangeKind {
    Added,
    Updated,
    Removed,
}
