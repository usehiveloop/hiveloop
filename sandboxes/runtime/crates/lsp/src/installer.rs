//! LSP Server Installer
//!
//! Handles installation of LSP servers via various package managers.
//! Runs asynchronously in the background when bridge starts with --install-lsp-servers.

use std::collections::HashMap;
use std::process::Stdio;
use tracing::{debug, error, info};

/// Installation method for an LSP server
#[derive(Debug, Clone)]
pub enum InstallMethod {
    /// Install via npm: `npm install -g <package>`
    Npm { package: String },
    /// Install via cargo: `cargo install <crate>`
    Cargo { crate_name: String },
    /// Install via go: `go install <path>@latest`
    Go { path: String },
    /// Install via gem: `gem install <gem>`
    Gem { gem: String },
    /// Install via pip: `pip install <package>`
    Pip { package: String },
    /// Install via luarocks: `luarocks install <rock>`
    LuaRocks { rock: String },
    /// Install via opam: `opam install <package>`
    Opam { package: String },
    /// Install via stack: `stack install <package>`
    Stack { package: String },
    /// Custom install command
    Custom { command: String, args: Vec<String> },
}

/// Information about an installable LSP server
#[derive(Debug, Clone)]
pub struct InstallableServer {
    /// Server ID (e.g., "typescript", "rust")
    pub id: String,
    /// Installation method
    pub method: InstallMethod,
    /// Binary name(s) to check if already installed
    pub binaries: Vec<String>,
    /// Description of the server
    pub description: String,
}

/// Returns all installable LSP servers
pub fn installable_servers() -> Vec<InstallableServer> {
    vec![
        // JavaScript/TypeScript
        InstallableServer {
            id: "typescript".to_string(),
            method: InstallMethod::Npm {
                package: "typescript-language-server".to_string(),
            },
            binaries: vec!["typescript-language-server".to_string()],
            description: "TypeScript/JavaScript language server".to_string(),
        },
        InstallableServer {
            id: "eslint".to_string(),
            method: InstallMethod::Npm {
                package: "eslint".to_string(),
            },
            binaries: vec!["eslint".to_string()],
            description: "ESLint LSP server".to_string(),
        },
        InstallableServer {
            id: "biome".to_string(),
            method: InstallMethod::Npm {
                package: "@biomejs/biome".to_string(),
            },
            binaries: vec!["biome".to_string()],
            description: "Biome LSP server for JS/TS/JSON/CSS".to_string(),
        },
        // Web frameworks
        InstallableServer {
            id: "vue".to_string(),
            method: InstallMethod::Npm {
                package: "@vue/language-server".to_string(),
            },
            binaries: vec!["vue-language-server".to_string()],
            description: "Vue language server".to_string(),
        },
        InstallableServer {
            id: "svelte".to_string(),
            method: InstallMethod::Npm {
                package: "svelte-language-server".to_string(),
            },
            binaries: vec!["svelteserver".to_string()],
            description: "Svelte language server".to_string(),
        },
        InstallableServer {
            id: "astro".to_string(),
            method: InstallMethod::Npm {
                package: "@astrojs/language-server".to_string(),
            },
            binaries: vec!["astro-ls".to_string()],
            description: "Astro language server".to_string(),
        },
        // Rust — download the prebuilt rust-analyzer binary. `cargo install`
        // builds from source and takes ~10 minutes; the release binary is a
        // few-megabyte download.
        InstallableServer {
            id: "rust".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec![
                    "-c".to_string(),
                    "set -eu; mkdir -p ~/.local/bin; \
                     curl -fsSL https://github.com/rust-lang/rust-analyzer/releases/latest/download/rust-analyzer-x86_64-unknown-linux-gnu.gz \
                       | gunzip > ~/.local/bin/rust-analyzer; \
                     chmod +x ~/.local/bin/rust-analyzer".to_string(),
                ],
            },
            binaries: vec!["rust-analyzer".to_string()],
            description: "Rust analyzer".to_string(),
        },
        // Go
        InstallableServer {
            id: "go".to_string(),
            method: InstallMethod::Go {
                path: "golang.org/x/tools/gopls@latest".to_string(),
            },
            binaries: vec!["gopls".to_string()],
            description: "Go language server".to_string(),
        },
        // Python
        InstallableServer {
            id: "python".to_string(),
            method: InstallMethod::Npm {
                package: "pyright".to_string(),
            },
            binaries: vec!["pyright-langserver".to_string()],
            description: "Pyright language server".to_string(),
        },
        // Ruby — the real LSP is the `ruby-lsp` gem (Shopify). `rubocop --lsp`
        // is a linter-only sidecar that doesn't provide document-symbol or
        // navigation operations.
        InstallableServer {
            id: "ruby-lsp".to_string(),
            method: InstallMethod::Gem {
                gem: "ruby-lsp".to_string(),
            },
            binaries: vec!["ruby-lsp".to_string()],
            description: "Ruby language server".to_string(),
        },
        // PHP
        InstallableServer {
            id: "php".to_string(),
            method: InstallMethod::Npm {
                package: "intelephense".to_string(),
            },
            binaries: vec!["intelephense".to_string()],
            description: "PHP language server".to_string(),
        },
        // Bash
        InstallableServer {
            id: "bash".to_string(),
            method: InstallMethod::Npm {
                package: "bash-language-server".to_string(),
            },
            binaries: vec!["bash-language-server".to_string()],
            description: "Bash language server".to_string(),
        },
        // Dart
        InstallableServer {
            id: "dart".to_string(),
            method: InstallMethod::Custom {
                command: "dart".to_string(),
                args: vec!["pub".to_string(), "global".to_string(), "activate".to_string(), "analyzer".to_string()],
            },
            binaries: vec!["dart".to_string()],
            description: "Dart language server".to_string(),
        },
        // Java/Kotlin — Eclipse JDT ships a platform-neutral tarball.
        InstallableServer {
            id: "jdtls".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec![
                    "-c".to_string(),
                    "set -eu; mkdir -p ~/.local/share/jdtls ~/.local/bin; \
                     wget -qO /tmp/jdtls.tar.gz https://download.eclipse.org/jdtls/snapshots/jdt-language-server-latest.tar.gz; \
                     tar -xzf /tmp/jdtls.tar.gz -C ~/.local/share/jdtls; \
                     ln -sf ~/.local/share/jdtls/bin/jdtls ~/.local/bin/jdtls; \
                     rm -f /tmp/jdtls.tar.gz".to_string(),
                ],
            },
            binaries: vec!["jdtls".to_string()],
            description: "Eclipse JDT Language Server".to_string(),
        },
        // C/C++ — apt install clangd. Requires the sandbox to run as root (or
        // passwordless sudo); the Dev-Box image ships with this.
        InstallableServer {
            id: "clangd".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec![
                    "-c".to_string(),
                    "set -eu; export DEBIAN_FRONTEND=noninteractive; \
                     if command -v sudo >/dev/null 2>&1 && [ \"$(id -u)\" != 0 ]; then \
                         sudo apt-get update -qq && sudo apt-get install -y --no-install-recommends clangd; \
                     else \
                         apt-get update -qq && apt-get install -y --no-install-recommends clangd; \
                     fi".to_string(),
                ],
            },
            binaries: vec!["clangd".to_string()],
            description: "Clangd C/C++ language server".to_string(),
        },
        // Zig
        InstallableServer {
            id: "zig".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec![
                    "-c".to_string(),
                    "set -eu; mkdir -p ~/.local/share/zls ~/.local/bin; \
                     wget -qO /tmp/zls.tar.gz https://github.com/zigtools/zls/releases/latest/download/zls-linux-x86_64.tar.gz; \
                     tar -xzf /tmp/zls.tar.gz -C ~/.local/share/zls; \
                     ln -sf ~/.local/share/zls/zls ~/.local/bin/zls; \
                     rm -f /tmp/zls.tar.gz".to_string(),
                ],
            },
            binaries: vec!["zls".to_string()],
            description: "Zig language server".to_string(),
        },
        // .NET
        InstallableServer {
            id: "csharp".to_string(),
            method: InstallMethod::Custom {
                command: "dotnet".to_string(),
                args: vec!["tool".to_string(), "install".to_string(), "--global".to_string(), "csharp-ls".to_string()],
            },
            binaries: vec!["csharp-ls".to_string()],
            description: "C# language server".to_string(),
        },
        // Haskell
        InstallableServer {
            id: "haskell".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec!["-c".to_string(), "echo 'Please install haskell-language-server via ghcup: ghcup install hls'".to_string()],
            },
            binaries: vec!["haskell-language-server-wrapper".to_string()],
            description: "Haskell language server".to_string(),
        },
        // Terraform
        InstallableServer {
            id: "terraform".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec![
                    "-c".to_string(),
                    // HashiCorp's /latest/ URL doesn't give the version directly,
                    // so resolve it via the GitHub releases API.
                    "set -eu; \
                     version=$(curl -fsSL https://api.github.com/repos/hashicorp/terraform-ls/releases/latest | sed -n 's/.*\"tag_name\": *\"v\\{0,1\\}\\([^\"]*\\)\".*/\\1/p'); \
                     [ -n \"$version\" ] || { echo 'could not resolve terraform-ls version' >&2; exit 1; }; \
                     mkdir -p ~/.local/bin; \
                     wget -qO /tmp/terraform-ls.zip \"https://releases.hashicorp.com/terraform-ls/${version}/terraform-ls_${version}_linux_amd64.zip\"; \
                     unzip -q -o /tmp/terraform-ls.zip -d ~/.local/bin/; \
                     chmod +x ~/.local/bin/terraform-ls; \
                     rm -f /tmp/terraform-ls.zip".to_string(),
                ],
            },
            binaries: vec!["terraform-ls".to_string()],
            description: "Terraform language server".to_string(),
        },
        // Dockerfile
        InstallableServer {
            id: "dockerfile".to_string(),
            method: InstallMethod::Npm {
                package: "dockerfile-language-server-nodejs".to_string(),
            },
            binaries: vec!["dockerfile-language-server-nodejs".to_string()],
            description: "Dockerfile language server".to_string(),
        },
        // YAML
        InstallableServer {
            id: "yaml-ls".to_string(),
            method: InstallMethod::Npm {
                package: "yaml-language-server".to_string(),
            },
            binaries: vec!["yaml-language-server".to_string()],
            description: "YAML language server".to_string(),
        },
        // Nix
        InstallableServer {
            id: "nixd".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec!["-c".to_string(), "echo 'Please install nixd via nix: nix profile install nixpkgs#nixd'".to_string()],
            },
            binaries: vec!["nixd".to_string()],
            description: "Nix language server".to_string(),
        },
        // Prisma
        InstallableServer {
            id: "prisma".to_string(),
            method: InstallMethod::Npm {
                package: "@prisma/language-server".to_string(),
            },
            binaries: vec!["prisma-language-server".to_string()],
            description: "Prisma language server".to_string(),
        },
        // Elm
        InstallableServer {
            id: "elm".to_string(),
            method: InstallMethod::Npm {
                package: "@elm-tooling/elm-language-server".to_string(),
            },
            binaries: vec!["elm-language-server".to_string()],
            description: "Elm language server".to_string(),
        },
        // Elixir
        InstallableServer {
            id: "elixir-ls".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec![
                    "-c".to_string(),
                    "cd /tmp && wget -q https://github.com/elixir-lsp/elixir-ls/releases/latest/download/elixir-ls.zip -O elixir-ls.zip && mkdir -p ~/.local/share/elixir-ls && unzip -q elixir-ls.zip -d ~/.local/share/elixir-ls && chmod +x ~/.local/share/elixir-ls/language_server.sh && ln -sf ~/.local/share/elixir-ls/language_server.sh ~/.local/bin/language_server.sh".to_string(),
                ],
            },
            binaries: vec!["language_server.sh".to_string()],
            description: "Elixir language server".to_string(),
        },
        // OCaml
        InstallableServer {
            id: "ocaml-lsp".to_string(),
            method: InstallMethod::Opam {
                package: "ocaml-lsp-server".to_string(),
            },
            binaries: vec!["ocamllsp".to_string()],
            description: "OCaml language server".to_string(),
        },
        // Clojure
        InstallableServer {
            id: "clojure-lsp".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec![
                    "-c".to_string(),
                    "cd /tmp && curl -sLO https://github.com/clojure-lsp/clojure-lsp/releases/latest/download/clojure-lsp-linux-amd64.zip && unzip -q clojure-lsp-linux-amd64.zip -d ~/.local/bin/ && chmod +x ~/.local/bin/clojure-lsp".to_string(),
                ],
            },
            binaries: vec!["clojure-lsp".to_string()],
            description: "Clojure language server".to_string(),
        },
        // Swift
        InstallableServer {
            id: "sourcekit-lsp".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec!["-c".to_string(), "echo 'Please install sourcekit-lsp via Swift toolchain'".to_string()],
            },
            binaries: vec!["sourcekit-lsp".to_string()],
            description: "Swift language server".to_string(),
        },
        // Julia
        InstallableServer {
            id: "julials".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec!["-c".to_string(), "echo 'Julia LanguageServer is auto-installed by the Julia package on first run'".to_string()],
            },
            binaries: vec!["julia".to_string()],
            description: "Julia language server".to_string(),
        },
        // Typst
        InstallableServer {
            id: "tinymist".to_string(),
            method: InstallMethod::Cargo {
                crate_name: "tinymist".to_string(),
            },
            binaries: vec!["tinymist".to_string()],
            description: "Typst language server".to_string(),
        },
        // Deno
        InstallableServer {
            id: "deno".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec!["-c".to_string(), "echo 'Please install Deno: curl -fsSL https://deno.land/install.sh | sh'".to_string()],
            },
            binaries: vec!["deno".to_string()],
            description: "Deno LSP (built into Deno CLI)".to_string(),
        },
        // Additional popular servers
        // Scala
        InstallableServer {
            id: "metals".to_string(),
            method: InstallMethod::Custom {
                command: "bash".to_string(),
                args: vec!["-c".to_string(), "cs install metals".to_string()],
            },
            binaries: vec!["metals".to_string()],
            description: "Scala language server (Metals)".to_string(),
        },
        // Python - Ruff (very fast linter/formatter with LSP)
        InstallableServer {
            id: "ruff".to_string(),
            method: InstallMethod::Pip {
                package: "ruff-lsp".to_string(),
            },
            binaries: vec!["ruff-lsp".to_string()],
            description: "Ruff Python LSP (fast linter/formatter)".to_string(),
        },
        // Python - python-lsp-server (alternative to pyright)
        InstallableServer {
            id: "pylsp".to_string(),
            method: InstallMethod::Pip {
                package: "python-lsp-server".to_string(),
            },
            binaries: vec!["pylsp".to_string()],
            description: "Python LSP Server (alternative to pyright)".to_string(),
        },
        // Tailwind CSS
        InstallableServer {
            id: "tailwindcss".to_string(),
            method: InstallMethod::Npm {
                package: "@tailwindcss/language-server".to_string(),
            },
            binaries: vec!["tailwindcss-language-server".to_string()],
            description: "Tailwind CSS language server".to_string(),
        },
        // Ruby - Official Ruby LSP (Shopify)
        InstallableServer {
            id: "ruby-lsp-official".to_string(),
            method: InstallMethod::Gem {
                gem: "ruby-lsp".to_string(),
            },
            binaries: vec!["ruby-lsp".to_string()],
            description: "Official Ruby LSP by Shopify".to_string(),
        },
        // GraphQL
        InstallableServer {
            id: "graphql".to_string(),
            method: InstallMethod::Npm {
                package: "graphql-language-service-cli".to_string(),
            },
            binaries: vec!["graphql-lsp".to_string()],
            description: "GraphQL language server".to_string(),
        },
        // CMake
        InstallableServer {
            id: "cmake".to_string(),
            method: InstallMethod::Pip {
                package: "cmake-language-server".to_string(),
            },
            binaries: vec!["cmake-language-server".to_string()],
            description: "CMake language server".to_string(),
        },
        // Ansible
        InstallableServer {
            id: "ansible".to_string(),
            method: InstallMethod::Pip {
                package: "ansible-language-server".to_string(),
            },
            binaries: vec!["ansible-language-server".to_string()],
            description: "Ansible language server".to_string(),
        },
        // VimScript
        InstallableServer {
            id: "vimls".to_string(),
            method: InstallMethod::Npm {
                package: "vim-language-server".to_string(),
            },
            binaries: vec!["vim-language-server".to_string()],
            description: "VimScript language server".to_string(),
        },
    ]
}

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
            InstallMethod::Gem { gem } => self.install_gem(gem).await,
            InstallMethod::Pip { package } => self.install_pip(package).await,
            InstallMethod::LuaRocks { rock } => self.install_luarocks(rock).await,
            InstallMethod::Opam { package } => self.install_opam(package).await,
            InstallMethod::Stack { package } => self.install_stack(package).await,
            InstallMethod::Custom { command, args } => self.install_custom(command, args).await,
        };

        match result {
            Ok(_) => {
                info!(server = %server_id, "installation complete");
                Ok(())
            }
            Err(e) => {
                error!(server = %server_id, error = %e, "installation failed");
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

    /// Install gem
    async fn install_gem(&self, gem: &str) -> Result<(), String> {
        debug!(gem = %gem, "running gem install");
        self.run_install_cmd("gem", &["install", gem], gem).await
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

    /// Install luarocks package
    async fn install_luarocks(&self, rock: &str) -> Result<(), String> {
        debug!(rock = %rock, "running luarocks install");
        let status = tokio::process::Command::new("luarocks")
            .args(["install", rock])
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .status()
            .await
            .map_err(|e| format!("Failed to run luarocks: {}", e))?;

        if status.success() {
            Ok(())
        } else {
            Err(format!("luarocks install failed for {}", rock))
        }
    }

    /// Install opam package
    async fn install_opam(&self, package: &str) -> Result<(), String> {
        debug!(package = %package, "running opam install");
        let status = tokio::process::Command::new("opam")
            .args(["install", "-y", package])
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .status()
            .await
            .map_err(|e| format!("Failed to run opam: {}", e))?;

        if status.success() {
            Ok(())
        } else {
            Err(format!("opam install failed for {}", package))
        }
    }

    /// Install stack package
    async fn install_stack(&self, package: &str) -> Result<(), String> {
        debug!(package = %package, "running stack install");
        let status = tokio::process::Command::new("stack")
            .args(["install", package])
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .status()
            .await
            .map_err(|e| format!("Failed to run stack: {}", e))?;

        if status.success() {
            Ok(())
        } else {
            Err(format!("stack install failed for {}", package))
        }
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_installable_servers_list() {
        let servers = installable_servers();
        assert!(!servers.is_empty(), "should have installable servers");

        // Check that popular servers are included
        let ids: Vec<&str> = servers.iter().map(|s| s.id.as_str()).collect();
        assert!(ids.contains(&"typescript"), "should include typescript");
        assert!(ids.contains(&"rust"), "should include rust");
        assert!(ids.contains(&"go"), "should include go");
        assert!(ids.contains(&"python"), "should include python");
    }

    #[test]
    fn test_installer_new() {
        let installer = LspInstaller::new();
        let available = installer.available_servers();
        assert!(!available.is_empty(), "should have available servers");
        assert!(available.contains(&"typescript".to_string()));
    }

    #[test]
    fn test_server_methods() {
        let servers = installable_servers();

        // Check various install methods are represented
        let has_npm = servers
            .iter()
            .any(|s| matches!(s.method, InstallMethod::Npm { .. }));
        let has_cargo = servers
            .iter()
            .any(|s| matches!(s.method, InstallMethod::Cargo { .. }));
        let has_go = servers
            .iter()
            .any(|s| matches!(s.method, InstallMethod::Go { .. }));

        assert!(has_npm, "should have npm-based servers");
        assert!(has_cargo, "should have cargo-based servers");
        assert!(has_go, "should have go-based servers");
    }
}
