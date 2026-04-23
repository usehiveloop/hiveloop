use std::collections::HashMap;
use std::process::Stdio;
use tracing::{debug, info, warn};

use super::methods::{InstallMethod, InstallableServer};
use super::servers::installable_servers;

/// LSP Installer handles installation of language servers
pub struct LspInstaller {
    servers: HashMap<String, InstallableServer>,
}

impl LspInstaller {
    /// Create a new installer with all available servers
    pub fn new() -> Self {
        let servers: HashMap<String, InstallableServer> = installable_servers()
            .into_iter()
            .map(|s| (s.id.clone(), s))
            .collect();
        Self { servers }
    }

    /// Get list of all installable server IDs
    pub fn available_servers(&self) -> Vec<String> {
        self.servers.keys().cloned().collect()
    }

    /// Check if a binary exists in PATH
    async fn binary_exists(&self, binary: &str) -> bool {
        match tokio::process::Command::new("which")
            .arg(binary)
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .await
        {
            Ok(status) => status.success(),
            Err(_) => false,
        }
    }

    /// Install a single server by ID
    async fn install_server(&self, server_id: &str) -> Result<(), String> {
        let server = self
            .servers
            .get(server_id)
            .ok_or_else(|| format!("Unknown LSP server: {}", server_id))?;

        // Check if already installed
        for binary in &server.binaries {
            if self.binary_exists(binary).await {
                info!(server = %server_id, binary = %binary, "already installed, skipping");
                return Ok(());
            }
        }

        info!(server = %server_id, method = ?server.method, "installing LSP server");

        let result = match &server.method {
            InstallMethod::Npm { package } => self.install_npm(package).await,
            InstallMethod::Cargo { crate_name } => self.install_cargo(crate_name).await,
            InstallMethod::Go { path } => self.install_go(path).await,
            InstallMethod::Pip { package } => self.install_pip(package).await,
            InstallMethod::Custom { command, args } => self.install_custom(command, args).await,
        };

        match result {
            Ok(_) => {
                info!(server = %server_id, "installation complete");
                Ok(())
            }
            Err(e) => {
                // Downgraded from error! → warn!: a single missing toolchain
                // (opam, dotnet, gem, ...) should not make `bridge install-lsp
                // all` look catastrophic. The CLI surfaces a summary at the
                // end and always exits 0; the operator can install the
                // underlying toolchain and re-run for the specific id.
                warn!(server = %server_id, error = %e, "installation failed");
                Err(e)
            }
        }
    }

    /// Install servers by IDs (or "all" for all servers)
    pub async fn install(&self, server_ids: &[String]) -> HashMap<String, Result<(), String>> {
        let ids_to_install: Vec<String> = if server_ids.contains(&"all".to_string()) {
            self.available_servers()
        } else {
            server_ids.to_vec()
        };

        let mut results = HashMap::new();

        for id in ids_to_install {
            let result = self.install_server(&id).await;
            results.insert(id, result);
        }

        results
    }

    /// Run an installer command, capturing stderr and surfacing it on failure.
    async fn run_install_cmd(
        &self,
        program: &str,
        args: &[&str],
        label: &str,
    ) -> Result<(), String> {
        let output = tokio::process::Command::new(program)
            .args(args)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .output()
            .await
            .map_err(|e| format!("Failed to run {}: {}", program, e))?;

        if output.status.success() {
            Ok(())
        } else {
            let stderr = String::from_utf8_lossy(&output.stderr);
            let tail: String = stderr
                .lines()
                .rev()
                .take(10)
                .collect::<Vec<_>>()
                .into_iter()
                .rev()
                .collect::<Vec<_>>()
                .join(" | ");
            let tail = if tail.is_empty() {
                String::from_utf8_lossy(&output.stdout)
                    .lines()
                    .rev()
                    .take(5)
                    .collect::<Vec<_>>()
                    .into_iter()
                    .rev()
                    .collect::<Vec<_>>()
                    .join(" | ")
            } else {
                tail
            };
            Err(format!(
                "{} install failed for {}: {}",
                program,
                label,
                tail.trim()
            ))
        }
    }

    /// Install npm package globally
    async fn install_npm(&self, package: &str) -> Result<(), String> {
        debug!(package = %package, "running npm install");
        self.run_install_cmd("npm", &["install", "-g", package], package)
            .await
    }

    /// Install cargo crate
    async fn install_cargo(&self, crate_name: &str) -> Result<(), String> {
        debug!(crate_name = %crate_name, "running cargo install");
        self.run_install_cmd("cargo", &["install", crate_name], crate_name)
            .await
    }

    /// Install go package
    async fn install_go(&self, path: &str) -> Result<(), String> {
        debug!(path = %path, "running go install");
        self.run_install_cmd("go", &["install", path], path).await
    }

    /// Install pip package. Uses `python3 -m pip install --user
    /// --break-system-packages` because (a) bare `pip` is missing on modern
    /// systems, (b) PEP 668-marked distros (Homebrew Python, recent Debian)
    /// reject `pip install` without the explicit override.
    async fn install_pip(&self, package: &str) -> Result<(), String> {
        debug!(package = %package, "running python3 -m pip install");
        self.run_install_cmd(
            "python3",
            &[
                "-m",
                "pip",
                "install",
                "--user",
                "--break-system-packages",
                package,
            ],
            package,
        )
        .await
    }

    /// Run custom install command
    async fn install_custom(&self, command: &str, args: &[String]) -> Result<(), String> {
        debug!(command = %command, args = ?args, "running custom install");
        let status = tokio::process::Command::new(command)
            .args(args)
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .status()
            .await
            .map_err(|e| format!("Failed to run {}: {}", command, e))?;

        if status.success() {
            Ok(())
        } else {
            Err(format!("custom install command failed: {}", command))
        }
    }
}

impl Default for LspInstaller {
    fn default() -> Self {
        Self::new()
    }
}
