//! Render a `ContextSummary` into the final markdown frame.
//!
//! Mirrors `forgecode/templates/forge-partial-summary-frame.md` verbatim
//! (within the limits of bridge's tool set). The output is a single string
//! that bridge splices into the conversation history as a user-role message
//! — no fake assistant ack, no scaffolding pairs.

use std::fmt::Write;

use super::summary::{
    ContextSummary, SummaryMessage, SummaryTool, TodoChange, TodoChangeKind, TodoStatus,
};

/// Header directive injected at the top of every summary frame. Tells the
/// receiving agent to treat the summary as authoritative and stop
/// re-exploring. Word-for-word equivalent of forgecode's preamble.
pub(crate) const HEADER: &str = "Use the following summary frames as the authoritative reference for all coding suggestions and decisions. Do not re-explain or revisit it unless I ask. Additional summary frames will be added as the conversation progresses.";

/// Footer that nudges the agent to take its next concrete action.
pub(crate) const FOOTER: &str = "Proceed with implementation based on this context.";

/// Body separator between header and the per-message blocks in every frame.
pub(crate) const SUMMARY_BODY_OPEN: &str = "\n\n## Summary\n\n";

/// Separator between the body and the footer.
pub(crate) const SUMMARY_BODY_CLOSE: &str = "---\n\n";

/// Render a `ContextSummary` into a single markdown string.
pub fn render(summary: &ContextSummary) -> String {
    let mut out = String::new();
    out.push_str(HEADER);
    out.push_str(SUMMARY_BODY_OPEN);
    for (idx, block) in summary.messages.iter().enumerate() {
        let _ = write!(out, "### {}. {}\n\n", idx + 1, block.role.label());
        for content in &block.contents {
            render_content(&mut out, content);
            out.push('\n');
        }
        out.push('\n');
    }
    out.push_str(SUMMARY_BODY_CLOSE);
    out.push_str(FOOTER);
    out
}

fn render_content(out: &mut String, content: &SummaryMessage) {
    match content {
        SummaryMessage::Text(text) => {
            // Quad-backtick fence — lets the inner text contain
            // triple-backtick fences without breaking out.
            let _ = write!(out, "````\n{}\n````", text);
        }
        SummaryMessage::ToolCall(tc) => {
            let marker = if tc.is_success { "" } else { " ✗" };
            match &tc.tool {
                SummaryTool::FileRead { path } => {
                    let _ = write!(out, "**Read:** `{}`{}", path, marker);
                }
                SummaryTool::FileUpdate { path } => {
                    let _ = write!(out, "**Update:** `{}`{}", path, marker);
                }
                SummaryTool::Shell { command } => {
                    if command.contains('\n') || command.len() > 80 {
                        let _ = write!(out, "**Execute:**{}\n```\n{}\n```", marker, command.trim());
                    } else {
                        let _ = write!(out, "**Execute:** `{}`{}", command.trim(), marker);
                    }
                }
                SummaryTool::Glob { pattern } => {
                    let _ = write!(out, "**Glob:** `{}`{}", pattern, marker);
                }
                SummaryTool::Search { pattern, path } => match path {
                    Some(p) => {
                        let _ = write!(out, "**Search:** `{}` in `{}`{}", pattern, p, marker);
                    }
                    None => {
                        let _ = write!(out, "**Search:** `{}`{}", pattern, marker);
                    }
                },
                SummaryTool::List { path } => {
                    let _ = write!(out, "**List:** `{}`{}", path, marker);
                }
                SummaryTool::TodoWrite { changes } => {
                    out.push_str("**Task Plan:**");
                    for change in changes {
                        out.push('\n');
                        render_todo_change(out, change);
                    }
                }
                SummaryTool::TodoRead => {
                    let _ = write!(out, "**Task Plan:** (read){}", marker);
                }
                SummaryTool::Lsp { action, target } => match target {
                    Some(t) => {
                        let _ = write!(out, "**LSP {}:** `{}`{}", action, t, marker);
                    }
                    None => {
                        let _ = write!(out, "**LSP {}**{}", action, marker);
                    }
                },
                SummaryTool::Mcp { name } => {
                    let _ = write!(out, "**{}**{}", name, marker);
                }
            }
        }
    }
}

fn render_todo_change(out: &mut String, change: &TodoChange) {
    match change.kind {
        TodoChangeKind::Added => {
            let _ = write!(out, "- [ADD] {}", change.content);
        }
        TodoChangeKind::Updated => match change.status {
            TodoStatus::Completed => {
                let _ = write!(out, "- [DONE] ~~{}~~", change.content);
            }
            TodoStatus::InProgress => {
                let _ = write!(out, "- [IN_PROGRESS] {}", change.content);
            }
            _ => {
                let _ = write!(out, "- [UPDATE] {}", change.content);
            }
        },
        TodoChangeKind::Removed => {
            let _ = write!(out, "- [CANCELLED] ~~{}~~", change.content);
        }
    }
}

#[allow(dead_code)]
pub(super) const fn header() -> &'static str {
    HEADER
}

#[allow(dead_code)]
pub(super) const fn footer() -> &'static str {
    FOOTER
}
