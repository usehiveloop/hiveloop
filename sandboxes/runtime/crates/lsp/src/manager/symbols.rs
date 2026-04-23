use std::path::Path;

use futures::future::join_all;
use lsp_types::{DocumentSymbol, SymbolInformation};
use tracing::warn;

use super::core::LspManager;
use super::uri::path_to_uri;
use crate::error::LspError;

impl LspManager {
    /// Get document symbols.
    /// Fans out to all matching servers, returns first non-empty result.
    pub async fn document_symbols(&self, file: &Path) -> Result<Vec<DocumentSymbol>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);

        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.get_document_symbols(sid, &uri))
            .collect();

        let results = join_all(futs).await;
        for result in results {
            match result {
                Ok(symbols) if !symbols.is_empty() => return Ok(symbols),
                Ok(_) => {}
                Err(e) => warn!(error = %e, "document_symbols failed on one server"),
            }
        }
        Ok(vec![])
    }

    /// Search workspace symbols.
    /// Fans out to all matching servers, merges results.
    pub async fn workspace_symbols(
        &self,
        file: &Path,
        query: &str,
    ) -> Result<Vec<SymbolInformation>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;

        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.get_workspace_symbols(sid, query))
            .collect();

        let results = join_all(futs).await;
        let mut all_symbols = Vec::new();
        for result in results {
            match result {
                Ok(symbols) => all_symbols.extend(symbols),
                Err(e) => warn!(error = %e, "workspace_symbols failed on one server"),
            }
        }
        Ok(all_symbols)
    }
}
