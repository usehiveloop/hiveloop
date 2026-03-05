use std::path::{Path, PathBuf};
use std::sync::Arc;

use async_trait::async_trait;
use lsp::manager::{format_location, uri_to_path};
use lsp::LspManager;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::registry::ToolExecutor;

/// Tool that exposes LSP operations to the LLM.
pub struct LspTool {
    manager: Arc<LspManager>,
}

impl LspTool {
    pub fn new(manager: Arc<LspManager>) -> Self {
        Self { manager }
    }
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct LspArgs {
    /// The LSP operation to perform
    pub operation: LspOperation,
    /// Path to the file (absolute or relative to project root)
    pub file_path: String,
    /// 1-based line number (required for position-based operations)
    #[serde(default)]
    pub line: Option<u32>,
    /// 1-based character/column number (required for position-based operations)
    #[serde(default)]
    pub character: Option<u32>,
    /// Search query (for workspaceSymbol operation)
    #[serde(default)]
    pub query: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub enum LspOperation {
    GoToDefinition,
    FindReferences,
    Hover,
    DocumentSymbol,
    WorkspaceSymbol,
    GoToImplementation,
    PrepareCallHierarchy,
    IncomingCalls,
    OutgoingCalls,
    Diagnostics,
}

/// Result types for JSON serialization
#[derive(Serialize)]
struct LocationResult {
    file: String,
    line: u32,
    character: u32,
}

#[derive(Serialize)]
struct SymbolResult {
    name: String,
    kind: String,
    range: RangeResult,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    children: Vec<SymbolResult>,
}

#[derive(Serialize)]
struct RangeResult {
    start_line: u32,
    start_character: u32,
    end_line: u32,
    end_character: u32,
}

#[derive(Serialize)]
struct CallResult {
    name: String,
    file: String,
    line: u32,
    character: u32,
}

#[derive(Serialize)]
struct DiagnosticResult {
    severity: String,
    line: u32,
    character: u32,
    message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    source: Option<String>,
}

/// Relevant symbol kinds for workspace symbol filtering.
const RELEVANT_SYMBOL_KINDS: &[lsp_types::SymbolKind] = &[
    lsp_types::SymbolKind::CLASS,
    lsp_types::SymbolKind::FUNCTION,
    lsp_types::SymbolKind::METHOD,
    lsp_types::SymbolKind::INTERFACE,
    lsp_types::SymbolKind::VARIABLE,
    lsp_types::SymbolKind::CONSTANT,
    lsp_types::SymbolKind::STRUCT,
    lsp_types::SymbolKind::ENUM,
];

/// Maximum number of workspace symbol results.
const WORKSPACE_SYMBOL_LIMIT: usize = 10;

impl LspArgs {
    /// Get 0-based line/character, converting from 1-based input.
    fn position(&self) -> Result<(u32, u32), String> {
        let line = self.line.ok_or("'line' is required for this operation")?;
        let character = self
            .character
            .ok_or("'character' is required for this operation")?;

        if line == 0 {
            return Err("'line' must be >= 1 (1-based)".into());
        }
        if character == 0 {
            return Err("'character' must be >= 1 (1-based)".into());
        }

        Ok((line - 1, character - 1))
    }

    /// Resolve file_path: if relative, resolve against cwd.
    fn resolve_file_path(&self) -> PathBuf {
        let path = Path::new(&self.file_path);
        if path.is_absolute() {
            path.to_path_buf()
        } else {
            std::env::current_dir().unwrap_or_default().join(path)
        }
    }
}

#[async_trait]
impl ToolExecutor for LspTool {
    fn name(&self) -> &str {
        "lsp"
    }

    fn description(&self) -> &str {
        include_str!("instructions/lsp.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(LspArgs)).unwrap_or_default()
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: LspArgs =
            serde_json::from_value(args).map_err(|e| format!("invalid arguments: {e}"))?;

        let file = args.resolve_file_path();

        // Validate file exists
        if !file.exists() {
            return Err(format!("file not found: {}", file.display()));
        }

        // Check if we have a server for this file type
        if !self.manager.has_server(&file) {
            return Err(format!(
                "no LSP server available for file type: {}",
                file.extension()
                    .and_then(|e| e.to_str())
                    .unwrap_or("unknown")
            ));
        }

        // Open the document first (sends didOpen on first call, didChange on subsequent)
        self.manager
            .open_document(&file)
            .await
            .map_err(|e| e.to_string())?;

        match args.operation {
            LspOperation::GoToDefinition => {
                let (line, character) = args.position()?;
                let result = self
                    .manager
                    .definition(&file, line, character)
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

            LspOperation::FindReferences => {
                let (line, character) = args.position()?;
                let locs = self
                    .manager
                    .references(&file, line, character)
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

            LspOperation::Hover => {
                let (line, character) = args.position()?;
                let result = self
                    .manager
                    .hover(&file, line, character)
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

            LspOperation::DocumentSymbol => {
                let symbols = self
                    .manager
                    .document_symbols(&file)
                    .await
                    .map_err(|e| e.to_string())?;

                if symbols.is_empty() {
                    return Ok("No symbols found in document.".into());
                }

                let results: Vec<SymbolResult> =
                    symbols.into_iter().map(convert_document_symbol).collect();

                serde_json::to_string_pretty(&results).map_err(|e| e.to_string())
            }

            LspOperation::WorkspaceSymbol => {
                let query = args.query.as_deref().unwrap_or("");
                let symbols = self
                    .manager
                    .workspace_symbols(&file, query)
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

            LspOperation::GoToImplementation => {
                let (line, character) = args.position()?;
                let result = self
                    .manager
                    .implementation(&file, line, character)
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

            LspOperation::PrepareCallHierarchy => {
                let (line, character) = args.position()?;
                let items = self
                    .manager
                    .prepare_call_hierarchy(&file, line, character)
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

            LspOperation::IncomingCalls => {
                let (line, character) = args.position()?;
                let calls = self
                    .manager
                    .incoming_calls(&file, line, character)
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

            LspOperation::OutgoingCalls => {
                let (line, character) = args.position()?;
                let calls = self
                    .manager
                    .outgoing_calls(&file, line, character)
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

            LspOperation::Diagnostics => {
                let diags = self
                    .manager
                    .diagnostics(&file)
                    .await
                    .map_err(|e| e.to_string())?;

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
        }
    }
}

fn markup_content_to_string(mc: lsp_types::MarkedString) -> String {
    match mc {
        lsp_types::MarkedString::String(s) => s,
        lsp_types::MarkedString::LanguageString(ls) => {
            format!("```{}\n{}\n```", ls.language, ls.value)
        }
    }
}

fn convert_document_symbol(sym: lsp_types::DocumentSymbol) -> SymbolResult {
    let children = sym
        .children
        .unwrap_or_default()
        .into_iter()
        .map(convert_document_symbol)
        .collect();

    SymbolResult {
        name: sym.name,
        kind: format!("{:?}", sym.kind),
        range: RangeResult {
            start_line: sym.range.start.line + 1,
            start_character: sym.range.start.character + 1,
            end_line: sym.range.end.line + 1,
            end_character: sym.range.end.character + 1,
        },
        children,
    }
}
