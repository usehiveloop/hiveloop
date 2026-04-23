use bridge_core::conversation::Message;
use std::sync::Arc;

/// Mutable per-conversation state that survives across turns.
/// Extracted so `run_conversation` doesn't need to declare 10 locals inline.
pub(super) struct LoopState {
    pub(super) history: Vec<rig::message::Message>,
    pub(super) persisted_messages: Arc<std::sync::Mutex<Vec<Message>>>,
    pub(super) turn_count: usize,
    pub(super) history_fp: crate::history_guard::HistoryFingerprint,
    pub(super) msg_id: String,
    pub(super) date_tracker: crate::system_reminder::DateTracker,
    pub(super) immortal_state: Option<crate::immortal::ImmortalState>,
    pub(super) enforcement_state: Option<crate::tool_enforcement::ToolEnforcementState>,
    pub(super) pending_tool_reminder: Option<String>,
}

impl LoopState {
    /// Build the initial loop state from the conversation's seed history,
    /// persisted messages, and immortal/enforcement config.
    pub(super) fn new(
        initial_history: Option<Vec<rig::message::Message>>,
        initial_persisted_messages: Option<Vec<Message>>,
        conversation_date: chrono::DateTime<chrono::Utc>,
        immortal_config: &Option<bridge_core::agent::ImmortalConfig>,
        journal_state: &Option<Arc<tools::journal::JournalState>>,
        tool_requirements: &[bridge_core::agent::ToolRequirement],
    ) -> Self {
        let history: Vec<rig::message::Message> = initial_history.unwrap_or_default();
        let persisted_messages: Arc<std::sync::Mutex<Vec<Message>>> = Arc::new(
            std::sync::Mutex::new(initial_persisted_messages.unwrap_or_default()),
        );
        let history_fp = crate::history_guard::HistoryFingerprint::take(&history);
        let msg_id = uuid::Uuid::new_v4().to_string();
        let date_tracker = crate::system_reminder::DateTracker::with_date(conversation_date);

        let immortal_state = immortal_config.as_ref().map(|_| {
            let chain_index = journal_state
                .as_ref()
                .map(|js| js.chain_index())
                .unwrap_or(0);
            crate::immortal::ImmortalState {
                current_chain_index: chain_index,
            }
        });

        let enforcement_state = if tool_requirements.is_empty() {
            None
        } else {
            Some(crate::tool_enforcement::ToolEnforcementState::new())
        };

        Self {
            history,
            persisted_messages,
            turn_count: 0,
            history_fp,
            msg_id,
            date_tracker,
            immortal_state,
            enforcement_state,
            pending_tool_reminder: None,
        }
    }
}
