use std::path::Path;

use lsp_types::Diagnostic;
use tracing::warn;

use super::core::LspManager;
use super::uri::path_to_uri;
use crate::error::LspError;

impl LspManager {
    /// Get diagnostics for a file from all matching LSP servers.
    pub async fn diagnostics(&self, file: &Path) -> Result<Vec<Diagnostic>, LspError> {
        let file = self.resolve_path(file);
        let server_ids = self.ensure_servers(&file).await?;
        let uri = path_to_uri(&file);

        let bridge = self.bridge.read().await;

        let mut all_diags = Vec::new();
        for sid in &server_ids {
            match bridge.get_diagnostics(sid, &uri) {
                Ok(diags) => all_diags.extend(diags),
                Err(e) => warn!(error = %e, "diagnostics failed on one server"),
            }
        }
        Ok(all_diags)
    }
}
