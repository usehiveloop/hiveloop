//! History compaction primitives — sequence selection + in-place splice.
//!
//! Mirrors forgecode 1:1:
//!   - `find_sequence_preserving_last_n` chooses a `(start, end)` inclusive
//!     range to compact, anchored at the first assistant message and
//!     respecting tool-call/result atomicity.
//!   - `to_fixed` converts an "evict X% of tokens" budget into a concrete
//!     message-count cap.
//!   - `splice_summary` replaces the chosen range in place with ONE user
//!     message holding the rendered summary frame. No fake assistant acks.
//!     No journal scaffolding. The messages before `start` and after `end`
//!     are kept verbatim.
//!
//! Token usage and the most-recent reasoning chain are carried forward by
//! the caller (`mod.rs::execute_chain_handoff`).

use rig::message::{AssistantContent, Message, UserContent};
use rig::OneOrMany;

use super::render::{FOOTER, HEADER, SUMMARY_BODY_CLOSE, SUMMARY_BODY_OPEN};
use crate::compaction;

/// Result of choosing a contiguous compaction range.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct CompactionRange {
    /// Inclusive lower bound. Always points at the first assistant message
    /// in the eligible region — earlier messages (system + initial user
    /// prompt) are preserved verbatim.
    pub start: usize,
    /// Inclusive upper bound. Adjusted to never split a tool_call from
    /// its tool_result.
    pub end: usize,
}

/// Convert "evict X fraction of total tokens" into a message-count cap.
/// Walks forward from index 0 (skipping system messages), accumulating
/// approximate per-message token counts until the budget is exhausted.
/// Returns the message index where the budget ran out — that's the
/// maximum number of messages eligible for eviction.
///
/// Mirrors forgecode's `CompactionStrategy::Evict.to_fixed`.
pub fn evict_msg_count(history: &[Message], fraction: f64) -> usize {
    if history.is_empty() || fraction <= 0.0 {
        return 0;
    }
    let fraction = fraction.min(1.0);
    let total_tokens = compaction::estimate_tokens(history);
    let mut budget = (fraction * total_tokens as f64).ceil() as usize;
    if budget == 0 {
        return 0;
    }
    for (idx, msg) in history.iter().enumerate() {
        let cost = compaction::estimate_tokens(std::slice::from_ref(msg));
        budget = budget.saturating_sub(cost);
        if budget == 0 {
            return idx;
        }
    }
    history.len().saturating_sub(1)
}

/// Pick the eligible compaction range. Mirrors forgecode's
/// `find_sequence_preserving_last_n` algorithm exactly:
///
/// 1. Start = index of the first assistant message (so system + early
///    user messages are always preserved).
/// 2. End   = `len - retention - 1` (preserves the last `retention`
///    messages verbatim).
/// 3. If `end` would split a tool_call/tool_result pair, walk it back to
///    the previous safe boundary.
///
/// Returns `None` when there's nothing safe to compact (no assistant
/// messages yet, retention covers everything, or the safe-boundary walk
/// pushes `end` below `start`).
pub fn find_compaction_range(
    history: &[Message],
    retention: usize,
    eviction_cap: usize,
) -> Option<CompactionRange> {
    if history.is_empty() {
        return None;
    }
    let len = history.len();
    let start = history
        .iter()
        .position(|m| matches!(m, Message::Assistant { .. }))?;
    if start >= len {
        return None;
    }
    // Cap eviction to whichever is stricter: retention window OR
    // eviction-fraction-derived message count.
    let max_compactable = std::cmp::min(
        len.saturating_sub(retention),
        eviction_cap.saturating_add(1),
    );
    if max_compactable == 0 || max_compactable <= start {
        return None;
    }
    let mut end = max_compactable.saturating_sub(1);
    // Don't split tool_call ↔ tool_result pairs.
    end = walk_back_to_safe_boundary(history, start, end)?;
    if end < start {
        return None;
    }
    Some(CompactionRange { start, end })
}

/// Walk `end` backward until cutting between `messages[end]` and
/// `messages[end+1]` doesn't split a tool batch.
///
/// Unsafe cuts:
///   - `messages[end]` is an assistant with pending tool_calls and the
///     next message would be the matching tool_result.
///   - `messages[end]` is a tool_result and `messages[end+1]` is also a
///     tool_result (mid-batch).
fn walk_back_to_safe_boundary(history: &[Message], start: usize, mut end: usize) -> Option<usize> {
    loop {
        if end < start {
            return None;
        }
        let here = history.get(end)?;
        // Case A: `end` is an assistant with at least one tool_call.
        // Cutting here orphans the call — its result lives in `end+1`.
        // Walk back past the tool_call.
        if assistant_has_tool_call(here) {
            if end == 0 {
                return None;
            }
            end -= 1;
            continue;
        }
        // Case B: `end` is a tool_result and `end+1` is also a
        // tool_result. We're mid-batch. Walk back through the whole
        // batch.
        if user_is_tool_result(here) && history.get(end + 1).is_some_and(user_is_tool_result) {
            // Walk back through the run of tool_results to the assistant
            // that issued them, then one more to land BEFORE the
            // assistant tool_call message.
            while end > start && user_is_tool_result(history.get(end)?) {
                end -= 1;
            }
            // `end` now points at the assistant tool_call — back one
            // more so we cut BEFORE the call.
            if end == 0 {
                return None;
            }
            end -= 1;
            continue;
        }
        return Some(end);
    }
}

fn assistant_has_tool_call(msg: &Message) -> bool {
    if let Message::Assistant { content, .. } = msg {
        content
            .iter()
            .any(|c| matches!(c, AssistantContent::ToolCall(_)))
    } else {
        false
    }
}

fn user_is_tool_result(msg: &Message) -> bool {
    if let Message::User { content } = msg {
        content
            .iter()
            .any(|c| matches!(c, rig::message::UserContent::ToolResult(_)))
    } else {
        false
    }
}

/// Replace the chosen range in place with one user message containing the
/// rendered summary frame. Returns the new history.
///
/// Forgecode-equivalent: `splice(start..=end, summary_user_message)`. No
/// scaffolding pairs, no fake assistant acks. The messages before `start`
/// and after `end` are kept verbatim — alternation is naturally maintained
/// because `start` is always an assistant message (so the message before
/// is non-assistant), the spliced summary is user, and `messages[end+1]`
/// is naturally the assistant turn that follows.
pub fn splice_summary(
    history: Vec<Message>,
    range: CompactionRange,
    summary_text: String,
) -> Vec<Message> {
    let mut out: Vec<Message> = Vec::with_capacity(history.len() - (range.end - range.start));
    let mut iter = history.into_iter().enumerate().peekable();
    while let Some((idx, msg)) = iter.next() {
        if idx < range.start {
            out.push(msg);
            continue;
        }
        if idx == range.start {
            // Insert the summary as a single user-text message.
            out.push(Message::user(summary_text.clone()));
            // Drop everything else in the range.
            while let Some((next_idx, _)) = iter.peek() {
                if *next_idx <= range.end {
                    iter.next();
                } else {
                    break;
                }
            }
            continue;
        }
        out.push(msg);
    }
    out
}

/// Locate the most-recent non-empty reasoning block within a slice.
/// Walks backward; returns the first one found. Used to carry the
/// extended-thinking chain across a compaction so the next assistant
/// turn doesn't lose its reasoning context.
pub fn extract_latest_reasoning(messages: &[Message]) -> Option<rig::message::Reasoning> {
    for msg in messages.iter().rev() {
        if let Message::Assistant { content, .. } = msg {
            for part in content.iter() {
                if let AssistantContent::Reasoning(r) = part {
                    if !r.content.is_empty() {
                        return Some(r.clone());
                    }
                }
            }
        }
    }
    None
}

/// Inject `reasoning` into the first assistant message after the spliced
/// summary, but only if that message has no reasoning of its own. Mirrors
/// forgecode's reasoning-preservation step. This prevents extended-thinking
/// chains from breaking when a compaction lands between the LLM's reasoning
/// and the next call that depends on it.
pub fn inject_reasoning_into_first_assistant(
    history: &mut [Message],
    splice_index: usize,
    reasoning: rig::message::Reasoning,
) {
    for msg in history.iter_mut().skip(splice_index + 1) {
        if let Message::Assistant { content, .. } = msg {
            // Only inject if no reasoning is already present.
            let has_reasoning = content
                .iter()
                .any(|c| matches!(c, AssistantContent::Reasoning(_)));
            if has_reasoning {
                return;
            }
            let mut parts: Vec<AssistantContent> = content.iter().cloned().collect();
            parts.insert(0, AssistantContent::Reasoning(reasoning));
            if let Ok(new_content) = OneOrMany::many(parts) {
                *content = new_content;
            }
            return;
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Summary head merging — addresses the accumulating-summaries problem.
//
// Every successful chain handoff splices ONE summary user-message in place.
// `find_compaction_range` anchors at the first `Message::Assistant`, so any
// user-text messages BEFORE that anchor live there forever — including all
// prior summary frames. After 6-8 chains the head fills with stacked
// summaries that exceed `token_budget` by themselves, leaving no room for
// new work.
//
// `maybe_merge_summary_head` detects this and condenses the OLDEST summaries
// into a single combined frame, deterministically (no LLM call). Mirrors
// forgecode's behavior of letting subsequent compactions absorb prior
// summaries rather than letting them pile up.
// ─────────────────────────────────────────────────────────────────────────────

/// Result returned when a head-merge actually fired.
#[derive(Debug, Clone, Copy)]
pub struct HeadMergeStats {
    /// Number of frames merged into one.
    pub merged_count: usize,
    /// Approximate tokens in the merged frames before merging.
    pub before_tokens: usize,
    /// Approximate tokens after merging.
    pub after_tokens: usize,
}

/// True if `msg` looks like a summary-frame user-text message bridge wrote
/// during a prior chain handoff. Detection is by header-prefix match on the
/// first text part — bridge guarantees every spliced summary starts with
/// `HEADER` and is wrapped in a single `Message::user(text)`.
pub fn is_summary_frame(msg: &Message) -> bool {
    if let Message::User { content } = msg {
        for part in content.iter() {
            if let UserContent::Text(t) = part {
                return t.text.starts_with(HEADER);
            }
        }
    }
    false
}

/// If accumulated summary frames at the head of `history` exceed
/// `budget_tokens / 2` cumulatively, merge ALL but the most recent
/// `keep_recent` frames into a single combined frame.
///
/// Returns `Some(stats)` when a merge fired, `None` when the head is fine.
///
/// Algorithm:
/// 1. Walk forward from index 0, collecting indices of summary-frame
///    messages. Stop on the first non-user-text or non-summary message
///    (the head is contiguous: summary frames always live BEFORE the
///    first Assistant message because every chain inserts them at the
///    splice point).
/// 2. If fewer than `keep_recent + 2` summaries exist, no work to do.
/// 3. Sum tokens; bail if under the threshold.
/// 4. Take the OLDEST `summaries.len() - keep_recent` and concatenate
///    their `## Summary` body sections into one frame. Replace those
///    indices in `history` with a single new merged user message.
pub fn maybe_merge_summary_head(
    history: &mut Vec<Message>,
    budget_tokens: usize,
    keep_recent: usize,
) -> Option<HeadMergeStats> {
    let summary_indices: Vec<usize> = head_summary_indices(history);
    if summary_indices.len() <= keep_recent + 1 {
        return None;
    }

    let total_tokens: usize = summary_indices
        .iter()
        .map(|&i| compaction::estimate_tokens(std::slice::from_ref(&history[i])))
        .sum();
    let threshold = budget_tokens / 2;
    if total_tokens < threshold {
        return None;
    }

    let to_merge_count = summary_indices.len() - keep_recent;
    let merge_indices = &summary_indices[..to_merge_count];

    let bodies: Vec<String> = merge_indices
        .iter()
        .filter_map(|&i| extract_summary_body(&history[i]))
        .collect();
    if bodies.is_empty() {
        return None;
    }

    let merged = render_merged_frame(&bodies);
    let after_tokens =
        compaction::estimate_tokens(std::slice::from_ref(&Message::user(merged.clone())));

    let first = merge_indices[0];
    let last = *merge_indices.last().unwrap();
    history.splice(first..=last, std::iter::once(Message::user(merged)));

    Some(HeadMergeStats {
        merged_count: to_merge_count,
        before_tokens: total_tokens,
        after_tokens,
    })
}

/// Walk forward from index 0 collecting indices of user-text messages that
/// look like summary frames. Stops at the first non-summary message.
///
/// We do NOT walk into the body of the conversation — once a non-summary
/// (system, assistant, tool result, regular user message) appears, the
/// head is over. Summary frames spliced in subsequent chains land at the
/// new "first assistant" anchor, which by definition pushes them in front
/// of the assistant. Result: every summary frame ever spliced ends up at
/// the head, in chronological order.
fn head_summary_indices(history: &[Message]) -> Vec<usize> {
    let mut out = Vec::new();
    for (idx, msg) in history.iter().enumerate() {
        if is_summary_frame(msg) {
            out.push(idx);
            continue;
        }
        // Allow the original user-prompt message at index 0 to NOT be a
        // summary — it's the conversation seed and lives forever before
        // the summaries.
        if matches!(msg, Message::User { .. }) && idx == 0 {
            continue;
        }
        break;
    }
    out
}

/// Pull the inner `## Summary\n\n…\n\n---\n\n` body out of a summary frame
/// user-message. Returns `None` if the message doesn't have the expected
/// shape (defensive — `is_summary_frame` already gates the caller).
fn extract_summary_body(msg: &Message) -> Option<String> {
    let Message::User { content } = msg else {
        return None;
    };
    let text = content.iter().find_map(|c| match c {
        UserContent::Text(t) => Some(t.text.clone()),
        _ => None,
    })?;
    let body_start = text.find(SUMMARY_BODY_OPEN)?;
    let after_open = &text[body_start + SUMMARY_BODY_OPEN.len()..];
    let close = after_open.find(&format!("\n\n{}", SUMMARY_BODY_CLOSE))?;
    Some(after_open[..close].to_string())
}

/// Concatenate body snippets into one full summary frame with the same
/// header/footer scaffold as a single-chain frame. The receiving agent
/// can't tell the difference between "one frame summarising a wide range"
/// and "merged frame combining several prior chains".
fn render_merged_frame(bodies: &[String]) -> String {
    let mut out = String::new();
    out.push_str(HEADER);
    out.push_str(SUMMARY_BODY_OPEN);
    for (i, body) in bodies.iter().enumerate() {
        if i > 0 {
            out.push_str("\n\n");
        }
        out.push_str(body);
    }
    out.push_str("\n\n");
    out.push_str(SUMMARY_BODY_CLOSE);
    out.push_str(FOOTER);
    out
}
