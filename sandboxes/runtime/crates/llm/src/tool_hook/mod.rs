//! [`ToolCallEmitter`] — a [`rig::agent::PromptHook`] implementation that
//! emits [`bridge_core::event::BridgeEvent`]s through the [`EventBus`]
//! whenever the agent loop invokes a tool.
//!
//! Also intercepts bash tool calls with `background: true` to spawn them
//! asynchronously and send a notification when they complete. This is
//! handled here (rather than in the bash tool's execute method) because
//! rig-core's tool server dispatches tool calls in separate
//! `tokio::spawn` tasks, which lose the `AGENT_CONTEXT` task_local. The
//! hook runs in the original task scope where `AGENT_CONTEXT` is
//! available.
//!
//! Additionally intercepts unknown tool names and returns helpful error
//! messages with suggestions (case-insensitive match or Levenshtein
//! distance).

use std::collections::{HashMap, HashSet};
use std::sync::{Arc, Mutex};
use std::time::Instant;

use bridge_core::conversation::Message;
use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::permission::ToolPermission;
use bridge_core::AgentMetrics;
use dashmap::DashMap;
use serde_json::json;
use storage::StorageHandle;
use tokio_util::sync::CancellationToken;
use tools::ToolExecutor;
use webhooks::EventBus;

use crate::permission_manager::PermissionManager;

mod background;
pub(crate) mod coerce;
mod execute;
mod hook_impl;
mod name_resolution;
mod permission;
mod persist;
pub mod repeat_guard;
pub(crate) mod result_classify;
mod result_hook;
mod self_agent;
mod sub_agent;
mod truncate;

pub use repeat_guard::RepeatGuardState;

#[cfg(test)]
mod tests;

/// A [`PromptHook`] that emits [`BridgeEvent`]s through the [`EventBus`]
/// whenever the agent loop invokes a tool.
///
/// Also intercepts bash tool calls with `background: true` to spawn them
/// asynchronously and send a notification when they complete. This is handled
/// here (rather than in the bash tool's execute method) because rig-core's
/// tool server dispatches tool calls in separate `tokio::spawn` tasks, which
/// lose the `AGENT_CONTEXT` task_local. The hook runs in the original task
/// scope where `AGENT_CONTEXT` is available.
///
/// Additionally intercepts unknown tool names and returns helpful error
/// messages with suggestions (case-insensitive match or Levenshtein distance).
#[derive(Clone)]
pub struct ToolCallEmitter {
    pub event_bus: Arc<EventBus>,
    pub cancel: CancellationToken,
    /// Known tool names for tool repair. When populated, unknown tool names
    /// are intercepted and a helpful suggestion is returned instead of letting
    /// rig-core return a generic error.
    pub tool_names: HashSet<String>,
    /// Tool executors keyed by canonical name. Used to execute tools directly
    /// when the LLM-provided name was auto-repaired (trimmed, case-fixed, etc.)
    /// and rig-core would not find the tool under the original name.
    pub tool_executors: HashMap<String, Arc<dyn ToolExecutor>>,
    /// Agent ID for event payloads.
    pub agent_id: String,
    /// Conversation ID for event payloads.
    pub conversation_id: String,
    /// Permission manager for handling tool approval requests.
    pub permission_manager: Arc<PermissionManager>,
    /// Per-tool permission overrides for this agent.
    pub agent_permissions: HashMap<String, ToolPermission>,
    /// Shared metrics for recording per-tool stats.
    pub metrics: Arc<AgentMetrics>,
    /// Per-conversation metrics for token/tool tracking.
    pub conversation_metrics: Option<Arc<bridge_core::metrics::ConversationMetrics>>,
    /// Pending tool call start times, keyed by tool_call_id.
    /// Used to measure latency for rig-core dispatched tools where
    /// timing spans on_tool_call → on_tool_result.
    pub pending_tool_timings: Arc<DashMap<String, (Instant, String)>>,
    /// Optional storage handle for incremental persistence after each tool call.
    pub storage: Option<StorageHandle>,
    /// Shared persisted messages — updated incrementally after each tool interaction.
    pub persisted_messages: Option<Arc<Mutex<Vec<Message>>>>,
    /// Optional mid-turn context-pressure threshold (bytes). When cumulative
    /// tool-output bytes this turn exceed this value, a one-shot
    /// `ContextPressureWarning` event is emitted. `None` disables the check.
    pub pressure_threshold_bytes: Option<usize>,
    /// Cumulative tool-output bytes for the current turn. Owned by each
    /// turn's emitter clone (see `conversation.rs`) — reset implicitly when
    /// a fresh emitter is constructed next turn.
    pub pressure_counter: Arc<std::sync::atomic::AtomicUsize>,
    /// Flag so ContextPressureWarning is only emitted once per turn.
    pub pressure_warned: Arc<std::sync::atomic::AtomicBool>,
    /// Shared across all turns in this conversation. Tracks consecutive
    /// identical tool calls so we can short-circuit a runaway model loop
    /// (e.g. Qwen3.6-plus re-emitting the same Read every turn). See
    /// `repeat_guard.rs` for the policy.
    pub repeat_guard: Arc<Mutex<RepeatGuardState>>,
}

impl ToolCallEmitter {
    /// Record `bytes_added` of tool output this turn and emit a one-shot
    /// `ContextPressureWarning` if cumulative bytes cross the configured
    /// threshold. No-op if no threshold was configured.
    pub(super) fn note_tool_output_bytes(&self, bytes_added: usize) {
        let Some(threshold) = self.pressure_threshold_bytes else {
            return;
        };
        use std::sync::atomic::Ordering;
        if self.pressure_warned.load(Ordering::Relaxed) {
            // Still count bytes, but don't re-warn.
            self.pressure_counter
                .fetch_add(bytes_added, Ordering::Relaxed);
            return;
        }
        let new_total = self
            .pressure_counter
            .fetch_add(bytes_added, Ordering::Relaxed)
            + bytes_added;
        if new_total >= threshold && !self.pressure_warned.swap(true, Ordering::Relaxed) {
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ContextPressureWarning,
                &self.agent_id,
                &self.conversation_id,
                json!({
                    "cumulative_tool_output_bytes": new_total,
                    "threshold_bytes": threshold,
                    "reason": "tool_output_accumulation",
                }),
            ));
        }
    }
}
