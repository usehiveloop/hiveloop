use std::time::Duration;

use bridge_core::agent::ImmortalConfig;
use bridge_core::BridgeError;
use rig::message::{Message, UserContent};
use tokio::time::timeout;
use tools::journal::{JournalEntry, JournalState};
use tracing::{debug, warn};

use crate::compaction;

/// Default checkpoint extraction prompt.
const DEFAULT_CHECKPOINT_PROMPT: &str = "\
You are extracting a structured checkpoint from a conversation that is being \
continued in a fresh context window. The user will not see this output — it will \
be injected into the new context to help the assistant continue seamlessly.

First, think through the entire conversation. Review the user's ORIGINAL high-level \
goal (from the earliest user turns), the actions the assistant took across every \
turn, tool outputs, file modifications, errors encountered, and any unresolved \
questions. Identify every piece of information needed to continue the work. The \
Overall Goal MUST reflect the conversation-wide objective — not just the most \
recent topic.

Then produce the checkpoint with these sections:

## Overall Goal
A single concise sentence describing the user's conversation-wide high-level \
objective (derived from the earliest user turns, not just the latest topic).

## Active Constraints
Explicit constraints, preferences, or rules established by the user or discovered \
during the conversation. Examples: brand voice guidelines, coding style preferences, \
budget limits, target audience, framework choices, regulatory requirements.

## Key Knowledge
Crucial facts and discoveries about the working context. This could be technical \
details (build commands, API endpoints, database schemas), domain knowledge \
(market segments, competitor analysis, audience demographics), or environmental \
facts (team structure, timelines, tool access) — anything the assistant needs to \
know to continue effectively.

## Work Trail
Key artifacts that were produced, modified, or reviewed, and WHY. Track the \
evolution of significant outputs and their rationale. Examples:
- For code: `src/auth.rs`: Refactored from JWT to session tokens for compliance.
- For content: Campaign brief v2: Revised targeting from 18-24 to 25-34 based on analytics.
- For research: Competitive analysis: Added 3 new entrants identified in Q3 reports.

## Key Decisions
Important decisions made during the conversation with brief rationale.

## Task State
The current plan with completion markers:
1. [DONE] Research phase
2. [IN PROGRESS] Draft deliverables  <-- CURRENT FOCUS
3. [TODO] Review and finalize

## Transition Context
A brief paragraph (2-4 sentences) telling the assistant exactly where things \
left off and what to do next. Address the assistant directly.

Keep the checkpoint focused and dense. Aim for under 1200 tokens total — prune \
details that are fully superseded by later decisions.";

/// Gemini-tuned checkpoint prompt. Google's prompt-design docs recommend XML
/// delimiters, critical rules first, explicit length caps per section, and
/// active-verb pruning directives. Without those, Gemini 2.5 Flash accumulates
/// content monotonically across chains (observed going 4k → 7k → 15k bytes over
/// 3 chains with the default prompt). With this prompt it stays roughly flat.
/// Selected automatically by [`default_prompt_for_provider`] when the model
/// string contains "gemini".
const GEMINI_CHECKPOINT_PROMPT: &str = "\
<role>
You are a conversation-checkpoint extractor. Your only job: compress a completed \
portion of an LLM conversation into a DENSE structured checkpoint that another \
assistant will read in a fresh context window to continue the work seamlessly. \
The user never sees your output — it is prompt-context injection only.
</role>

<hard_rules>
1. Your entire output MUST be under 900 tokens. Longer output is a failure.
2. Produce EXACTLY the 7 sections below, in order, using the exact markdown \
headings shown. No preamble, no closing remarks. Start your response DIRECTLY \
with the line \"## Overall Goal\".
3. When a <previous_checkpoint> is supplied, you MUST actively PRUNE it. DELETE \
items fully superseded by later decisions. DELETE items marked DONE more than \
one chain ago. MERGE near-duplicate bullets. Do NOT preserve content \"for \
completeness\" — the whole point of a checkpoint is compression, not accumulation.
4. The Overall Goal is the user's CONVERSATION-WIDE objective from the earliest \
user turns. Never narrow it to the most recent topic, even if recent turns focus \
on one sub-area.
5. Every bullet is a concrete specific fact: library names with parameters, \
numeric constants, file paths, decisions with reasons. Forbidden words in bullets: \
\"discussed\", \"covered\", \"explored\", \"looked at\", \"considered\". Write the \
conclusion, not the activity.
6. If a section has no relevant content write \"- (none)\". Do not invent content.
</hard_rules>

<output_template>
## Overall Goal
One sentence, max 30 words — the conversation-wide objective.

## Active Constraints
Bullets. Max 8 items, each ≤20 words. Rules/preferences/limits still binding.

## Key Knowledge
Bullets. Max 12 items, each ≤25 words. Concrete technical facts (libraries + \
versions + parameters, schema details, endpoints, file paths, numeric constants). \
No generalities.

## Work Trail
Bullets. Max 8 items, each in the form \"`<artifact>`: <what changed + why>\". \
Drop items fully superseded.

## Key Decisions
Bullets. Max 8 items, each ≤25 words. Named decisions + one-phrase rationale.

## Task State
Numbered list. Each line tagged [DONE] / [IN PROGRESS] / [TODO]. Mark exactly one \
[IN PROGRESS] with \"<-- CURRENT FOCUS\".

## Transition Context
One paragraph, 2-3 sentences, ≤70 words. Address the assistant directly: where \
things stopped, what to do next.
</output_template>

<pruning_discipline>
When previous_checkpoint(s) are present:
- KEEP: decisions still governing forward work; facts still needed; constraints \
not yet met.
- DROP: tasks marked DONE more than one chain ago; details superseded by later \
decisions; duplicated bullets; narrative framing; prior Transition Context \
paragraphs.
- MERGE: near-duplicate bullets into one tighter bullet.
</pruning_discipline>

Read the entire conversation and any previous checkpoints below, then produce \
your checkpoint. Remember: start with \"## Overall Goal\" on the first line.";

/// Pick the built-in checkpoint prompt that best fits the configured
/// summarizer. Gemini-family models benefit substantially from stricter
/// structure + pruning directives (see [`GEMINI_CHECKPOINT_PROMPT`]); every
/// other provider falls through to the generic default.
fn default_prompt_for_provider(provider: &bridge_core::provider::ProviderConfig) -> &'static str {
    use bridge_core::provider::ProviderType;
    let is_gemini = matches!(provider.provider_type, ProviderType::Google)
        || provider.model.to_ascii_lowercase().contains("gemini");
    if is_gemini {
        GEMINI_CHECKPOINT_PROMPT
    } else {
        DEFAULT_CHECKPOINT_PROMPT
    }
}

/// Verification prompt for the (optional) second phase of checkpoint extraction.
const VERIFICATION_PROMPT: &str = "\
Critically evaluate the checkpoint you just generated against the conversation \
history. Did you omit any important details — artifacts produced, user constraints, \
key facts about the working context, or task state? Is the Overall Goal the \
conversation-wide objective, or did you narrow it to the most recent topic? \
If anything important is missing, produce a FINAL improved checkpoint with the \
same section structure. Otherwise, repeat the exact same checkpoint.";

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
    journal_state: &JournalState,
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
    let previous_checkpoint_context = if state.current_chain_index > 0 {
        let max_n = config.max_previous_checkpoints.max(1) as usize;
        let entries = journal_state.committed_entries().await;
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
    } else {
        String::new()
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

    // Build the new history
    let journal_entries = journal_state.committed_entries().await;
    let new_history = build_chain_history(
        &journal_entries,
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
    execute_chain_handoff(history, config, state, journal_state, trigger)
        .await
        .map(Some)
}

/// Find the carry-forward start index, respecting BOTH:
///   - a hard ceiling on the number of user-text turns carried (`max_turns`)
///   - a token-budget cap on the carried tail (`token_cap`)
///
/// Walks backward from the end of history. At each user-text message we stop
/// and check whether including this turn would exceed the token cap. If yes
/// and we already have at least one turn, we stop at the previous boundary.
/// Returns `(start_index, tokens_in_tail)`.
pub fn find_token_bounded_carry_forward(
    history: &[Message],
    max_turns: usize,
    token_cap: usize,
) -> (usize, usize) {
    if history.is_empty() || max_turns == 0 {
        return (history.len(), 0);
    }

    // Walk backward and find successive user-text boundaries. Stop when
    // adding the next turn would break the token cap or exceed max_turns.
    let mut best_start = history.len();
    let mut best_tokens = 0usize;
    let mut user_turns_found = 0usize;

    for i in (0..history.len()).rev() {
        if is_user_text_message(&history[i]) {
            // Tokens of the tail if we start here.
            let tail_tokens = compaction::estimate_tokens(&history[i..]);
            if tail_tokens > token_cap && user_turns_found > 0 {
                // Don't cross this boundary — keep the previous `best_start`.
                break;
            }
            best_start = i;
            best_tokens = tail_tokens;
            user_turns_found += 1;
            if user_turns_found >= max_turns {
                break;
            }
        }
    }

    (best_start, best_tokens)
}

/// Build the new history for a chain link.
fn build_chain_history(
    journal_entries: &[JournalEntry],
    checkpoint_text: &str,
    previous_chain_index: u32,
    carry_forward: &[Message],
) -> Vec<Message> {
    let mut new_history = Vec::new();

    // 1. Inject journal if non-empty
    if !journal_entries.is_empty() {
        let journal_text = format_journal(journal_entries);
        new_history.push(Message::user(format!(
            "[Conversation Journal — {} entries across {} chain(s)]\n\n{}",
            journal_entries.len(),
            previous_chain_index + 1,
            journal_text
        )));
        new_history.push(Message::assistant(
            "I've reviewed the journal entries and have full context. Ready to continue.",
        ));
    }

    // 2. Inject checkpoint
    new_history.push(Message::user(format!(
        "[Context Checkpoint — chain {}]\n\n{}",
        previous_chain_index, checkpoint_text
    )));
    new_history.push(Message::assistant(
        "Understood. I have the checkpoint context and will continue seamlessly.",
    ));

    // 3. Append carried-forward messages verbatim
    new_history.extend_from_slice(carry_forward);

    new_history
}

/// Format journal entries as readable text for LLM context injection.
pub fn format_journal(entries: &[JournalEntry]) -> String {
    let mut output = String::new();
    for entry in entries {
        let category = entry
            .category
            .as_deref()
            .unwrap_or(entry.entry_type.as_str());
        output.push_str(&format!(
            "- [{}] [chain {}] {}\n",
            category, entry.chain_index, entry.content
        ));
    }
    output
}

/// Check if a rig message is a user message containing actual text (not a tool result).
fn is_user_text_message(msg: &Message) -> bool {
    match msg {
        Message::User { content } => content.iter().any(|c| matches!(c, UserContent::Text(_))),
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_find_carry_forward_empty() {
        let history: Vec<Message> = vec![];
        let (start, toks) = find_token_bounded_carry_forward(&history, 2, 10_000);
        assert_eq!(start, 0);
        assert_eq!(toks, 0);
    }

    #[test]
    fn test_find_carry_forward_respects_turn_ceiling() {
        let history = vec![
            Message::user("a"),
            Message::assistant("b"),
            Message::user("c"),
            Message::assistant("d"),
            Message::user("e"),
            Message::assistant("f"),
        ];
        // 1 user turn → start at "e"
        let (start, _) = find_token_bounded_carry_forward(&history, 1, 10_000);
        assert_eq!(start, 4);
        // 2 user turns → start at "c"
        let (start, _) = find_token_bounded_carry_forward(&history, 2, 10_000);
        assert_eq!(start, 2);
        // 10 requested but only 3 exist → all of it
        let (start, _) = find_token_bounded_carry_forward(&history, 10, 10_000);
        assert_eq!(start, 0);
    }

    #[test]
    fn test_find_carry_forward_respects_token_cap() {
        // Each message is roughly "x" (tiny). 2 turns would be ~5 messages.
        let history = vec![
            Message::user("a"),
            Message::assistant("b"),
            Message::user("c"),
            Message::assistant("d"),
            Message::user("e"),
            Message::assistant("f"),
        ];
        // Cap far below any turn's tokens → should still take at least 1 turn.
        let (start, _) = find_token_bounded_carry_forward(&history, 5, 1);
        assert!(start >= 4, "token cap must not evict the last user turn");
    }

    #[test]
    fn test_format_journal_entries() {
        let entries = vec![JournalEntry {
            id: "1".to_string(),
            chain_index: 0,
            entry_type: "agent_note".to_string(),
            content: "decision".to_string(),
            category: Some("decision".to_string()),
            timestamp: chrono::Utc::now(),
        }];
        let formatted = format_journal(&entries);
        assert!(formatted.contains("[decision] [chain 0]"));
    }

    #[test]
    fn test_build_chain_history_with_journal_and_checkpoint() {
        let entries = vec![JournalEntry {
            id: "1".to_string(),
            chain_index: 0,
            entry_type: "agent_note".to_string(),
            content: "Important decision".to_string(),
            category: Some("decision".to_string()),
            timestamp: chrono::Utc::now(),
        }];

        let carry_forward = vec![
            Message::user("Continue working on X"),
            Message::assistant("Sure, I'll continue."),
        ];

        let history = build_chain_history(&entries, "Checkpoint text here", 0, &carry_forward);

        // journal_user + journal_ack + checkpoint_user + checkpoint_ack + 2 carry-forward
        assert_eq!(history.len(), 6);
    }

    #[test]
    fn test_build_chain_history_no_journal() {
        let entries: Vec<JournalEntry> = vec![];
        let carry_forward = vec![Message::user("Continue"), Message::assistant("OK")];

        let history = build_chain_history(&entries, "Checkpoint text", 0, &carry_forward);
        assert_eq!(history.len(), 4);
    }
}
