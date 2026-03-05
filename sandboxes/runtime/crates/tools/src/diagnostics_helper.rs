use lsp::LspManager;
use lsp_types::Diagnostic;
use std::path::Path;
use std::sync::Arc;

/// Maximum number of diagnostics to include per file.
const MAX_DIAGNOSTICS_PER_FILE: usize = 20;

/// Format a single LSP diagnostic into a human-readable string.
pub fn format_diagnostic(d: &Diagnostic) -> String {
    let severity = match d.severity {
        Some(lsp_types::DiagnosticSeverity::ERROR) => "ERROR",
        Some(lsp_types::DiagnosticSeverity::WARNING) => "WARNING",
        Some(lsp_types::DiagnosticSeverity::INFORMATION) => "INFO",
        Some(lsp_types::DiagnosticSeverity::HINT) => "HINT",
        _ => "DIAGNOSTIC",
    };
    let line = d.range.start.line + 1; // LSP is 0-indexed
    let col = d.range.start.character + 1;
    format!("{severity} [{line}:{col}] {}", d.message)
}

/// Fetch LSP diagnostics for a file after a mutation.
///
/// Opens the document in the LSP server (to notify it of changes),
/// waits briefly for the server to process, then retrieves diagnostics.
/// Only ERROR-severity diagnostics are included.
///
/// Returns a formatted string suitable for appending to tool output,
/// or an empty string if there are no errors or the LSP is unavailable.
pub async fn fetch_diagnostics_output(lsp_manager: &Arc<LspManager>, file_path: &str) -> String {
    let path = Path::new(file_path);

    // Notify the LSP about the file change
    if lsp_manager.open_document(path).await.is_err() {
        return String::new();
    }

    // Small delay for LSP to process the change
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    // Fetch diagnostics
    let diagnostics = match lsp_manager.diagnostics(path).await {
        Ok(d) => d,
        Err(_) => return String::new(),
    };

    // Filter to errors only
    let errors: Vec<&Diagnostic> = diagnostics
        .iter()
        .filter(|d| d.severity == Some(lsp_types::DiagnosticSeverity::ERROR))
        .take(MAX_DIAGNOSTICS_PER_FILE)
        .collect();

    if errors.is_empty() {
        return String::new();
    }

    let formatted: Vec<String> = errors.iter().map(|d| format_diagnostic(d)).collect();
    let total_errors = diagnostics
        .iter()
        .filter(|d| d.severity == Some(lsp_types::DiagnosticSeverity::ERROR))
        .count();

    let mut output = format!("\n\nLSP Diagnostics ({total_errors} error(s)):\n");
    for line in &formatted {
        output.push_str(&format!("  {line}\n"));
    }
    if total_errors > MAX_DIAGNOSTICS_PER_FILE {
        output.push_str(&format!(
            "  ... and {} more error(s)\n",
            total_errors - MAX_DIAGNOSTICS_PER_FILE
        ));
    }

    output
}

#[cfg(test)]
mod tests {
    use super::*;
    use lsp_types::{DiagnosticSeverity, Position, Range};

    fn make_diagnostic(severity: DiagnosticSeverity, line: u32, col: u32, msg: &str) -> Diagnostic {
        Diagnostic {
            range: Range {
                start: Position {
                    line,
                    character: col,
                },
                end: Position {
                    line,
                    character: col + 1,
                },
            },
            severity: Some(severity),
            message: msg.to_string(),
            ..Default::default()
        }
    }

    #[test]
    fn test_format_diagnostic_error() {
        let d = make_diagnostic(DiagnosticSeverity::ERROR, 9, 4, "expected `;`");
        let result = format_diagnostic(&d);
        assert_eq!(result, "ERROR [10:5] expected `;`");
    }

    #[test]
    fn test_format_diagnostic_warning() {
        let d = make_diagnostic(DiagnosticSeverity::WARNING, 0, 0, "unused variable");
        let result = format_diagnostic(&d);
        assert_eq!(result, "WARNING [1:1] unused variable");
    }

    #[test]
    fn test_format_diagnostic_info() {
        let d = make_diagnostic(
            DiagnosticSeverity::INFORMATION,
            5,
            10,
            "consider refactoring",
        );
        let result = format_diagnostic(&d);
        assert_eq!(result, "INFO [6:11] consider refactoring");
    }
}
