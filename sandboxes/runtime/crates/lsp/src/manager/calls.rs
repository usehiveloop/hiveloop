use std::path::Path;

use futures::future::join_all;
use lsp_types::{
    CallHierarchyIncomingCall, CallHierarchyItem, CallHierarchyOutgoingCall, Position,
};
use tracing::warn;

use super::core::LspManager;
use super::uri::path_to_uri;
use crate::error::LspError;

impl LspManager {
    /// Prepare call hierarchy at the given position.
    pub async fn prepare_call_hierarchy(
        &self,
        file: &Path,
        line: u32,
        character: u32,
    ) -> Result<Vec<CallHierarchyItem>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);
        let position = Position::new(line, character);

        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.prepare_call_hierarchy(sid, &uri, position))
            .collect();

        let results = join_all(futs).await;
        for result in results {
            match result {
                Ok(items) if !items.is_empty() => return Ok(items),
                Ok(_) => {}
                Err(e) => warn!(error = %e, "prepare_call_hierarchy failed on one server"),
            }
        }
        Ok(vec![])
    }

    /// Get incoming calls for a call hierarchy item.
    pub async fn incoming_calls(
        &self,
        file: &Path,
        line: u32,
        character: u32,
    ) -> Result<Vec<CallHierarchyIncomingCall>, LspError> {
        let file_resolved = self.resolve_path(file);
        let items = self
            .prepare_call_hierarchy(&file_resolved, line, character)
            .await?;
        let item = items.into_iter().next().ok_or_else(|| {
            LspError::OperationFailed("no call hierarchy item at position".into())
        })?;

        let server_ids = self.ensure_servers(&file_resolved).await?;
        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.get_incoming_calls(sid, item.clone()))
            .collect();

        let results = join_all(futs).await;
        let mut all_calls = Vec::new();
        for result in results {
            match result {
                Ok(calls) => all_calls.extend(calls),
                Err(e) => warn!(error = %e, "incoming_calls failed on one server"),
            }
        }
        Ok(all_calls)
    }

    /// Get outgoing calls for a call hierarchy item.
    pub async fn outgoing_calls(
        &self,
        file: &Path,
        line: u32,
        character: u32,
    ) -> Result<Vec<CallHierarchyOutgoingCall>, LspError> {
        let file_resolved = self.resolve_path(file);
        let items = self
            .prepare_call_hierarchy(&file_resolved, line, character)
            .await?;
        let item = items.into_iter().next().ok_or_else(|| {
            LspError::OperationFailed("no call hierarchy item at position".into())
        })?;

        let server_ids = self.ensure_servers(&file_resolved).await?;
        let bridge = self.bridge.read().await;

        let futs: Vec<_> = server_ids
            .iter()
            .map(|sid| bridge.get_outgoing_calls(sid, item.clone()))
            .collect();

        let results = join_all(futs).await;
        let mut all_calls = Vec::new();
        for result in results {
            match result {
                Ok(calls) => all_calls.extend(calls),
                Err(e) => warn!(error = %e, "outgoing_calls failed on one server"),
            }
        }
        Ok(all_calls)
    }
}
