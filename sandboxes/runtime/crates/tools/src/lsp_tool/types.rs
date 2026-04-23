use std::path::{Path, PathBuf};

use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

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
pub(super) struct LocationResult {
    pub file: String,
    pub line: u32,
    pub character: u32,
}

#[derive(Serialize)]
pub(super) struct SymbolResult {
    pub name: String,
    pub kind: String,
    pub range: RangeResult,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub children: Vec<SymbolResult>,
}

#[derive(Serialize)]
pub(super) struct RangeResult {
    pub start_line: u32,
    pub start_character: u32,
    pub end_line: u32,
    pub end_character: u32,
}

#[derive(Serialize)]
pub(super) struct CallResult {
    pub name: String,
    pub file: String,
    pub line: u32,
    pub character: u32,
}

#[derive(Serialize)]
pub(super) struct DiagnosticResult {
    pub severity: String,
    pub line: u32,
    pub character: u32,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub source: Option<String>,
}

/// Relevant symbol kinds for workspace symbol filtering.
pub(super) const RELEVANT_SYMBOL_KINDS: &[lsp_types::SymbolKind] = &[
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
pub(super) const WORKSPACE_SYMBOL_LIMIT: usize = 10;

impl LspArgs {
    /// Get 0-based line/character, converting from 1-based input.
    pub(super) fn position(&self) -> Result<(u32, u32), String> {
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
    pub(super) fn resolve_file_path(&self) -> PathBuf {
        let path = Path::new(&self.file_path);
        if path.is_absolute() {
            path.to_path_buf()
        } else {
            std::env::current_dir().unwrap_or_default().join(path)
        }
    }
}

pub(super) fn markup_content_to_string(mc: lsp_types::MarkedString) -> String {
    match mc {
        lsp_types::MarkedString::String(s) => s,
        lsp_types::MarkedString::LanguageString(ls) => {
            format!("```{}\n{}\n```", ls.language, ls.value)
        }
    }
}

pub(super) fn convert_document_symbol(sym: lsp_types::DocumentSymbol) -> SymbolResult {
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
