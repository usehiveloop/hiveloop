use std::time::Duration;

use bridge_core::agent::ImmortalConfig;
use bridge_core::BridgeError;
use rig::message::Message;
use tokio::time::timeout;
use tools::journal::{JournalEntry, JournalState};
use tracing::{debug, warn};

use crate::compaction;

mod handoff;
mod prompts;
#[cfg(test)]
mod tests;

pub use handoff::{find_token_bounded_carry_forward, format_journal};

use handoff::build_chain_history;
use prompts::{default_prompt_for_provider, VERIFICATION_PROMPT};

/// Result of a successful chain handoff.
pub struct ChainHandoffResult {
    /// New history to replace the current one.
    pub new_history: Vec<Message>,
    /// The raw checkpoint text extracted by the LLM.
    pub checkpoint_text: String,
    /// The new chain index.
    pub chain_index: u32,
    /// Number of messages carried forward verbatim.
    pub carry_forward_count: usize,
    /// Tokens the carried-forward messages consume.
    pub carry_forward_tokens: usize,
    /// Pre-chain token count.
    pub pre_chain_tokens: usize,
    /// Whether phase-2 verification ran.
    pub verified: bool,
}

/// In-memory state tracking for an immortal conversation.
pub struct ImmortalState {
    /// Current chain index (0 = original, 1 = first chain, etc.)
    pub current_chain_index: u32,
}

/// Result of a cheap "do we need to chain" probe. Returned by
/// [`chain_needed`] and consumed by [`execute_chain_handoff`].
pub struct ChainTrigger {
    pub pre_chain_tokens: usize,
}

/// Cheap probe: does the history exceed the budget right now?
///
/// Uses the fast byte-count estimator first and falls through to a precise
/// tiktoken count only near the boundary. Returns `None` if no handoff is
/// needed; `Some(trigger)` otherwise with the measured token count.
pub fn chain_needed(history: &[Message], config: &ImmortalConfig) -> Option<ChainTrigger> {
    let budget = config.token_budget as usize;

    let pre_tokens = match compaction::estimate_tokens_fast(history, budget) {
        Some(fast_est) if fast_est <= budget => return None,
        Some(fast_est) => {
            let precise = compaction::estimate_tokens(history);
            if precise <= budget {
                return None;
            }
            precise.max(fast_est)
        }
        None => {
            let precise = compaction::estimate_tokens(history);
            if precise <= budget {
                return None;
            }
            precise
        }
    };

    Some(ChainTrigger {
        pre_chain_tokens: pre_tokens,
    })
}

/// Execute a chain handoff. Caller is expected to have already emitted
/// `ChainStarted` so consumers can show progress while this runs.
///
/// On success returns the new history, checkpoint text, and metadata.
/// On checkpoint-LLM failure, returns `Err` — caller should continue with
/// the oversized history and emit `ChainFailed`.
pub async fn execute_chain_handoff(
    history: &[Message],
    config: &ImmortalConfig,
    state: &ImmortalState,
    journal_state: Option<&JournalState>,
    todos_snapshot: Option<String>,
    trigger: ChainTrigger,
) -> Result<ChainHandoffResult, BridgeError> {
    debug!(
        pre_tokens = trigger.pre_chain_tokens,
        budget = config.token_budget,
        chain_index = state.current_chain_index,
        "running chain handoff"
    );

    let new_chain_index = state.current_chain_index + 1;

    // Token-budget-based carry-forward: start from the latest turn-boundary
    // and accept whole user-text turns so long as they fit the budget cap.
    let budget = config.token_budget as usize;
    let carry_cap = ((budget as f32) * config.carry_forward_budget_fraction).max(0.0) as usize;
    let carry_cap = carry_cap.max(256); // never less than ~1 user message

    let (carry_start, carry_tokens) =
        find_token_bounded_carry_forward(history, config.carry_forward_turns as usize, carry_cap);

    if carry_start == history.len() {
        // Nothing to carry forward — unusual but possible with 0 turns configured.
        return Err(BridgeError::ProviderError(
            "chain handoff: no carry-forward boundary found".to_string(),
        ));
    }

    let carry_forward = &history[carry_start..];
    let to_checkpoint = &history[..carry_start];

    // Build checkpoint extraction prompt. An explicit `checkpoint_prompt` on
    // the agent wins; otherwise we pick a provider-aware default — Gemini
    // models get a stricter, XML-delimited template (proven to keep checkpoint
    // size flat across chains instead of climbing).
    let preamble = config
        .checkpoint_prompt
        .as_deref()
        .unwrap_or_else(|| default_prompt_for_provider(&config.checkpoint_provider));

    let summarizer_def = bridge_core::agent::AgentDefinition {
        id: String::new(),
        name: String::new(),
        description: None,
        system_prompt: preamble.to_string(),
        provider: config.checkpoint_provider.clone(),
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![],
        config: bridge_core::agent::AgentConfig {
            max_tokens: Some(config.checkpoint_max_tokens),
            ..Default::default()
        },
        subagents: vec![],
        permissions: std::collections::HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: None,
        updated_at: None,
    };

    let checkpoint_agent = llm::providers::create_agent(
        &config.checkpoint_provider,
        vec![],
        preamble,
        &summarizer_def,
    )?;

    // Build previous checkpoint context: integrate only the last N prior
    // checkpoints so the prompt doesn't grow unboundedly across chains.
    // When journal is unavailable, no prior checkpoints are stored; the
    // context is empty for every rotation and the todos snapshot below
    // carries the cross-chain state.
    let previous_checkpoint_context = match journal_state {
        Some(js) if state.current_chain_index > 0 => {
            let max_n = config.max_previous_checkpoints.max(1) as usize;
            let entries = js.committed_entries().await;
            let recent_checkpoints: Vec<&JournalEntry> = entries
                .iter()
                .rev()
                .filter(|e| e.entry_type == "checkpoint")
                .take(max_n)
                .collect();

            if recent_checkpoints.is_empty() {
                String::new()
            } else {
                let mut buf = String::from(
                    "Previous checkpoint(s) exist. Integrate all still-relevant \
                     information, updating with recent events. Do not lose established \
                     constraints or knowledge, BUT prune items fully superseded by later \
                     turns.\n\n",
                );
                // Iterate newest-first then render oldest-first for chronological reading.
                for cp in recent_checkpoints.iter().rev() {
                    buf.push_str(&format!(
                        "<previous_checkpoint chain={}>\n{}\n</previous_checkpoint>\n\n",
                        cp.chain_index, cp.content
                    ));
                }
                buf
            }
        }
        _ => String::new(),
    };

    // Serialize the history to checkpoint
    let serialized_history = compaction::serialize_history_for_summary(to_checkpoint);

    let per_call_timeout = Duration::from_secs(config.checkpoint_timeout_secs as u64);

    // Phase 1: Generate checkpoint
    let phase1_input = format!("{}{}", previous_checkpoint_context, serialized_history);
    let phase1_fut = checkpoint_agent.prompt_simple(&phase1_input);
    let initial_checkpoint = timeout(per_call_timeout, phase1_fut)
        .await
        .map_err(|_| {
            BridgeError::ProviderError(format!(
                "checkpoint phase-1 timed out after {}s",
                config.checkpoint_timeout_secs
            ))
        })?
        .map_err(|e| BridgeError::ProviderError(format!("checkpoint phase-1 error: {}", e)))?;

    // Phase 2: verification is opt-in. Default false to avoid doubling cost.
    let (checkpoint_text, verified) = if config.verify_checkpoint {
        let phase2_input = format!(
            "CONVERSATION HISTORY:\n{}\n\nYOUR CHECKPOINT:\n{}\n\n{}",
            serialized_history, initial_checkpoint, VERIFICATION_PROMPT
        );
        let phase2_fut = checkpoint_agent.prompt_simple(&phase2_input);
        match timeout(per_call_timeout, phase2_fut).await {
            Ok(Ok(verified_text)) => (verified_text, true),
            Ok(Err(e)) => {
                warn!(error = %e, "checkpoint phase-2 failed, using phase-1 output");
                (initial_checkpoint, false)
            }
            Err(_) => {
                warn!(
                    timeout_s = config.checkpoint_timeout_secs,
                    "checkpoint phase-2 timed out, using phase-1 output"
                );
                (initial_checkpoint, false)
            }
        }
    } else {
        (initial_checkpoint, false)
    };

    // Build the new history. Journal entries are injected only when the
    // journal is available; otherwise the todos_snapshot takes over as the
    // cross-chain memory carrier.
    let journal_entries: Vec<JournalEntry> = if let Some(js) = journal_state {
        js.committed_entries().await
    } else {
        Vec::new()
    };
    let new_history = build_chain_history(
        &journal_entries,
        todos_snapshot.as_deref(),
        &checkpoint_text,
        state.current_chain_index,
        carry_forward,
    );

    let carry_forward_count = carry_forward.len();

    Ok(ChainHandoffResult {
        new_history,
        checkpoint_text,
        chain_index: new_chain_index,
        carry_forward_count,
        carry_forward_tokens: carry_tokens,
        pre_chain_tokens: trigger.pre_chain_tokens,
        verified,
    })
}

/// Back-compat wrapper used by older call sites / tests. Prefer the
/// [`chain_needed`] + [`execute_chain_handoff`] split for new code so the
/// caller can emit a ChainStarted event before the expensive extraction.
#[cfg(test)]
pub async fn maybe_chain(
    history: &[Message],
    config: &ImmortalConfig,
    state: &ImmortalState,
    journal_state: &JournalState,
) -> Result<Option<ChainHandoffResult>, BridgeError> {
    let Some(trigger) = chain_needed(history, config) else {
        return Ok(None);
    };
    execute_chain_handoff(history, config, state, Some(journal_state), None, trigger)
        .await
        .map(Some)
}
