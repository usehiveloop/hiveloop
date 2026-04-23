//! Patch application: compute replacements, apply them, and write files.

use std::path::Path;

use super::matcher::seek_sequence;
use super::parser::{Hunk, UpdateChunk};

pub(super) fn compute_replacements(
    original_lines: &[String],
    file_path: &str,
    chunks: &[UpdateChunk],
) -> Result<Vec<(usize, usize, Vec<String>)>, String> {
    let mut replacements: Vec<(usize, usize, Vec<String>)> = Vec::new();
    let mut line_index: usize = 0;

    for chunk in chunks {
        // Handle context-based seeking
        if let Some(ref context) = chunk.context {
            let context_pattern = vec![context.clone()];
            let context_idx = seek_sequence(original_lines, &context_pattern, line_index, false)
                .ok_or_else(|| format!("Failed to find context '{}' in {}", context, file_path))?;
            line_index = context_idx + 1;
        }

        // Handle pure addition (no old lines)
        if chunk.old_lines.is_empty() {
            let insertion_idx = if !original_lines.is_empty()
                && original_lines.last().is_some_and(|l| l.is_empty())
            {
                original_lines.len() - 1
            } else {
                original_lines.len()
            };
            replacements.push((insertion_idx, 0, chunk.new_lines.clone()));
            continue;
        }

        // Try to match old lines in the file
        let mut pattern = chunk.old_lines.clone();
        let mut new_slice = chunk.new_lines.clone();
        let found = seek_sequence(original_lines, &pattern, line_index, chunk.is_end_of_file);

        let found = match found {
            Some(idx) => Some(idx),
            None => {
                // Retry without trailing empty line
                if !pattern.is_empty() && pattern.last().is_some_and(|l| l.is_empty()) {
                    pattern.pop();
                    if !new_slice.is_empty() && new_slice.last().is_some_and(|l| l.is_empty()) {
                        new_slice.pop();
                    }
                    seek_sequence(original_lines, &pattern, line_index, chunk.is_end_of_file)
                } else {
                    None
                }
            }
        };

        if let Some(idx) = found {
            replacements.push((idx, pattern.len(), new_slice));
            line_index = idx + pattern.len();
        } else {
            return Err(format!(
                "Failed to find expected lines in {}:\n{}",
                file_path,
                chunk.old_lines.join("\n")
            ));
        }
    }

    // Sort replacements by index
    replacements.sort_by_key(|r| r.0);

    Ok(replacements)
}

pub(super) fn apply_replacements(
    lines: &[String],
    replacements: &[(usize, usize, Vec<String>)],
) -> Vec<String> {
    let mut result: Vec<String> = lines.to_vec();

    // Apply replacements in reverse order to avoid index shifting
    for &(start_idx, old_len, ref new_segment) in replacements.iter().rev() {
        // Remove old lines and insert new ones
        let end = (start_idx + old_len).min(result.len());
        result.splice(start_idx..end, new_segment.iter().cloned());
    }

    result
}

pub(super) async fn apply_hunks(hunks: &[Hunk]) -> Result<Vec<String>, String> {
    let mut summary = Vec::new();

    for hunk in hunks {
        match hunk {
            Hunk::Add { path, contents } => {
                let p = Path::new(path);
                if let Some(parent) = p.parent() {
                    if !parent.exists() {
                        tokio::fs::create_dir_all(parent)
                            .await
                            .map_err(|e| format!("Failed to create dirs for {path}: {e}"))?;
                    }
                }
                tokio::fs::write(path, contents)
                    .await
                    .map_err(|e| format!("Failed to write {path}: {e}"))?;
                summary.push(format!("A {path}"));
            }
            Hunk::Delete { path } => {
                tokio::fs::remove_file(path)
                    .await
                    .map_err(|e| format!("Failed to delete {path}: {e}"))?;
                summary.push(format!("D {path}"));
            }
            Hunk::Update {
                path,
                move_path,
                chunks,
            } => {
                let content = tokio::fs::read_to_string(path)
                    .await
                    .map_err(|e| format!("Failed to read {path}: {e}"))?;
                // Normalize CRLF/CR to LF
                let content = content.replace("\r\n", "\n").replace('\r', "\n");

                let mut original_lines: Vec<String> =
                    content.split('\n').map(|s| s.to_string()).collect();

                // Drop trailing empty element for consistent line counting
                if original_lines.last().is_some_and(|l| l.is_empty()) {
                    original_lines.pop();
                }

                let replacements = compute_replacements(&original_lines, path, chunks)?;
                let mut new_lines = apply_replacements(&original_lines, &replacements);

                // Ensure trailing newline
                if new_lines.is_empty() || !new_lines.last().is_none_or(|l| l.is_empty()) {
                    new_lines.push(String::new());
                }

                let new_content = new_lines.join("\n");

                let target_path = move_path.as_deref().unwrap_or(path.as_str());

                if let Some(mp) = move_path {
                    let target = Path::new(mp.as_str());
                    if let Some(parent) = target.parent() {
                        if !parent.exists() {
                            tokio::fs::create_dir_all(parent)
                                .await
                                .map_err(|e| format!("Failed to create dirs for {mp}: {e}"))?;
                        }
                    }
                    tokio::fs::write(mp, &new_content)
                        .await
                        .map_err(|e| format!("Failed to write {mp}: {e}"))?;
                    tokio::fs::remove_file(path)
                        .await
                        .map_err(|e| format!("Failed to remove old file {path}: {e}"))?;
                } else {
                    tokio::fs::write(path, &new_content)
                        .await
                        .map_err(|e| format!("Failed to write {path}: {e}"))?;
                }

                summary.push(format!("M {target_path}"));
            }
        }
    }

    Ok(summary)
}
