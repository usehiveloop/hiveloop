//! Conversation message attachments.
//!
//! Callers can include a `full_message` field on `POST /conversations/.../messages`
//! when the message content is too large to send to the LLM directly (huge
//! stack traces, log dumps, file contents). Bridge writes the full payload
//! to disk under a per-conversation directory, leaves the short `content`
//! the caller provided (or a first-N-byte auto-summary if they didn't), and
//! appends a `<system-reminder>` pointing the agent at the file — with a
//! search-tool hint tailored to what's actually registered for that agent.
//!
//! Failures are **never surfaced as API errors**: if the disk write fails,
//! we log a warning and deliver the message without the attachment. This
//! preserves the invariant that `full_message` cannot block a message from
//! being accepted.

use std::collections::HashSet;
use std::path::{Path, PathBuf};

use tokio::fs;
use tracing::warn;

/// Environment variable that overrides the attachments root directory.
pub const ENV_ATTACHMENTS_DIR: &str = "BRIDGE_ATTACHMENTS_DIR";

/// Default attachments root when the env var is unset — relative to
/// bridge's cwd so agent tools like `Read`/`RipGrep` naturally reach it
/// via the usual filesystem boundary.
pub const DEFAULT_ATTACHMENTS_DIR: &str = ".bridge-attachments";

/// Number of leading bytes of `full_message` used as the summary when the
/// caller omitted `content`. Small enough to stay tiny in context, large
/// enough to give the model a meaningful hint of what's in the file.
pub const AUTO_SUMMARY_BYTES: usize = 500;

/// Root directory for conversation attachments.
pub fn attachments_root() -> PathBuf {
    std::env::var(ENV_ATTACHMENTS_DIR)
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from(DEFAULT_ATTACHMENTS_DIR))
}

/// Write `full_message` to `{attachments_root}/{conv_id}/{uuid}.txt` and
/// return the **absolute** path. Any failure is logged and `None` is
/// returned — callers proceed without the attachment.
pub async fn write_full_message(conv_id: &str, full_message: &str) -> Option<PathBuf> {
    let dir = attachments_root().join(conv_id);
    if let Err(e) = fs::create_dir_all(&dir).await {
        warn!(
            conversation_id = %conv_id,
            error = %e,
            "failed to create attachments directory — proceeding without attachment"
        );
        return None;
    }
    let filename = format!("{}.txt", uuid::Uuid::new_v4());
    let path = dir.join(filename);
    if let Err(e) = fs::write(&path, full_message.as_bytes()).await {
        warn!(
            conversation_id = %conv_id,
            path = %path.display(),
            error = %e,
            "failed to write attachment — proceeding without attachment"
        );
        return None;
    }
    // Return an absolute path if possible — agents may run with different
    // `cwd` assumptions and relative paths are fragile. `canonicalize`
    // requires the file to exist (it does; we just wrote it).
    match fs::canonicalize(&path).await {
        Ok(abs) => Some(abs),
        Err(_) => std::env::current_dir().ok().map(|cwd| cwd.join(&path)),
    }
}

/// Remove a conversation's attachments directory. Best-effort: missing
/// directory is a non-error; other failures are logged and swallowed.
pub async fn cleanup_conversation_attachments(conv_id: &str) {
    let dir = attachments_root().join(conv_id);
    if !dir.exists() {
        return;
    }
    if let Err(e) = fs::remove_dir_all(&dir).await {
        warn!(
            conversation_id = %conv_id,
            path = %dir.display(),
            error = %e,
            "failed to clean up attachments directory on conversation end"
        );
    }
}

/// Build the `<system-reminder>` block that tells the agent where the
/// full payload lives and which tool(s) to use to read it. Tool hint
/// degrades gracefully when the obvious tools (`RipGrep`, `Read`) aren't
/// registered for this specific agent.
pub fn build_reminder(attachment_path: &Path, available_tools: &HashSet<String>) -> String {
    let path = attachment_path.display();
    let has_ripgrep = available_tools.contains("RipGrep");
    let has_read = available_tools.contains("Read");
    let has_ast_grep = available_tools.contains("AstGrep");
    let has_bash = available_tools.contains("bash");

    let tool_hint = match (has_ripgrep, has_read) {
        (true, true) => "Use the `RipGrep` tool to search the file for specifics, or the \
             `Read` tool to open a specific byte/line range."
            .to_string(),
        (true, false) => {
            "Use the `RipGrep` tool to search the file for the specifics you need.".to_string()
        }
        (false, true) => "Use the `Read` tool to open the file (prefer a specific line range \
             to keep your context window light)."
            .to_string(),
        (false, false) => {
            // Neither built-in search/read is available — pick any reasonable fallback.
            if has_ast_grep {
                "Use the `AstGrep` tool to inspect the file.".to_string()
            } else if has_bash {
                "Use the `bash` tool (e.g. `grep -n <pattern> <path>` or `sed -n '100,150p' <path>`) \
                 to inspect the file. Avoid `cat <path>` — it would dump the full payload back \
                 into context, defeating the purpose of the attachment."
                    .to_string()
            } else {
                "Your agent does not have a filesystem-read or search tool registered, \
                 so you cannot inspect this file directly — treat the summary above as \
                 authoritative and ask the user for specifics if needed."
                    .to_string()
            }
        }
    };

    format!(
        "<system-reminder>\nThe user's message was truncated because it was too long. \
         The complete original payload is saved to `{path}`. \
         {tool_hint}\n</system-reminder>"
    )
}

/// Build the final LLM-visible content for a message that carried a
/// `full_message` attachment. If the caller also supplied `content`, we
/// use it as the summary. If not, we take the first [`AUTO_SUMMARY_BYTES`]
/// of the full message (cut on a char boundary) as a best-effort summary
/// — we never reject the message for missing `content`.
pub fn compose_with_attachment(
    caller_content: &str,
    full_message: &str,
    attachment_path: &Path,
    available_tools: &HashSet<String>,
) -> String {
    let summary = if !caller_content.trim().is_empty() {
        caller_content.to_string()
    } else {
        auto_summary(full_message)
    };
    let reminder = build_reminder(attachment_path, available_tools);
    format!("{summary}\n\n{reminder}")
}

/// Truncate `full_message` to a safe char boundary ≤ `AUTO_SUMMARY_BYTES`.
/// Appends an ellipsis marker so the agent knows it's a truncation.
fn auto_summary(full_message: &str) -> String {
    if full_message.len() <= AUTO_SUMMARY_BYTES {
        return full_message.to_string();
    }
    // Back off to the nearest char boundary so we don't split a UTF-8 codepoint.
    let mut end = AUTO_SUMMARY_BYTES;
    while end > 0 && !full_message.is_char_boundary(end) {
        end -= 1;
    }
    format!(
        "{}\n\n[… truncated — see attached file for the complete payload]",
        &full_message[..end]
    )
}

// ── Tests ────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn tools(names: &[&str]) -> HashSet<String> {
        names.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn test_reminder_with_rg_and_read() {
        let r = build_reminder(Path::new("/tmp/foo.txt"), &tools(&["RipGrep", "Read"]));
        assert!(r.contains("/tmp/foo.txt"));
        assert!(r.contains("`RipGrep`"));
        assert!(r.contains("`Read`"));
    }

    #[test]
    fn test_reminder_rg_only() {
        let r = build_reminder(Path::new("/tmp/foo.txt"), &tools(&["RipGrep"]));
        assert!(r.contains("`RipGrep`"));
        assert!(!r.contains("`Read`"));
    }

    #[test]
    fn test_reminder_read_only() {
        let r = build_reminder(Path::new("/tmp/foo.txt"), &tools(&["Read"]));
        assert!(r.contains("`Read`"));
        assert!(!r.contains("`RipGrep`"));
    }

    #[test]
    fn test_reminder_fallback_bash() {
        let r = build_reminder(Path::new("/tmp/foo.txt"), &tools(&["bash"]));
        assert!(r.contains("`bash`"));
        assert!(r.contains("grep"));
        assert!(r.contains("Avoid"));
    }

    #[test]
    fn test_reminder_no_read_tools() {
        let r = build_reminder(Path::new("/tmp/foo.txt"), &tools(&["some_other_tool"]));
        // Must still mention the path so clients / humans can find it.
        assert!(r.contains("/tmp/foo.txt"));
        // Must explicitly call out the missing-tool situation.
        assert!(r.to_lowercase().contains("does not have"));
    }

    #[test]
    fn test_compose_uses_caller_summary_when_provided() {
        let out = compose_with_attachment(
            "please fix this error",
            "ERROR: segmentation fault in handler...",
            Path::new("/tmp/x.txt"),
            &tools(&["RipGrep", "Read"]),
        );
        assert!(out.starts_with("please fix this error"));
        assert!(out.contains("<system-reminder>"));
    }

    #[test]
    fn test_compose_auto_summary_when_caller_blank() {
        let full = "X".repeat(2000);
        let out = compose_with_attachment("", &full, Path::new("/tmp/x.txt"), &tools(&[]));
        // Takes first AUTO_SUMMARY_BYTES as summary.
        let summary_marker = "[… truncated";
        assert!(out.contains(summary_marker));
        // Summary is present before the reminder tag.
        let summary_idx = out.find(summary_marker).unwrap();
        let reminder_idx = out.find("<system-reminder>").unwrap();
        assert!(summary_idx < reminder_idx);
    }

    #[test]
    fn test_compose_short_full_message_no_truncation() {
        let full = "short payload";
        let out = compose_with_attachment("", full, Path::new("/tmp/x.txt"), &tools(&["Read"]));
        assert!(out.starts_with("short payload"));
        assert!(!out.contains("truncated —")); // no truncation marker for small inputs
    }

    #[test]
    fn test_auto_summary_respects_utf8_boundary() {
        // Build a string where AUTO_SUMMARY_BYTES lands inside a multi-byte codepoint.
        let prefix = "a".repeat(AUTO_SUMMARY_BYTES - 1);
        // Pushing a 2-byte emoji byte-1 lands at position AUTO_SUMMARY_BYTES.
        let input = format!("{prefix}é trailing content that extends past the cap");
        // Must not panic — and the returned string must be valid UTF-8.
        let out = auto_summary(&input);
        let _ = out.as_str(); // proves valid UTF-8
    }

    #[test]
    fn test_attachments_root_respects_env() {
        let prev = std::env::var(ENV_ATTACHMENTS_DIR).ok();
        unsafe {
            std::env::set_var(ENV_ATTACHMENTS_DIR, "/tmp/custom-attachments");
        }
        let root = attachments_root();
        assert_eq!(root, PathBuf::from("/tmp/custom-attachments"));
        unsafe {
            match prev {
                Some(v) => std::env::set_var(ENV_ATTACHMENTS_DIR, v),
                None => std::env::remove_var(ENV_ATTACHMENTS_DIR),
            }
        }
    }
}
