use std::sync::Arc;

use async_trait::async_trait;
use lsp::LspManager;

use crate::registry::ToolExecutor;

mod ops;
mod types;

pub use types::{LspArgs, LspOperation};

/// Tool that exposes LSP operations to the LLM.
pub struct LspTool {
    manager: Arc<LspManager>,
}

impl LspTool {
    pub fn new(manager: Arc<LspManager>) -> Self {
        Self { manager }
    }
}

#[async_trait]
impl ToolExecutor for LspTool {
    fn name(&self) -> &str {
        "lsp"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/lsp.txt")
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
                ops::go_to_definition(&self.manager, &args, &file).await
            }
            LspOperation::FindReferences => ops::find_references(&self.manager, &args, &file).await,
            LspOperation::Hover => ops::hover(&self.manager, &args, &file).await,
            LspOperation::DocumentSymbol => ops::document_symbol(&self.manager, &file).await,
            LspOperation::WorkspaceSymbol => {
                ops::workspace_symbol(&self.manager, &args, &file).await
            }
            LspOperation::GoToImplementation => {
                ops::go_to_implementation(&self.manager, &args, &file).await
            }
            LspOperation::PrepareCallHierarchy => {
                ops::prepare_call_hierarchy(&self.manager, &args, &file).await
            }
            LspOperation::IncomingCalls => ops::incoming_calls(&self.manager, &args, &file).await,
            LspOperation::OutgoingCalls => ops::outgoing_calls(&self.manager, &args, &file).await,
            LspOperation::Diagnostics => ops::diagnostics(&self.manager, &file).await,
        }
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
