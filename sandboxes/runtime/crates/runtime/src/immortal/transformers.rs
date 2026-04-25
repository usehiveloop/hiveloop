//! Post-extraction transformers, applied to a `ContextSummary` before
//! rendering to markdown. Mirrors forgecode's `SummaryTransformer` pipeline:
//!
//! ```text
//! DropRole(System) → DedupeRole(User) → TrimContextSummary → StripWorkingDir
//! ```
//!
//! Each transformer is small, deterministic, and composable. Applied in
//! order; later passes see the output of earlier ones.

use std::path::Path;

use super::summary::{ContextSummary, SummaryBlock, SummaryMessage, SummaryRole, SummaryTool};

/// Apply the full forgecode-style transformer pipeline.
pub fn apply_pipeline(summary: ContextSummary, working_dir: Option<&Path>) -> ContextSummary {
    let summary = dedupe_consecutive_user_blocks(summary);
    let summary = trim_consecutive_same_op(summary);
    if let Some(wd) = working_dir {
        strip_working_dir(summary, wd)
    } else {
        summary
    }
}

/// Merge consecutive `User` blocks into one. Forgecode's `DedupeRole(User)`.
/// (System blocks are already excluded by the extractor; assistant blocks
/// alternate naturally.)
fn dedupe_consecutive_user_blocks(summary: ContextSummary) -> ContextSummary {
    let mut out: Vec<SummaryBlock> = Vec::with_capacity(summary.messages.len());
    for block in summary.messages {
        if let Some(last) = out.last_mut() {
            if last.role == SummaryRole::User && block.role == SummaryRole::User {
                last.contents.extend(block.contents);
                continue;
            }
        }
        out.push(block);
    }
    ContextSummary { messages: out }
}

/// Within each assistant block, drop a tool call if the IMMEDIATELY
/// PRECEDING block content is a tool call on the same resource (same file
/// path, same shell command, etc.). Mirrors forgecode's
/// `TrimContextSummary`. Crucially: only consecutive same-resource ops
/// collapse — interleaved ops are preserved.
fn trim_consecutive_same_op(mut summary: ContextSummary) -> ContextSummary {
    for block in summary.messages.iter_mut() {
        if block.role != SummaryRole::Assistant {
            continue;
        }
        let mut kept: Vec<SummaryMessage> = Vec::with_capacity(block.contents.len());
        for content in std::mem::take(&mut block.contents) {
            if let SummaryMessage::ToolCall(ref new_call) = content {
                if let Some(SummaryMessage::ToolCall(prev_call)) = kept.last() {
                    if same_op(&prev_call.tool, &new_call.tool) {
                        kept.pop();
                    }
                }
            }
            kept.push(content);
        }
        block.contents = kept;
    }
    summary
}

/// Two tools target the same resource if they're the same variant AND
/// the load-bearing field (path, command, pattern, etc.) matches. This
/// is what makes "Read x.rs, Read x.rs" collapse to one but "Read x.rs,
/// Read y.rs" stay as two.
fn same_op(a: &SummaryTool, b: &SummaryTool) -> bool {
    use SummaryTool::*;
    match (a, b) {
        (FileRead { path: p1 }, FileRead { path: p2 }) => p1 == p2,
        (FileUpdate { path: p1 }, FileUpdate { path: p2 }) => p1 == p2,
        (Shell { command: c1 }, Shell { command: c2 }) => c1 == c2,
        (Glob { pattern: p1 }, Glob { pattern: p2 }) => p1 == p2,
        (
            Search {
                pattern: p1,
                path: pa1,
            },
            Search {
                pattern: p2,
                path: pa2,
            },
        ) => p1 == p2 && pa1 == pa2,
        (List { path: p1 }, List { path: p2 }) => p1 == p2,
        (TodoRead, TodoRead) => true,
        (
            Lsp {
                action: a1,
                target: t1,
            },
            Lsp {
                action: a2,
                target: t2,
            },
        ) => a1 == a2 && t1 == t2,
        (Mcp { name: n1 }, Mcp { name: n2 }) => n1 == n2,
        // TodoWrite is never deduped — every write is meaningful (it's
        // a state change, not a query).
        _ => false,
    }
}

/// Strip a leading `working_dir/` (or `working_dir`) prefix from any path
/// field in the summary. Mirrors forgecode's `StripWorkingDir`. The goal
/// is shorter, less-noisy paths in the rendered markdown.
fn strip_working_dir(mut summary: ContextSummary, working_dir: &Path) -> ContextSummary {
    let prefix = working_dir
        .to_string_lossy()
        .trim_end_matches('/')
        .to_string();
    if prefix.is_empty() {
        return summary;
    }
    let prefix_with_slash = format!("{}/", prefix);
    let strip = |s: &mut String| {
        if let Some(rest) = s.strip_prefix(&prefix_with_slash) {
            *s = rest.to_string();
        } else if s == prefix.as_str() {
            *s = ".".to_string();
        }
    };
    for block in summary.messages.iter_mut() {
        for content in block.contents.iter_mut() {
            if let SummaryMessage::ToolCall(tc) = content {
                match &mut tc.tool {
                    SummaryTool::FileRead { path } => strip(path),
                    SummaryTool::FileUpdate { path } => strip(path),
                    SummaryTool::List { path } => strip(path),
                    SummaryTool::Search {
                        path: Some(path), ..
                    } => strip(path),
                    SummaryTool::Lsp {
                        target: Some(target),
                        ..
                    } => strip(target),
                    _ => {}
                }
            }
        }
    }
    summary
}
