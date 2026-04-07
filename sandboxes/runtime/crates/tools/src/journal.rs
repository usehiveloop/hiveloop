use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Arc;
use tokio::sync::RwLock;

use crate::registry::ToolExecutor;

/// A single journal entry stored in memory and persisted to storage.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JournalEntry {
    pub id: String,
    pub chain_index: u32,
    pub entry_type: String,
    pub content: String,
    pub category: Option<String>,
    pub timestamp: chrono::DateTime<chrono::Utc>,
}

/// Shared state for the journal, accessible by the tool and the chain handoff engine.
#[derive(Clone)]
pub struct JournalState {
    entries: Arc<RwLock<Vec<JournalEntry>>>,
    conversation_id: String,
    chain_index: Arc<AtomicU32>,
    storage: Option<storage::StorageHandle>,
}

impl JournalState {
    pub fn new(conversation_id: String, storage: Option<storage::StorageHandle>) -> Self {
        Self {
            entries: Arc::new(RwLock::new(Vec::new())),
            conversation_id,
            chain_index: Arc::new(AtomicU32::new(0)),
            storage,
        }
    }

    /// Create a JournalState pre-populated with existing entries (for hydration).
    pub fn with_entries(
        conversation_id: String,
        storage: Option<storage::StorageHandle>,
        entries: Vec<JournalEntry>,
        chain_index: u32,
    ) -> Self {
        Self {
            entries: Arc::new(RwLock::new(entries)),
            conversation_id,
            chain_index: Arc::new(AtomicU32::new(chain_index)),
            storage,
        }
    }

    /// Append an entry to the in-memory journal and persist to storage.
    pub async fn append(&self, entry: JournalEntry) {
        // Persist to storage (fire-and-forget)
        if let Some(storage) = &self.storage {
            storage.append_journal_entry(
                entry.id.clone(),
                self.conversation_id.clone(),
                entry.chain_index,
                entry.entry_type.clone(),
                entry.content.clone(),
                entry.timestamp,
            );
        }

        // Append to in-memory state
        self.entries.write().await.push(entry);
    }

    /// Get a snapshot of all journal entries.
    pub async fn entries(&self) -> Vec<JournalEntry> {
        self.entries.read().await.clone()
    }

    /// Get the current chain index.
    pub fn chain_index(&self) -> u32 {
        self.chain_index.load(Ordering::Relaxed)
    }

    /// Update the chain index (called after a chain handoff).
    pub fn set_chain_index(&self, index: u32) {
        self.chain_index.store(index, Ordering::Relaxed);
    }

    /// Get the conversation ID.
    pub fn conversation_id(&self) -> &str {
        &self.conversation_id
    }
}

/// Arguments for the journal_write tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct JournalWriteArgs {
    /// The journal entry content. Should capture key decisions, discoveries,
    /// or insights that would be valuable if conversation context is reset.
    pub content: String,
    /// Optional category: "decision", "discovery", "blocker", "progress", "preference".
    #[serde(default)]
    pub category: Option<String>,
}

/// Result returned by the journal_write tool.
#[derive(Debug, Serialize)]
pub struct JournalWriteResult {
    pub entry_id: String,
    pub total_entries: usize,
}

/// Result returned by the journal_read tool.
#[derive(Debug, Serialize)]
pub struct JournalReadResult {
    pub entries: Vec<JournalEntryView>,
    pub total: usize,
}

/// A journal entry as returned by the read tool.
#[derive(Debug, Serialize)]
pub struct JournalEntryView {
    pub id: String,
    pub chain_index: u32,
    pub entry_type: String,
    pub content: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub category: Option<String>,
    pub timestamp: String,
}

/// Built-in tool that allows the agent to write to a persistent journal.
///
/// The journal survives conversation chain handoffs (context resets) and is
/// injected into the new context at the start of each chain link. Use it to
/// record high-signal information: key decisions, user preferences, architectural
/// choices, or important discoveries.
pub struct JournalWriteTool {
    state: Arc<JournalState>,
}

impl JournalWriteTool {
    pub fn new(state: Arc<JournalState>) -> Self {
        Self { state }
    }

    /// Get access to the journal state.
    pub fn state(&self) -> &Arc<JournalState> {
        &self.state
    }
}

#[async_trait]
impl ToolExecutor for JournalWriteTool {
    fn name(&self) -> &str {
        "journal_write"
    }

    fn description(&self) -> &str {
        include_str!("instructions/journal_write.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(JournalWriteArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: JournalWriteArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let entry = JournalEntry {
            id: uuid::Uuid::new_v4().to_string(),
            chain_index: self.state.chain_index(),
            entry_type: "agent_note".to_string(),
            content: args.content,
            category: args.category,
            timestamp: chrono::Utc::now(),
        };

        let entry_id = entry.id.clone();
        self.state.append(entry).await;

        let total_entries = self.state.entries().await.len();

        let result = JournalWriteResult {
            entry_id,
            total_entries,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

/// Built-in tool that reads the conversation journal (no parameters).
pub struct JournalReadTool {
    state: Arc<JournalState>,
}

impl JournalReadTool {
    pub fn new(state: Arc<JournalState>) -> Self {
        Self { state }
    }
}

/// Empty args for journal_read — no parameters needed.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct JournalReadArgs {}

#[async_trait]
impl ToolExecutor for JournalReadTool {
    fn name(&self) -> &str {
        "journal_read"
    }

    fn description(&self) -> &str {
        include_str!("instructions/journal_read.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(JournalReadArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
        let entries = self.state.entries().await;

        let views: Vec<JournalEntryView> = entries
            .iter()
            .map(|e| JournalEntryView {
                id: e.id.clone(),
                chain_index: e.chain_index,
                entry_type: e.entry_type.clone(),
                content: e.content.clone(),
                category: e.category.clone(),
                timestamp: e.timestamp.to_rfc3339(),
            })
            .collect();

        let total = views.len();
        let result = JournalReadResult {
            entries: views,
            total,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
