/// Maximum output size in bytes before truncation (~2KB).
/// Must stay at or below the central tool_hook cap so the already-spilled
/// output isn't re-truncated (which would lose the spill-path pointer).
pub(super) const MAX_OUTPUT_BYTES: usize = 2048;

/// Head/tail sizes for the spill summary. Chosen so head + marker + tail stays
/// under MAX_OUTPUT_BYTES with headroom for the path and byte-count numbers.
const SPILL_HEAD_BYTES: usize = 800;
const SPILL_TAIL_BYTES: usize = 800;

pub(super) fn truncate_output(bytes: &[u8]) -> String {
    let s = String::from_utf8_lossy(bytes);
    if s.len() <= MAX_OUTPUT_BYTES {
        return s.into_owned();
    }

    // Spill full output to a temp file so the LLM can read it later
    let spill_path = std::env::temp_dir().join(format!("bridge_bash_{}.txt", uuid::Uuid::new_v4()));
    if let Ok(()) = std::fs::write(&spill_path, bytes) {
        let head_end = s.floor_char_boundary(s.len().min(SPILL_HEAD_BYTES));
        let head = &s[..head_end];
        let tail_start = s.ceil_char_boundary(s.len().saturating_sub(SPILL_TAIL_BYTES));
        let tail = &s[tail_start..];
        let path_str = spill_path.display().to_string();
        format!(
            "{head}\n\n... [Output truncated. Full output ({} bytes) saved to: {path_str}. To find specific content, call the RipGrep tool with path=\"{path_str}\" and a regex pattern.] ...\n\n{tail}",
            bytes.len(),
        )
    } else {
        // Fallback if we can't write the temp file
        let end = s.floor_char_boundary(MAX_OUTPUT_BYTES);
        let truncated = &s[..end];
        format!("{truncated}\n[output truncated]")
    }
}
