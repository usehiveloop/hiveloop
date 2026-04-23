use std::path::Path;

use lsp::manager::{format_location, uri_to_path};
use lsp::LspManager;

use super::types::*;

pub(super) async fn go_to_definition(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let (line, character) = args.position()?;
    let result = manager
        .definition(file, line, character)
        .await
        .map_err(|e| e.to_string())?;

    match result {
        Some(loc) => {
            let result = LocationResult {
                file: uri_to_path(loc.uri.as_str())
                    .map(|p| p.display().to_string())
                    .unwrap_or_else(|| loc.uri.to_string()),
                line: loc.range.start.line + 1,
                character: loc.range.start.character + 1,
            };
            serde_json::to_string_pretty(&result).map_err(|e| e.to_string())
        }
        None => Ok("No definition found at this position.".into()),
    }
}

pub(super) async fn find_references(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let (line, character) = args.position()?;
    let locs = manager
        .references(file, line, character)
        .await
        .map_err(|e| e.to_string())?;

    if locs.is_empty() {
        return Ok("No references found.".into());
    }

    let results: Vec<LocationResult> = locs
        .iter()
        .map(|loc| LocationResult {
            file: uri_to_path(loc.uri.as_str())
                .map(|p| p.display().to_string())
                .unwrap_or_else(|| loc.uri.to_string()),
            line: loc.range.start.line + 1,
            character: loc.range.start.character + 1,
        })
        .collect();

    serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
}

pub(super) async fn hover(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let (line, character) = args.position()?;
    let result = manager
        .hover(file, line, character)
        .await
        .map_err(|e| e.to_string())?;

    match result {
        Some(hover) => {
            let content = match hover.contents {
                lsp_types::HoverContents::Scalar(mc) => markup_content_to_string(mc),
                lsp_types::HoverContents::Array(arr) => arr
                    .into_iter()
                    .map(markup_content_to_string)
                    .collect::<Vec<_>>()
                    .join("\n\n"),
                lsp_types::HoverContents::Markup(mc) => mc.value,
            };
            Ok(content)
        }
        None => Ok("No hover information at this position.".into()),
    }
}

pub(super) async fn document_symbol(manager: &LspManager, file: &Path) -> Result<String, String> {
    let symbols = manager
        .document_symbols(file)
        .await
        .map_err(|e| e.to_string())?;

    if symbols.is_empty() {
        return Ok("No symbols found in document.".into());
    }

    let results: Vec<SymbolResult> = symbols.into_iter().map(convert_document_symbol).collect();

    serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
}

pub(super) async fn workspace_symbol(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let query = args.query.as_deref().unwrap_or("");
    let symbols = manager
        .workspace_symbols(file, query)
        .await
        .map_err(|e| e.to_string())?;

    // Filter to relevant symbol kinds and limit results
    let filtered: Vec<&lsp_types::SymbolInformation> = symbols
        .iter()
        .filter(|s| RELEVANT_SYMBOL_KINDS.contains(&s.kind))
        .take(WORKSPACE_SYMBOL_LIMIT)
        .collect();

    if filtered.is_empty() {
        return Ok(format!("No workspace symbols found for query '{query}'."));
    }

    let results: Vec<serde_json::Value> = filtered
        .iter()
        .map(|s| {
            let loc_str = format_location(&s.location);
            serde_json::json!({
                "name": s.name,
                "kind": format!("{:?}", s.kind),
                "location": loc_str,
            })
        })
        .collect();

    serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
}

pub(super) async fn go_to_implementation(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let (line, character) = args.position()?;
    let result = manager
        .implementation(file, line, character)
        .await
        .map_err(|e| e.to_string())?;

    match result {
        Some(loc) => {
            let result = LocationResult {
                file: uri_to_path(loc.uri.as_str())
                    .map(|p| p.display().to_string())
                    .unwrap_or_else(|| loc.uri.to_string()),
                line: loc.range.start.line + 1,
                character: loc.range.start.character + 1,
            };
            serde_json::to_string_pretty(&result).map_err(|e| e.to_string())
        }
        None => Ok("No implementation found at this position.".into()),
    }
}

pub(super) async fn prepare_call_hierarchy(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let (line, character) = args.position()?;
    let items = manager
        .prepare_call_hierarchy(file, line, character)
        .await
        .map_err(|e| e.to_string())?;

    if items.is_empty() {
        return Ok("No call hierarchy item at this position.".into());
    }

    let results: Vec<CallResult> = items
        .iter()
        .map(|item| CallResult {
            name: item.name.clone(),
            file: uri_to_path(item.uri.as_str())
                .map(|p| p.display().to_string())
                .unwrap_or_else(|| item.uri.to_string()),
            line: item.selection_range.start.line + 1,
            character: item.selection_range.start.character + 1,
        })
        .collect();

    serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
}

pub(super) async fn incoming_calls(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let (line, character) = args.position()?;
    let calls = manager
        .incoming_calls(file, line, character)
        .await
        .map_err(|e| e.to_string())?;

    if calls.is_empty() {
        return Ok("No incoming calls found.".into());
    }

    let results: Vec<CallResult> = calls
        .iter()
        .map(|call| CallResult {
            name: call.from.name.clone(),
            file: uri_to_path(call.from.uri.as_str())
                .map(|p| p.display().to_string())
                .unwrap_or_else(|| call.from.uri.to_string()),
            line: call.from.selection_range.start.line + 1,
            character: call.from.selection_range.start.character + 1,
        })
        .collect();

    serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
}

pub(super) async fn outgoing_calls(
    manager: &LspManager,
    args: &LspArgs,
    file: &Path,
) -> Result<String, String> {
    let (line, character) = args.position()?;
    let calls = manager
        .outgoing_calls(file, line, character)
        .await
        .map_err(|e| e.to_string())?;

    if calls.is_empty() {
        return Ok("No outgoing calls found.".into());
    }

    let results: Vec<CallResult> = calls
        .iter()
        .map(|call| CallResult {
            name: call.to.name.clone(),
            file: uri_to_path(call.to.uri.as_str())
                .map(|p| p.display().to_string())
                .unwrap_or_else(|| call.to.uri.to_string()),
            line: call.to.selection_range.start.line + 1,
            character: call.to.selection_range.start.character + 1,
        })
        .collect();

    serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
}

pub(super) async fn diagnostics(manager: &LspManager, file: &Path) -> Result<String, String> {
    let diags = manager.diagnostics(file).await.map_err(|e| e.to_string())?;

    if diags.is_empty() {
        return Ok("No diagnostics found.".into());
    }

    let results: Vec<DiagnosticResult> = diags
        .iter()
        .map(|d| DiagnosticResult {
            severity: match d.severity {
                Some(lsp_types::DiagnosticSeverity::ERROR) => "error".into(),
                Some(lsp_types::DiagnosticSeverity::WARNING) => "warning".into(),
                Some(lsp_types::DiagnosticSeverity::INFORMATION) => "info".into(),
                Some(lsp_types::DiagnosticSeverity::HINT) => "hint".into(),
                _ => "unknown".into(),
            },
            line: d.range.start.line + 1,
            character: d.range.start.character + 1,
            message: d.message.clone(),
            source: d.source.clone(),
        })
        .collect();

    serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
}
