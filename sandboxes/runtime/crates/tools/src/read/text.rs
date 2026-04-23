//! Text file reading: line pagination, hard gate, and line truncation.

use tokio::io::{AsyncBufReadExt, BufReader};

use super::{ReadResult, DEFAULT_LINE_LIMIT, MAX_LINE_LENGTH};
use crate::file_tracker::FileTracker;

/// Read a text file, apply the large-file hard gate, and render `ReadResult`.
pub(super) async fn read_text_file(
    file_path: &str,
    offset: usize,
    limit: usize,
    limit_explicit: bool,
    tracker: &Option<FileTracker>,
) -> Result<String, String> {
    // Walk the entire file into memory. Bounded by HARD_MAX_BYTES (10MB)
    // check above, so OOM is not a concern.
    let file = tokio::fs::File::open(file_path)
        .await
        .map_err(|e| format!("Failed to open file: {e}"))?;
    let reader = BufReader::new(file);
    let mut lines_stream = reader.lines();

    let mut all_lines: Vec<String> = Vec::new();
    while let Some(line) = lines_stream
        .next_line()
        .await
        .map_err(|e| format!("Failed to read line: {e}"))?
    {
        all_lines.push(line);
    }

    let total_lines = all_lines.len();

    // Hard gate: when the caller did not supply `limit`, refuse to
    // ingest files above the default line cap. Steers the agent toward
    // the three intents — ranged read, search, or summarize — rather
    // than silently truncating and hoping the first N lines suffice.
    if !limit_explicit && total_lines > DEFAULT_LINE_LIMIT {
        return Err(format!(
            "File {} has {} lines (default limit: {}). Choose one:\n\
            - Ranged read: call Read again with explicit offset and limit (e.g., offset=1, limit=500)\n\
            - Search: call RipGrep with path=\"{}\" and a regex pattern to find specific content\n\
            - Summarize: spawn self_agent with a focused question so the file stays out of this context",
            file_path, total_lines, DEFAULT_LINE_LIMIT, file_path
        ));
    }

    // Check for out-of-range offset (special case: offset=1 on empty file is OK)
    if offset > total_lines && !(total_lines == 0 && offset == 1) {
        return Err(format!(
            "Offset {offset} is out of range for this file ({total_lines} lines)"
        ));
    }

    // Apply offset (1-indexed) and limit
    let start = if offset > 0 { offset - 1 } else { 0 };
    let end = total_lines.min(start + limit);
    let selected_lines = if start < total_lines {
        &all_lines[start..end]
    } else {
        &[]
    };

    let lines_read = selected_lines.len();
    let truncated = end < total_lines;

    // Format lines as "{line_number}: {content}" (e.g., "1: foo")
    let mut content = String::new();
    for (i, line) in selected_lines.iter().enumerate() {
        let line_num = start + i + 1;
        let display_line = if line.len() > MAX_LINE_LENGTH {
            format!("{}...", &line[..MAX_LINE_LENGTH])
        } else {
            line.to_string()
        };
        content.push_str(&format!("{}: {}\n", line_num, display_line));
    }

    // Mark file as read for edit/write tracking
    if let Some(ref t) = tracker {
        t.mark_read(file_path);
    }

    let result = ReadResult {
        content,
        total_lines,
        lines_read,
        truncated,
    };

    serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
}
