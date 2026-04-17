use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use futures::future::join_all;
use lsp_bridge::LspBridge;
use lsp_bridge::LspServerConfig as BridgeServerConfig;
use lsp_types::*;
use tokio::sync::{Notify, RwLock};
use tracing::{info, warn};

use crate::config::LspServerConfig;
use crate::error::LspError;
use crate::server::{builtin_servers, find_root, ServerDef};

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
    bridge: RwLock<LspBridge>,
    servers: Vec<ServerDef>,
    /// Maps "server_id:root_path" -> bridge server ID
    registered: RwLock<HashMap<String, String>>,
    /// Server IDs that failed to spawn and should not be retried
    broken: RwLock<std::collections::HashSet<String>>,
    /// Document URI -> version counter for didOpen/didChange tracking
    documents: RwLock<HashMap<String, u32>>,
    /// Spawn deduplication: reg_key -> Notify for waiters
    spawning: RwLock<HashMap<String, Arc<Notify>>>,
    project_root: PathBuf,
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
    fn resolve_path(&self, path: &Path) -> PathBuf {
        if path.is_absolute() {
            path.to_path_buf()
        } else {
            self.project_root.join(path)
        }
    }

    /// Find server definitions that handle the given file extension.
    fn servers_for_ext(&self, ext: &str) -> Vec<&ServerDef> {
        self.servers
            .iter()
            .filter(|s| s.extensions.iter().any(|e| e == ext))
            .collect()
    }

    /// Ensure ALL matching servers are running for the given file.
    /// Returns server IDs for all successfully started servers.
    async fn ensure_servers(&self, file: &Path) -> Result<Vec<String>, LspError> {
        let ext = file.extension().and_then(|e| e.to_str()).unwrap_or("");

        let defs = self.servers_for_ext(ext);
        if defs.is_empty() {
            return Err(LspError::NoServerForExtension {
                ext: ext.to_string(),
                path: file.display().to_string(),
            });
        }

        let mut server_ids = Vec::new();
        let mut spawn_errors: Vec<String> = Vec::new();

        for def in &defs {
            let root =
                find_root(file, &def.root_markers).unwrap_or_else(|| self.project_root.clone());

            let reg_key = format!("{}:{}", def.id, root.display());

            // Already registered?
            {
                let registered = self.registered.read().await;
                if let Some(server_id) = registered.get(&reg_key) {
                    server_ids.push(server_id.clone());
                    continue;
                }
            }

            // Known broken?
            {
                let broken = self.broken.read().await;
                if broken.contains(&reg_key) {
                    spawn_errors.push(format!("{}: previously failed to start", def.id));
                    continue;
                }
            }

            // Spawn deduplication: check if another task is already spawning this server
            {
                let spawning = self.spawning.read().await;
                if let Some(notify) = spawning.get(&reg_key) {
                    let notify = notify.clone();
                    drop(spawning);
                    notify.notified().await;
                    // After notification, check if server is now registered
                    let registered = self.registered.read().await;
                    if let Some(server_id) = registered.get(&reg_key) {
                        server_ids.push(server_id.clone());
                    } else {
                        spawn_errors
                            .push(format!("{}: concurrent spawn attempt failed", def.id));
                    }
                    continue;
                }
            }

            // Insert spawn lock
            let notify = Arc::new(Notify::new());
            self.spawning
                .write()
                .await
                .insert(reg_key.clone(), notify.clone());

            let result = self.spawn_server(def, &root, &reg_key).await;

            // Remove spawn lock and notify waiters
            self.spawning.write().await.remove(&reg_key);
            notify.notify_waiters();

            match result {
                Ok(server_id) => server_ids.push(server_id),
                Err(e) => spawn_errors.push(format!("{}: {}", def.id, e)),
            }
        }

        if server_ids.is_empty() {
            let reason = if spawn_errors.is_empty() {
                "no spawn attempts made".to_string()
            } else {
                spawn_errors.join("; ")
            };
            return Err(LspError::AllSpawnsFailed {
                path: file.display().to_string(),
                reason,
            });
        }

        Ok(server_ids)
    }

    /// Spawn a single LSP server. Returns the bridge server ID on success.
    async fn spawn_server(
        &self,
        def: &ServerDef,
        root: &Path,
        reg_key: &str,
    ) -> Result<String, LspError> {
        // Check binary exists
        let binary = def
            .command
            .first()
            .ok_or_else(|| LspError::Config(format!("server '{}' has empty command", def.id)))?;

        if which::which(binary).is_err() {
            warn!(server = %def.id, binary = %binary, "LSP server binary not found");
            self.broken.write().await.insert(reg_key.to_string());
            return Err(LspError::BinaryNotFound {
                binary: binary.clone(),
            });
        }

        // Build bridge config
        let mut bridge_config = BridgeServerConfig::new()
            .command(binary)
            .root_path(root)
            .startup_timeout(Duration::from_secs(30))
            .request_timeout(Duration::from_secs(30));

        // Add args (skip first element which is the binary)
        for arg in def.command.iter().skip(1) {
            bridge_config = bridge_config.arg(arg);
        }

        // Add environment variables
        for (k, v) in &def.env {
            bridge_config = bridge_config.env(k, v);
        }

        // Add initialization options
        if let Some(ref opts) = def.init_options {
            bridge_config = bridge_config.initialization_options(opts.clone());
        }

        // Register and start
        let server_id = format!("{}-{}", def.id, root.display());
        let mut bridge = self.bridge.write().await;

        match bridge.register_server(&server_id, bridge_config).await {
            Ok(_) => {}
            Err(e) => {
                warn!(server = %def.id, error = %e, "failed to register LSP server");
                self.broken.write().await.insert(reg_key.to_string());
                return Err(LspError::SpawnFailed {
                    server: def.id.clone(),
                    reason: e.to_string(),
                });
            }
        }

        match bridge.start_server(&server_id).await {
            Ok(_) => {
                info!(server = %def.id, root = %root.display(), "LSP server started");
                // Wait for server to be ready
                if let Err(e) = bridge.wait_server_ready(&server_id).await {
                    warn!(server = %def.id, error = %e, "LSP server failed to become ready");
                    let _ = bridge.stop_server(&server_id).await;
                    self.broken.write().await.insert(reg_key.to_string());
                    return Err(LspError::SpawnFailed {
                        server: def.id.clone(),
                        reason: e.to_string(),
                    });
                }
                self.registered
                    .write()
                    .await
                    .insert(reg_key.to_string(), server_id.clone());
                Ok(server_id)
            }
            Err(e) => {
                warn!(server = %def.id, error = %e, "failed to start LSP server");
                self.broken.write().await.insert(reg_key.to_string());
                Err(LspError::SpawnFailed {
                    server: def.id.clone(),
                    reason: e.to_string(),
                })
            }
        }
    }

    /// Check if any server is available (or could be started) for the given file.
    pub fn has_server(&self, file: &Path) -> bool {
        let file = self.resolve_path(file);
        let ext = file.extension().and_then(|e| e.to_str()).unwrap_or("");
        !self.servers_for_ext(ext).is_empty()
    }

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

/// Convert a file path to a `file://` URI.
fn path_to_uri(path: &Path) -> String {
    let abs = if path.is_absolute() {
        path.to_path_buf()
    } else {
        std::env::current_dir().unwrap_or_default().join(path)
    };
    format!("file://{}", abs.display())
}

/// Convert a `file://` URI back to a file path.
pub fn uri_to_path(uri: &str) -> Option<PathBuf> {
    uri.strip_prefix("file://").map(PathBuf::from)
}

/// Format a Location as a human-readable string.
pub fn format_location(loc: &Location) -> String {
    let path = uri_to_path(loc.uri.as_str())
        .map(|p| p.display().to_string())
        .unwrap_or_else(|| loc.uri.to_string());
    format!(
        "{}:{}:{}",
        path,
        loc.range.start.line + 1,
        loc.range.start.character + 1,
    )
}
