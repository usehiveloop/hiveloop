use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use lsp_bridge::LspServerConfig as BridgeServerConfig;
use tokio::sync::Notify;
use tracing::{info, warn};

use super::core::LspManager;
use crate::error::LspError;
use crate::server::{find_root, ServerDef};

impl LspManager {
    /// Ensure ALL matching servers are running for the given file.
    /// Returns server IDs for all successfully started servers.
    pub(super) async fn ensure_servers(&self, file: &Path) -> Result<Vec<String>, LspError> {
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
                        spawn_errors.push(format!("{}: concurrent spawn attempt failed", def.id));
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
}
