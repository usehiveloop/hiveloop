use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use lsp_bridge::LspBridge;
use tokio::sync::{Notify, RwLock};
use tracing::{info, warn};

use crate::config::LspServerConfig;
use crate::server::{builtin_servers, ServerDef};

/// Manages LSP server lifecycle and routes operations to the correct server.
///
/// Wraps `lsp_bridge::LspBridge` and adds:
/// - Lazy server spawning (only when a matching file is first accessed)
/// - Extension-based routing to the correct server
/// - Multi-client fan-out (operations hit all matching servers)
/// - Document version tracking (re-opens send didChange)
/// - Spawn deduplication (concurrent spawns wait on each other)
/// - Broken server tracking (avoids repeated spawn attempts)
/// - Merging of built-in and user-defined server configurations
pub struct LspManager {
    pub(super) bridge: RwLock<LspBridge>,
    pub(super) servers: Vec<ServerDef>,
    /// Maps "server_id:root_path" -> bridge server ID
    pub(super) registered: RwLock<HashMap<String, String>>,
    /// Server IDs that failed to spawn and should not be retried
    pub(super) broken: RwLock<std::collections::HashSet<String>>,
    /// Document URI -> version counter for didOpen/didChange tracking
    pub(super) documents: RwLock<HashMap<String, u32>>,
    /// Spawn deduplication: reg_key -> Notify for waiters
    pub(super) spawning: RwLock<HashMap<String, Arc<Notify>>>,
    pub(super) project_root: PathBuf,
}

impl LspManager {
    /// Create a new `LspManager`.
    ///
    /// Merges built-in server definitions with any user-defined custom servers.
    /// User configs with the same ID as a built-in server override the built-in.
    pub fn new(
        project_root: PathBuf,
        custom_config: Option<HashMap<String, LspServerConfig>>,
    ) -> Self {
        let mut servers = builtin_servers();

        if let Some(custom) = custom_config {
            for (id, cfg) in custom {
                if cfg.disabled {
                    // Remove built-in if user disabled it
                    servers.retain(|s| s.id != id);
                    continue;
                }

                // Override or add
                if let Some(existing) = servers.iter_mut().find(|s| s.id == id) {
                    existing.command = cfg.command;
                    if !cfg.extensions.is_empty() {
                        existing.extensions = cfg.extensions;
                    }
                    existing.env = cfg.env;
                    existing.init_options = cfg.initialization_options;
                } else {
                    servers.push(ServerDef {
                        id: id.clone(),
                        command: cfg.command,
                        extensions: cfg.extensions,
                        root_markers: vec![],
                        env: cfg.env,
                        init_options: cfg.initialization_options,
                    });
                }
            }
        }

        Self {
            bridge: RwLock::new(LspBridge::new()),
            servers,
            registered: RwLock::new(HashMap::new()),
            broken: RwLock::new(std::collections::HashSet::new()),
            documents: RwLock::new(HashMap::new()),
            spawning: RwLock::new(HashMap::new()),
            project_root,
        }
    }

    /// Resolve a path that may be relative against the project root.
    pub(super) fn resolve_path(&self, path: &Path) -> PathBuf {
        if path.is_absolute() {
            path.to_path_buf()
        } else {
            self.project_root.join(path)
        }
    }

    /// Find server definitions that handle the given file extension.
    pub(super) fn servers_for_ext(&self, ext: &str) -> Vec<&ServerDef> {
        self.servers
            .iter()
            .filter(|s| s.extensions.iter().any(|e| e == ext))
            .collect()
    }

    /// Check if any server is available (or could be started) for the given file.
    pub fn has_server(&self, file: &Path) -> bool {
        let file = self.resolve_path(file);
        let ext = file.extension().and_then(|e| e.to_str()).unwrap_or("");
        !self.servers_for_ext(ext).is_empty()
    }

    /// Shut down all LSP servers.
    pub async fn shutdown(&self) {
        let mut bridge = self.bridge.write().await;
        if let Err(e) = bridge.shutdown().await {
            warn!(error = %e, "error shutting down LSP servers");
        }
        self.registered.write().await.clear();
        self.documents.write().await.clear();
        info!("LSP servers shut down");
    }
}
