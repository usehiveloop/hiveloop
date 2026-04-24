use bridge_core::agent::HistoryStripConfig;
use rig::message::{Message, ToolResultContent, UserContent};
use rig::one_or_many::OneOrMany;

mod helpers;
#[cfg(test)]
mod tests;

use helpers::{
    build_strip_marker, extract_spill_path, is_already_stripped, is_exempt, looks_like_error,
    tool_result_byte_count, MIN_STRIPPABLE_BYTES, PER_RESULT_SLOT_BYTES,
};

/// Strip old tool-result bodies from `history` in place.
///
/// Walks backward, preserving the most recent window of tool output (sized
/// via `config.age_threshold` * ~2KB-per-result) and replacing older bodies
/// with a short pointer that names the on-disk spill file (from the 2KB-cap
/// pipeline) so the agent can recover specifics via `RipGrep`.
///
/// Determinism guarantee: for a given input history and config, the output
/// is byte-identical across calls. Once a result is stripped, it stays
/// stripped (we detect the marker prefix and skip). This keeps the provider
/// prompt cache hot after the first strip.
///
/// Skips in order of precedence:
///   1. Non-`User::ToolResult` content.
///   2. Already-stripped results (marker prefix present).
///   3. Exempt tools (journal, todo).
///   4. Results with `is_error: true` when `config.pin_errors` is set.
///   5. Results with no detectable spill path — small enough to keep inline.
///   6. Results whose bytes fit within the protection window.
pub fn strip_old_tool_outputs(history: &mut [Message], config: &HistoryStripConfig) {
    if !config.enabled {
        return;
    }

    let protection_budget = config.age_threshold.saturating_mul(PER_RESULT_SLOT_BYTES);

    // Phase 1: Walk backward, identify which message indices to strip.
    let mut protected_bytes: usize = 0;
    let mut should_strip: std::collections::HashSet<usize> = std::collections::HashSet::new();

    for msg_idx in (0..history.len()).rev() {
        let content = match &history[msg_idx] {
            Message::User { content } => content,
            _ => continue,
        };

        for part in content.iter() {
            let tr = match part {
                UserContent::ToolResult(tr) => tr,
                _ => continue,
            };

            let text_bytes = tool_result_byte_count(tr);

            if text_bytes < MIN_STRIPPABLE_BYTES {
                continue;
            }
            if is_exempt(&tr.id) {
                continue;
            }
            if is_already_stripped(tr) {
                // Counts against the protection budget so results further back
                // don't slip into the protected window when one ahead of them
                // is already a pointer-sized marker.
                protected_bytes += text_bytes;
                continue;
            }
            if config.pin_errors && looks_like_error(tr) {
                protected_bytes += text_bytes;
                continue;
            }
            // Spill-path is no longer required. When present, the marker
            // tells the agent how to recover the full output via RipGrep.
            // When absent (most Read, RipGrep, Glob, todowrite, etc. results),
            // we still strip the body and leave a generic marker — the agent
            // can always re-call the tool if it needs the content again. A
            // "small" 500B result that's repeated across 60 turns still adds
            // up to 30KB of cumulative cached input; over a long conversation
            // these dominate the bill. We'd rather drop and let the model
            // re-fetch than bloat history.

            if protected_bytes + text_bytes <= protection_budget {
                protected_bytes += text_bytes;
            } else {
                should_strip.insert(msg_idx);
            }
        }
    }

    // Phase 2: Rewrite stripped messages.
    for msg_idx in &should_strip {
        let new_msg = {
            let content = match &history[*msg_idx] {
                Message::User { content } => content,
                _ => continue,
            };

            let new_parts: Vec<UserContent> = content
                .iter()
                .map(|part| match part {
                    UserContent::ToolResult(tr) => {
                        if should_strip_result(tr, config) {
                            let bytes = tool_result_byte_count(tr);
                            let spill_path = extract_spill_path(tr).unwrap_or_default();
                            UserContent::ToolResult(rig::message::ToolResult {
                                id: tr.id.clone(),
                                call_id: tr.call_id.clone(),
                                content: OneOrMany::one(ToolResultContent::Text(
                                    rig::message::Text {
                                        text: build_strip_marker(bytes, &spill_path),
                                    },
                                )),
                            })
                        } else {
                            part.clone()
                        }
                    }
                    other => other.clone(),
                })
                .collect();

            match OneOrMany::many(new_parts) {
                Ok(new_content) => Some(Message::User {
                    content: new_content,
                }),
                Err(_) => None,
            }
        };

        if let Some(msg) = new_msg {
            history[*msg_idx] = msg;
        }
    }
}

/// Strip with default config (enabled, standard thresholds). Convenience
/// wrapper used in tests and by call sites that don't carry a per-agent
/// config yet.
pub fn strip_old_tool_outputs_default(history: &mut [Message]) {
    strip_old_tool_outputs(history, &HistoryStripConfig::default());
}

fn should_strip_result(tr: &rig::message::ToolResult, config: &HistoryStripConfig) -> bool {
    if is_exempt(&tr.id) {
        return false;
    }
    if is_already_stripped(tr) {
        return false;
    }
    if config.pin_errors && looks_like_error(tr) {
        return false;
    }
    if tool_result_byte_count(tr) < MIN_STRIPPABLE_BYTES {
        return false;
    }
    // NOTE: previously required `extract_spill_path(tr).is_some()` — that's
    // the bug that caused strip to fire zero times over an entire 100-turn
    // run. Phase 1 (above) was relaxed but this Phase 2 gate silently kept
    // the old behavior. Now: any tool result meeting the size + exempt
    // criteria is strippable. When no spill path exists, `build_strip_marker`
    // emits a "re-call the tool if needed" marker (see helpers.rs).
    true
}
