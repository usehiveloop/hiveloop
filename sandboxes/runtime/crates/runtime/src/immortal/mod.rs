//! Bridge's "immortal" mode — forgecode-style in-place compaction.
//!
//! When a conversation grows past the configured token budget, the eligible
//! head of the history is replaced in place with a single user message
//! containing a structured summary. No fake assistant acks, no journal
//! scaffolding, no checkpoint chaining, no LLM call (the summary is a pure
//! function of the messages being compacted).
//!
//! Triggers:
//!   1. **Top-of-turn** (`maybe_run_context_management`): every new user
//!      turn checks whether the history exceeds the budget; if so, runs
//!      one compaction pass before sending the prompt to the LLM.
//!   2. **Mid-rig-loop** (`PromptHook::on_completion_call`): the running
//!      history's approximate token count is checked before every LLM
//!      call inside rig's multi-turn loop. When it crosses the threshold
//!      the hook returns `Terminate { reason: "bridge:immortal" }`; the
//!      outer streaming loop catches the resulting `PromptCancelled`,
//!      runs a compaction, and re-invokes streaming with the spliced
//!      history.
//!
//! The mid-loop trigger is necessary because rig owns the loop and a
//! single user turn can blow the context window before bridge's
//! per-turn check runs again.

use bridge_core::agent::ImmortalConfig;
use bridge_core::BridgeError;
use rig::message::Message;
use tracing::info;

use crate::compaction;

mod extractor;
mod handoff;
mod render;
mod summary;
#[cfg(test)]
mod tests;
mod transformers;

pub use handoff::{
    evict_msg_count, extract_latest_reasoning, find_compaction_range,
    inject_reasoning_into_first_assistant, maybe_merge_summary_head, splice_summary,
    CompactionRange, HeadMergeStats,
};
pub use render::render as render_summary;
pub use summary::ContextSummary;

/// In-memory state tracking. Kept for telemetry compatibility (chain
/// counter shows up in `ChainStarted` / `ChainCompleted` events) — has
/// no semantic effect on compaction itself.
#[derive(Debug, Clone)]
pub struct ImmortalState {
    pub current_chain_index: u32,
}

/// Cheap probe — does the current history exceed the configured budget?
/// Returns the precise token count when it does, else `None`. Caller
/// (`maybe_run_context_management`) emits `ChainStarted` and runs
/// `execute_chain_handoff` when this returns `Some`.
pub fn chain_needed(history: &[Message], config: &ImmortalConfig) -> Option<ChainTrigger> {
    let budget = config.token_budget as usize;
    let fast = compaction::estimate_tokens_fast(history, budget);
    let precise = compaction::estimate_tokens(history);
    info!(
        history_len = history.len(),
        budget,
        fast_estimate = ?fast,
        precise_estimate = precise,
        triggered = precise > budget,
        "chain_needed"
    );
    if precise <= budget {
        return None;
    }
    Some(ChainTrigger {
        pre_chain_tokens: precise,
    })
}

/// Reason a handoff was scheduled, plus the measured pre-handoff token
/// count (surfaced in `ChainStarted` events for telemetry).
#[derive(Debug, Clone, Copy)]
pub struct ChainTrigger {
    pub pre_chain_tokens: usize,
}

/// Result of a successful compaction pass.
pub struct ChainHandoffResult {
    /// New history with the compacted range replaced by one summary
    /// user message. Caller installs this as bridge's working history
    /// and refreshes persisted state.
    pub new_history: Vec<Message>,
    /// The rendered markdown summary text. Surfaced in `ChainCompleted`
    /// events (for size telemetry) and as the body of the spliced user
    /// message.
    pub summary_text: String,
    /// Bumped chain counter (telemetry only).
    pub chain_index: u32,
    /// Number of messages collapsed into the single summary message.
    pub messages_compacted: usize,
    /// Number of messages preserved verbatim AFTER the splice point.
    /// Forgecode-style: messages BEFORE `start` are also preserved
    /// verbatim and are reflected in `new_history.len()` minus
    /// `1 + messages_after`.
    pub messages_after: usize,
    /// Pre-handoff token count (snapshot from the trigger).
    pub pre_chain_tokens: usize,
}

/// Execute one compaction pass.
///
/// Pure forgecode flow:
/// 1. `find_compaction_range` picks the `(start, end)` to compact —
///    anchored at the first assistant message, respecting tool atomicity.
/// 2. `extractor::extract` walks the range and produces a structured
///    `ContextSummary`.
/// 3. `transformers::apply_pipeline` dedupes consecutive user blocks,
///    trims consecutive same-resource ops, and strips the working dir
///    prefix from paths.
/// 4. `render::render` flattens the summary to markdown.
/// 5. `splice_summary` replaces `[start..=end]` in the original history
///    with a single user-text message holding that markdown.
/// 6. The most-recent reasoning block from the compacted range is
///    re-attached to the first surviving assistant message (extended
///    thinking continuity).
///
/// `journal_state` and `todos_snapshot` parameters are accepted for
/// backwards compatibility with the `context_mgmt` call site but are
/// not used — the new pipeline carries no scaffolding.
pub async fn execute_chain_handoff(
    history: &[Message],
    config: &ImmortalConfig,
    state: &ImmortalState,
    _journal_state: Option<&tools::journal::JournalState>,
    _todos_snapshot: Option<String>,
    trigger: ChainTrigger,
) -> Result<ChainHandoffResult, BridgeError> {
    let new_chain_index = state.current_chain_index + 1;
    let retention = config.retention_window as usize;
    let eviction_cap = evict_msg_count(history, config.eviction_window);

    let range = find_compaction_range(history, retention, eviction_cap).ok_or_else(|| {
        BridgeError::ProviderError(
            "compaction: no eligible range (retention covers everything or no assistant message)"
                .to_string(),
        )
    })?;

    let to_compact = &history[range.start..=range.end];
    let messages_compacted = to_compact.len();
    let reasoning = extract_latest_reasoning(to_compact);

    // Extract → transform → render. Pure functions, no LLM call.
    let summary = extractor::extract(to_compact);
    let working_dir = std::env::current_dir().ok();
    let summary = transformers::apply_pipeline(summary, working_dir.as_deref());
    let summary_text = render::render(&summary);

    let mut new_history = splice_summary(history.to_vec(), range, summary_text.clone());

    // Carry the extended-thinking chain forward.
    if let Some(reasoning) = reasoning {
        inject_reasoning_into_first_assistant(&mut new_history, range.start, reasoning);
    }

    // After splicing, the head may now hold many accumulated summary
    // frames from prior chains. Without compaction at the head they pile
    // up to ~5-10K tokens by chain 8 and start tripping the budget on
    // every LLM iteration (rapid-fire hook fires, no room for new work).
    // Merge OLDEST summaries when the cumulative head exceeds half the
    // budget, keeping only the most recent few intact.
    const KEEP_RECENT_SUMMARIES: usize = 2;
    if let Some(merge_stats) = maybe_merge_summary_head(
        &mut new_history,
        config.token_budget as usize,
        KEEP_RECENT_SUMMARIES,
    ) {
        info!(
            chain_index = new_chain_index,
            merged_frames = merge_stats.merged_count,
            head_tokens_before = merge_stats.before_tokens,
            head_tokens_after = merge_stats.after_tokens,
            "summary_head_merged"
        );
    }

    let messages_after = new_history.len().saturating_sub(range.start + 1);

    info!(
        chain_index = new_chain_index,
        messages_compacted,
        messages_after,
        summary_bytes = summary_text.len(),
        pre_chain_tokens = trigger.pre_chain_tokens,
        "compaction_handoff"
    );

    Ok(ChainHandoffResult {
        new_history,
        summary_text,
        chain_index: new_chain_index,
        messages_compacted,
        messages_after,
        pre_chain_tokens: trigger.pre_chain_tokens,
    })
}
