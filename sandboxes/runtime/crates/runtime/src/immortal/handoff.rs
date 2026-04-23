//! History construction and carry-forward helpers for chain handoffs.

use rig::message::{Message, UserContent};
use tools::journal::JournalEntry;

use crate::compaction;

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
pub(super) fn build_chain_history(
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
