use bridge_core::agent::ImmortalConfig;
use bridge_core::BridgeError;
use rig::message::{Message, UserContent};
use tools::journal::{JournalEntry, JournalState};
use tracing::debug;

use crate::compaction;

/// Default checkpoint extraction prompt.
const DEFAULT_CHECKPOINT_PROMPT: &str = "\
You are extracting a structured checkpoint from a conversation that is being \
continued in a fresh context window. The user will not see this output — it will \
be injected into the new context to help the assistant continue seamlessly.

First, think through the entire conversation. Review the user's goals, your \
actions, tool outputs, file modifications, errors encountered, and any unresolved \
questions. Identify every piece of information needed to continue the work.

Then produce the checkpoint with these sections:

## Overall Goal
A single concise sentence describing the user's high-level objective.

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
left off and what to do next. Address the assistant directly.";

/// Verification prompt for the second phase of checkpoint extraction.
const VERIFICATION_PROMPT: &str = "\
Critically evaluate the checkpoint you just generated against the conversation \
history. Did you omit any important details — artifacts produced, user constraints, \
key facts about the working context, or task state? If anything important is \
missing, produce a FINAL improved checkpoint with the same section structure. \
Otherwise, repeat the exact same checkpoint.";

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
    /// Pre-chain token count.
    pub pre_chain_tokens: usize,
}

/// In-memory state tracking for an immortal conversation.
pub struct ImmortalState {
    /// Current chain index (0 = original, 1 = first chain, etc.)
    pub current_chain_index: u32,
}

/// Check if history exceeds the token budget and perform a chain handoff if so.
///
/// Returns `None` if history is under budget. Chain failure is surfaced as an
/// error so the caller can decide to continue with the full (oversized) history.
pub async fn maybe_chain(
    history: &[Message],
    config: &ImmortalConfig,
    state: &ImmortalState,
    journal_state: &JournalState,
) -> Result<Option<ChainHandoffResult>, BridgeError> {
    let budget = config.token_budget as usize;

    // Fast path: use byte-count heuristic
    let pre_tokens = match compaction::estimate_tokens_fast(history, budget) {
        Some(fast_est) if fast_est <= budget => return Ok(None),
        Some(_) => {
            let precise = compaction::estimate_tokens(history);
            if precise <= budget {
                return Ok(None);
            }
            precise
        }
        None => {
            let precise = compaction::estimate_tokens(history);
            if precise <= budget {
                return Ok(None);
            }
            precise
        }
    };

    debug!(
        pre_tokens = pre_tokens,
        budget = config.token_budget,
        chain_index = state.current_chain_index,
        "history exceeds token budget, initiating chain handoff"
    );

    let new_chain_index = state.current_chain_index + 1;

    // Find the carry-forward boundary: last N complete user turns
    let carry_start = find_carry_forward_boundary(history, config.carry_forward_turns as usize);

    // If we can't find a good boundary (entire history is one turn), bail
    if carry_start == 0 {
        debug!("cannot find carry-forward boundary, skipping chain");
        return Ok(None);
    }

    let carry_forward = &history[carry_start..];
    let to_checkpoint = &history[..carry_start];

    // Build checkpoint extraction prompt
    let preamble = config
        .checkpoint_prompt
        .as_deref()
        .unwrap_or(DEFAULT_CHECKPOINT_PROMPT);

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
        config: bridge_core::agent::AgentConfig::default(),
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

    // Build previous checkpoint context for continuity across chains
    let previous_checkpoint_context = if state.current_chain_index > 0 {
        let entries = journal_state.entries().await;
        let last_checkpoint = entries.iter().rev().find(|e| e.entry_type == "checkpoint");
        match last_checkpoint {
            Some(cp) => format!(
                "A previous checkpoint exists from chain {}. Integrate all still-relevant \
                 information, updating with recent events. Do not lose established \
                 constraints or knowledge.\n\n<previous_checkpoint>\n{}\n</previous_checkpoint>\n\n",
                cp.chain_index, cp.content
            ),
            None => String::new(),
        }
    } else {
        String::new()
    };

    // Serialize the history to checkpoint
    let serialized_history = compaction::serialize_history_for_summary(to_checkpoint);

    // Phase 1: Generate checkpoint
    let phase1_input = format!("{}{}", previous_checkpoint_context, serialized_history);
    let initial_checkpoint = checkpoint_agent
        .prompt_simple(&phase1_input)
        .await
        .map_err(|e| BridgeError::ProviderError(format!("checkpoint extraction error: {}", e)))?;

    // Phase 2: Verify and improve
    let phase2_input = format!(
        "CONVERSATION HISTORY:\n{}\n\nYOUR CHECKPOINT:\n{}\n\n{}",
        serialized_history, initial_checkpoint, VERIFICATION_PROMPT
    );
    let checkpoint_text = match checkpoint_agent.prompt_simple(&phase2_input).await {
        Ok(verified) => verified,
        Err(e) => {
            debug!(error = %e, "checkpoint verification failed, using initial checkpoint");
            initial_checkpoint
        }
    };

    // Build the new history
    let journal_entries = journal_state.entries().await;
    let new_history = build_chain_history(
        &journal_entries,
        &checkpoint_text,
        state.current_chain_index,
        carry_forward,
    );

    let carry_forward_count = carry_forward.len();

    Ok(Some(ChainHandoffResult {
        new_history,
        checkpoint_text,
        chain_index: new_chain_index,
        carry_forward_count,
        pre_chain_tokens: pre_tokens,
    }))
}

/// Find the index where carry-forward messages start.
///
/// Walks backward through history to find the start of the last N complete
/// user turns. A "user turn" starts at a User message containing text
/// (not a tool result).
pub fn find_carry_forward_boundary(history: &[Message], turns: usize) -> usize {
    if turns == 0 || history.is_empty() {
        return history.len();
    }

    let mut user_turns_found = 0;
    let mut boundary = history.len();

    for i in (0..history.len()).rev() {
        if is_user_text_message(&history[i]) {
            user_turns_found += 1;
            if user_turns_found >= turns {
                boundary = i;
                break;
            }
        }
    }

    // If we didn't find enough turns, carry forward everything (boundary = 0)
    if user_turns_found < turns {
        return 0;
    }

    boundary
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
    fn test_find_carry_forward_boundary_empty() {
        let history: Vec<Message> = vec![];
        assert_eq!(find_carry_forward_boundary(&history, 2), 0);
    }

    #[test]
    fn test_find_carry_forward_boundary_single_turn() {
        let history = vec![Message::user("Hello"), Message::assistant("Hi there!")];
        // Requesting 2 turns but only 1 exists → carry forward everything (0)
        assert_eq!(find_carry_forward_boundary(&history, 2), 0);
        // Requesting 1 turn → boundary at first user message
        assert_eq!(find_carry_forward_boundary(&history, 1), 0);
    }

    #[test]
    fn test_find_carry_forward_boundary_multiple_turns() {
        let history = vec![
            Message::user("First question"),
            Message::assistant("First answer"),
            Message::user("Second question"),
            Message::assistant("Second answer"),
            Message::user("Third question"),
            Message::assistant("Third answer"),
        ];
        // Carry forward last 2 turns → boundary at index 2 (start of "Second question")
        assert_eq!(find_carry_forward_boundary(&history, 2), 2);
        // Carry forward last 1 turn → boundary at index 4 (start of "Third question")
        assert_eq!(find_carry_forward_boundary(&history, 1), 4);
    }

    #[test]
    fn test_find_carry_forward_zero_turns() {
        let history = vec![Message::user("Hello"), Message::assistant("Hi")];
        // 0 turns → carry forward nothing (boundary at end)
        assert_eq!(find_carry_forward_boundary(&history, 0), 2);
    }

    #[test]
    fn test_format_journal_empty() {
        let entries: Vec<JournalEntry> = vec![];
        assert_eq!(format_journal(&entries), "");
    }

    #[test]
    fn test_format_journal_entries() {
        let entries = vec![
            JournalEntry {
                id: "1".to_string(),
                chain_index: 0,
                entry_type: "agent_note".to_string(),
                content: "User prefers PostgreSQL".to_string(),
                category: Some("decision".to_string()),
                timestamp: chrono::Utc::now(),
            },
            JournalEntry {
                id: "2".to_string(),
                chain_index: 1,
                entry_type: "checkpoint".to_string(),
                content: "Working on auth module refactor".to_string(),
                category: None,
                timestamp: chrono::Utc::now(),
            },
        ];
        let formatted = format_journal(&entries);
        assert!(formatted.contains("[decision] [chain 0] User prefers PostgreSQL"));
        assert!(formatted.contains("[checkpoint] [chain 1] Working on auth module refactor"));
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

        // Should have: journal_user + journal_ack + checkpoint_user + checkpoint_ack + 2 carry-forward
        assert_eq!(history.len(), 6);
    }

    #[test]
    fn test_build_chain_history_no_journal() {
        let entries: Vec<JournalEntry> = vec![];
        let carry_forward = vec![Message::user("Continue"), Message::assistant("OK")];

        let history = build_chain_history(&entries, "Checkpoint text", 0, &carry_forward);

        // No journal → checkpoint_user + checkpoint_ack + 2 carry-forward = 4
        assert_eq!(history.len(), 4);
    }
}
