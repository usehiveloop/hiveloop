//! Patch text parsing: heredoc stripping and hunk extraction.

/// A single parsed hunk from the patch.
#[derive(Debug, Clone)]
pub(super) enum Hunk {
    Add {
        path: String,
        contents: String,
    },
    Delete {
        path: String,
    },
    Update {
        path: String,
        move_path: Option<String>,
        chunks: Vec<UpdateChunk>,
    },
}

/// A chunk within an Update hunk.
#[derive(Debug, Clone)]
pub(super) struct UpdateChunk {
    pub(super) context: Option<String>,
    pub(super) old_lines: Vec<String>,
    pub(super) new_lines: Vec<String>,
    pub(super) is_end_of_file: bool,
}

pub(super) fn strip_heredoc(input: &str) -> String {
    // Match heredoc patterns like: cat <<'EOF'\n...\nEOF or <<EOF\n...\nEOF
    // Can't use regex backreferences in Rust's regex crate, so parse manually.
    let trimmed = input.trim();
    let first_newline = match trimmed.find('\n') {
        Some(pos) => pos,
        None => return input.to_string(),
    };

    let first_line = &trimmed[..first_newline];

    // Check if first line matches: (cat )? << 'DELIM' or <<DELIM
    let rest = first_line.trim();
    let rest = rest.strip_prefix("cat").map_or(rest, |r| r.trim_start());
    if !rest.starts_with("<<") {
        return input.to_string();
    }
    let after_arrows = rest[2..].trim_start();

    // Strip optional quotes around delimiter
    let delimiter = after_arrows
        .trim_start_matches(['\'', '"'])
        .trim_end_matches(['\'', '"'])
        .trim();

    if delimiter.is_empty() {
        return input.to_string();
    }

    // Check if the last line matches the delimiter
    let body = &trimmed[first_newline + 1..];
    if let Some(last_newline) = body.rfind('\n') {
        let last_line = body[last_newline + 1..].trim();
        if last_line == delimiter {
            return body[..last_newline].to_string();
        }
    }

    input.to_string()
}

pub(super) fn parse_patch(patch_text: &str) -> Result<Vec<Hunk>, String> {
    // Normalize CRLF and CR line endings to LF
    let patch_text = patch_text.replace("\r\n", "\n").replace('\r', "\n");
    let cleaned = strip_heredoc(patch_text.trim());
    let lines: Vec<&str> = cleaned.split('\n').collect();

    let begin_marker = "*** Begin Patch";
    let end_marker = "*** End Patch";

    let begin_idx = lines.iter().position(|l| l.trim() == begin_marker);
    let end_idx = lines.iter().position(|l| l.trim() == end_marker);

    let (begin_idx, end_idx) = match (begin_idx, end_idx) {
        (Some(b), Some(e)) if b < e => (b, e),
        _ => return Err("Invalid patch format: missing Begin/End markers".to_string()),
    };

    let mut hunks = Vec::new();
    let mut i = begin_idx + 1;

    while i < end_idx {
        let line = lines[i];

        if let Some(rest) = line.strip_prefix("*** Add File:") {
            let file_path = rest.trim().to_string();
            if file_path.is_empty() {
                return Err("Add File header has empty path".to_string());
            }
            i += 1;

            // Parse add content (+ prefixed lines)
            let mut content = String::new();
            while i < end_idx && !lines[i].starts_with("***") {
                if let Some(stripped) = lines[i].strip_prefix('+') {
                    if !content.is_empty() {
                        content.push('\n');
                    }
                    content.push_str(stripped);
                }
                i += 1;
            }

            hunks.push(Hunk::Add {
                path: file_path,
                contents: content,
            });
        } else if let Some(rest) = line.strip_prefix("*** Delete File:") {
            let file_path = rest.trim().to_string();
            if file_path.is_empty() {
                return Err("Delete File header has empty path".to_string());
            }
            hunks.push(Hunk::Delete { path: file_path });
            i += 1;
        } else if let Some(rest) = line.strip_prefix("*** Update File:") {
            let file_path = rest.trim().to_string();
            if file_path.is_empty() {
                return Err("Update File header has empty path".to_string());
            }
            i += 1;

            // Check for move directive
            let mut move_path = None;
            if i < end_idx && lines[i].starts_with("*** Move to:") {
                move_path = Some(lines[i]["*** Move to:".len()..].trim().to_string());
                i += 1;
            }

            // Parse update chunks
            let mut chunks = Vec::new();
            while i < end_idx && !lines[i].starts_with("***") {
                if lines[i].starts_with("@@") {
                    let context_line = lines[i][2..].trim().to_string();
                    let context = if context_line.is_empty() {
                        None
                    } else {
                        Some(context_line)
                    };
                    i += 1;

                    let mut old_lines = Vec::new();
                    let mut new_lines = Vec::new();
                    let mut is_end_of_file = false;

                    while i < end_idx && !lines[i].starts_with("@@") && !lines[i].starts_with("***")
                    {
                        let change_line = lines[i];

                        if change_line == "*** End of File" {
                            is_end_of_file = true;
                            i += 1;
                            break;
                        }

                        if let Some(kept) = change_line.strip_prefix(' ') {
                            old_lines.push(kept.to_string());
                            new_lines.push(kept.to_string());
                        } else if let Some(removed) = change_line.strip_prefix('-') {
                            old_lines.push(removed.to_string());
                        } else if let Some(added) = change_line.strip_prefix('+') {
                            new_lines.push(added.to_string());
                        }

                        i += 1;
                    }

                    chunks.push(UpdateChunk {
                        context,
                        old_lines,
                        new_lines,
                        is_end_of_file,
                    });
                } else {
                    i += 1;
                }
            }

            hunks.push(Hunk::Update {
                path: file_path,
                move_path,
                chunks,
            });
        } else {
            i += 1;
        }
    }

    Ok(hunks)
}
