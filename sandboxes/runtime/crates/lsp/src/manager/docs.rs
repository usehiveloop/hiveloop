use std::path::Path;

use futures::future::join_all;
use lsp_types::{Hover, Location, Position};
use tracing::warn;

use super::core::LspManager;
use super::uri::path_to_uri;
use crate::error::LspError;

impl LspManager {
    /// Open a document in the appropriate LSP server(s).
    /// On first open, sends didOpen. On subsequent opens, re-reads from disk
    /// and sends didChange to keep the server in sync.
    pub async fn open_document(&self, file: &Path) -> Result<(), LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);
        let content = tokio::fs::read_to_string(&file)
            .await
            .map_err(|e| LspError::FileNotFound(format!("{}: {}", file.display(), e)))?;

        let bridge = self.bridge.read().await;

        let is_reopen = {
            let mut docs = self.documents.write().await;
            if let Some(v) = docs.get_mut(&uri) {
                *v += 1;
                true
            } else {
                docs.insert(uri.clone(), 0);
                false
            }
        };

        for server_id in &server_ids {
            let result = if is_reopen {
                bridge.update_document(server_id, &uri, &content).await
            } else {
                bridge.open_document(server_id, &uri, &content).await
            };

            if let Err(e) = result {
                warn!(server = %server_id, error = %e, "failed to open/update document");
            }
        }

        Ok(())
    }

    /// Get hover information at the given position.
    /// Fans out to all matching servers, returns first result.
    pub async fn hover(
        &self,
        file: &Path,
        line: u32,
        character: u32,
    ) -> Result<Option<Hover>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);
        let position = Position::new(line, character);

        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.get_hover(sid, &uri, position))
            .collect();

        let results = join_all(futs).await;
        for result in results {
            match result {
                Ok(Some(hover)) => return Ok(Some(hover)),
                Ok(None) => {}
                Err(e) => warn!(error = %e, "hover failed on one server"),
            }
        }
        Ok(None)
    }

    /// Go to definition at the given position.
    /// Fans out to all matching servers, returns first result.
    pub async fn definition(
        &self,
        file: &Path,
        line: u32,
        character: u32,
    ) -> Result<Option<Location>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);
        let position = Position::new(line, character);

        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.go_to_definition(sid, &uri, position))
            .collect();

        let results = join_all(futs).await;
        for result in results {
            match result {
                Ok(Some(loc)) => return Ok(Some(loc)),
                Ok(None) => {}
                Err(e) => warn!(error = %e, "definition failed on one server"),
            }
        }
        Ok(None)
    }

    /// Find all references at the given position.
    /// Fans out to all matching servers, merges results.
    pub async fn references(
        &self,
        file: &Path,
        line: u32,
        character: u32,
    ) -> Result<Vec<Location>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);
        let position = Position::new(line, character);

        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.find_references(sid, &uri, position))
            .collect();

        let results = join_all(futs).await;
        let mut all_refs = Vec::new();
        for result in results {
            match result {
                Ok(refs) => all_refs.extend(refs),
                Err(e) => warn!(error = %e, "references failed on one server"),
            }
        }
        Ok(all_refs)
    }

    /// Go to implementation at the given position.
    /// Fans out to all matching servers, returns first result.
    pub async fn implementation(
        &self,
        file: &Path,
        line: u32,
        character: u32,
    ) -> Result<Option<Location>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);
        let position = Position::new(line, character);

        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.get_implementation(sid, &uri, position))
            .collect();

        let results = join_all(futs).await;
        for result in results {
            match result {
                Ok(Some(loc)) => return Ok(Some(loc)),
                Ok(None) => {}
                Err(e) => warn!(error = %e, "implementation failed on one server"),
            }
        }
        Ok(None)
    }
}
