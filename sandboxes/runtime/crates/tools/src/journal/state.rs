use serde::{Deserialize, Serialize};
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Arc;
use tokio::sync::RwLock;

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
///
/// Two kinds of entries live here:
///   - **committed** entries, durably persisted, survive across chain handoffs;
///   - **staged** entries, live only in-memory until the current turn commits.
///
/// Staged writes exist so journal_write calls during a failed/rolled-back turn
/// don't permanently pollute the journal. The agent still sees staged entries
/// via [`entries()`] during the turn — they're just not durable yet.
#[derive(Clone)]
pub struct JournalState {
    entries: Arc<RwLock<Vec<JournalEntry>>>,
    /// Pending entries from the current turn. Flushed on [`commit_staged`] or
    /// dropped on [`discard_staged`].
    staged: Arc<RwLock<Vec<JournalEntry>>>,
    conversation_id: String,
    chain_index: Arc<AtomicU32>,
    storage: Option<storage::StorageHandle>,
}

impl JournalState {
    pub fn new(conversation_id: String, storage: Option<storage::StorageHandle>) -> Self {
        Self {
            entries: Arc::new(RwLock::new(Vec::new())),
            staged: Arc::new(RwLock::new(Vec::new())),
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
            staged: Arc::new(RwLock::new(Vec::new())),
            conversation_id,
            chain_index: Arc::new(AtomicU32::new(chain_index)),
            storage,
        }
    }

    /// Append an entry to the committed journal and persist to storage immediately.
    /// Use this for system-generated entries (checkpoints) that must survive
    /// even if the surrounding turn fails.
    pub async fn append(&self, entry: JournalEntry) {
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
        self.entries.write().await.push(entry);
    }

    /// Stage an entry in the current turn's pending buffer. Persists only
    /// after [`commit_staged`] is called at turn-commit time.
    /// Use this for agent-initiated entries (journal_write tool) so they
    /// don't permanently accumulate when a turn is rolled back on error.
    pub async fn append_staged(&self, entry: JournalEntry) {
        self.staged.write().await.push(entry);
    }

    /// Flush staged entries into the committed log + storage. Called at turn
    /// success in the conversation loop.
    pub async fn commit_staged(&self) -> usize {
        let drained: Vec<JournalEntry> = {
            let mut guard = self.staged.write().await;
            std::mem::take(&mut *guard)
        };
        let count = drained.len();
        if count == 0 {
            return 0;
        }
        if let Some(storage) = &self.storage {
            for entry in &drained {
                storage.append_journal_entry(
                    entry.id.clone(),
                    self.conversation_id.clone(),
                    entry.chain_index,
                    entry.entry_type.clone(),
                    entry.content.clone(),
                    entry.timestamp,
                );
            }
        }
        self.entries.write().await.extend(drained);
        count
    }

    /// Drop staged entries without persisting. Called on turn error / rollback.
    pub async fn discard_staged(&self) -> usize {
        let mut guard = self.staged.write().await;
        let n = guard.len();
        guard.clear();
        n
    }

    /// Get a snapshot of all journal entries (committed + staged, chronological).
    /// The agent sees its own pending writes during the current turn via this.
    pub async fn entries(&self) -> Vec<JournalEntry> {
        let committed = self.entries.read().await;
        let staged = self.staged.read().await;
        let mut out = Vec::with_capacity(committed.len() + staged.len());
        out.extend_from_slice(&committed);
        out.extend_from_slice(&staged);
        out
    }

    /// Get only the committed entries (excludes staged).
    /// Used by the chain handoff engine so in-flight writes aren't passed to
    /// the checkpoint summarizer.
    pub async fn committed_entries(&self) -> Vec<JournalEntry> {
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
