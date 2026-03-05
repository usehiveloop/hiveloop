use std::collections::HashMap;
use std::path::{Path, PathBuf};

/// A server definition describes how to launch and configure an LSP server.
#[derive(Debug, Clone)]
pub struct ServerDef {
    /// Unique identifier for this server (e.g., "typescript", "rust")
    pub id: String,
    /// Command and arguments to launch the server
    pub command: Vec<String>,
    /// File extensions this server handles
    pub extensions: Vec<String>,
    /// Files/directories that indicate the project root
    pub root_markers: Vec<String>,
    /// Environment variables to set when spawning
    pub env: HashMap<String, String>,
    /// Custom initialization options
    pub init_options: Option<serde_json::Value>,
}

/// Helper to build a ServerDef concisely.
fn server(id: &str, command: &[&str], extensions: &[&str], root_markers: &[&str]) -> ServerDef {
    ServerDef {
        id: id.into(),
        command: command.iter().map(|s| s.to_string()).collect(),
        extensions: extensions.iter().map(|s| s.to_string()).collect(),
        root_markers: root_markers.iter().map(|s| s.to_string()).collect(),
        env: HashMap::new(),
        init_options: None,
    }
}

/// Returns the built-in LSP server definitions.
pub fn builtin_servers() -> Vec<ServerDef> {
    vec![
        // --- JavaScript / TypeScript ---
        server(
            "typescript",
            &["typescript-language-server", "--stdio"],
            &["ts", "tsx", "js", "jsx", "mjs", "cjs", "mts", "cts"],
            &[
                "tsconfig.json",
                "jsconfig.json",
                "package.json",
                "package-lock.json",
                "yarn.lock",
                "pnpm-lock.yaml",
                "bun.lockb",
            ],
        ),
        server(
            "deno",
            &["deno", "lsp"],
            &["ts", "tsx", "js", "jsx", "mjs"],
            &["deno.json", "deno.jsonc"],
        ),
        server(
            "eslint",
            &["eslint", "--lsp"],
            &["ts", "tsx", "js", "jsx"],
            &[
                "package.json",
                ".eslintrc",
                ".eslintrc.js",
                ".eslintrc.json",
                ".eslintrc.yml",
                "eslint.config.js",
                "eslint.config.mjs",
                "eslint.config.ts",
            ],
        ),
        server(
            "biome",
            &["biome", "lsp-proxy", "--stdio"],
            &["ts", "tsx", "js", "jsx", "json", "css"],
            &["biome.json", "biome.jsonc"],
        ),
        // --- Web frameworks ---
        server(
            "vue",
            &["vue-language-server", "--stdio"],
            &["vue"],
            &[
                "package.json",
                "package-lock.json",
                "yarn.lock",
                "pnpm-lock.yaml",
            ],
        ),
        server(
            "svelte",
            &["svelte-language-server", "--stdio"],
            &["svelte"],
            &[
                "package.json",
                "package-lock.json",
                "yarn.lock",
                "pnpm-lock.yaml",
            ],
        ),
        server(
            "astro",
            &["astro-ls", "--stdio"],
            &["astro"],
            &[
                "package.json",
                "package-lock.json",
                "yarn.lock",
                "pnpm-lock.yaml",
            ],
        ),
        // --- Systems ---
        server(
            "rust",
            &["rust-analyzer"],
            &["rs"],
            &["Cargo.toml", "Cargo.lock"],
        ),
        server("go", &["gopls"], &["go"], &["go.mod", "go.sum"]),
        server(
            "clangd",
            &["clangd", "--background-index", "--clang-tidy"],
            &["c", "cpp", "cc", "cxx", "h", "hpp", "hh", "hxx"],
            &["compile_commands.json", "CMakeLists.txt", "Makefile"],
        ),
        server("zig", &["zls"], &["zig", "zon"], &["build.zig"]),
        // --- Scripting ---
        server(
            "python",
            &["pyright-langserver", "--stdio"],
            &["py", "pyi"],
            &[
                "pyproject.toml",
                "setup.py",
                "setup.cfg",
                "requirements.txt",
                "Pipfile",
                "pyrightconfig.json",
            ],
        ),
        server(
            "ruby-lsp",
            &["rubocop", "--lsp"],
            &["rb", "rake", "gemspec", "ru"],
            &["Gemfile"],
        ),
        server(
            "php",
            &["intelephense", "--stdio"],
            &["php"],
            &["composer.json"],
        ),
        server(
            "lua-ls",
            &["lua-language-server"],
            &["lua"],
            &[".luarc.json", ".stylua.toml"],
        ),
        server(
            "bash",
            &["bash-language-server", "start"],
            &["sh", "bash", "zsh", "ksh"],
            &[],
        ),
        server(
            "dart",
            &["dart", "language-server", "--lsp"],
            &["dart"],
            &["pubspec.yaml"],
        ),
        // --- JVM ---
        server("jdtls", &["jdtls"], &["java"], &["pom.xml", "build.gradle"]),
        server(
            "kotlin-ls",
            &["kotlin-language-server"],
            &["kt", "kts"],
            &["settings.gradle", "build.gradle", "pom.xml"],
        ),
        // --- .NET ---
        server(
            "csharp",
            &["csharp-ls"],
            &["cs"],
            &[".sln", ".csproj", "global.json"],
        ),
        server(
            "fsharp",
            &["fsautocomplete"],
            &["fs", "fsi", "fsx"],
            &[".sln", ".fsproj", "global.json"],
        ),
        // --- Functional ---
        server(
            "elixir-ls",
            &["language_server.sh"],
            &["ex", "exs"],
            &["mix.exs", "mix.lock"],
        ),
        server(
            "haskell",
            &["haskell-language-server-wrapper", "--lsp"],
            &["hs", "lhs"],
            &["stack.yaml", "cabal.project", "hie.yaml"],
        ),
        server(
            "ocaml-lsp",
            &["ocamllsp"],
            &["ml", "mli"],
            &["dune-project", "opam"],
        ),
        server("gleam", &["gleam", "lsp"], &["gleam"], &["gleam.toml"]),
        server(
            "clojure-lsp",
            &["clojure-lsp", "listen"],
            &["clj", "cljs", "cljc", "edn"],
            &["deps.edn", "project.clj"],
        ),
        server("elm", &["elm-language-server"], &["elm"], &["elm.json"]),
        // --- Other ---
        server(
            "prisma",
            &["prisma-language-server", "--stdio"],
            &["prisma"],
            &["schema.prisma"],
        ),
        server(
            "terraform",
            &["terraform-ls", "serve"],
            &["tf", "tfvars"],
            &[".terraform.lock.hcl"],
        ),
        server("texlab", &["texlab"], &["tex", "bib"], &[".latexmkrc"]),
        server(
            "dockerfile",
            &["dockerfile-language-server-nodejs", "--stdio"],
            &["dockerfile"],
            &[],
        ),
        server("nixd", &["nixd"], &["nix"], &["flake.nix"]),
        server("tinymist", &["tinymist"], &["typ", "typc"], &["typst.toml"]),
        server(
            "julials",
            &[
                "julia",
                "--startup-file=no",
                "-e",
                "using LanguageServer; runserver()",
            ],
            &["jl"],
            &["Project.toml"],
        ),
        server(
            "sourcekit-lsp",
            &["sourcekit-lsp"],
            &["swift"],
            &["Package.swift"],
        ),
        server(
            "yaml-ls",
            &["yaml-language-server", "--stdio"],
            &["yaml", "yml"],
            &[],
        ),
    ]
}

/// Walk up from `file` looking for any of the given marker files/directories.
/// Returns the directory containing the first marker found, or `None`.
pub fn find_root(file: &Path, markers: &[String]) -> Option<PathBuf> {
    let mut dir = if file.is_file() {
        file.parent()?.to_path_buf()
    } else {
        file.to_path_buf()
    };

    loop {
        for marker in markers {
            if dir.join(marker).exists() {
                return Some(dir);
            }
        }
        if !dir.pop() {
            return None;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_builtin_servers_count() {
        let servers = builtin_servers();
        // Should have 30+ servers
        assert!(
            servers.len() >= 30,
            "expected at least 30 servers, got {}",
            servers.len()
        );
    }

    #[test]
    fn test_builtin_server_ids() {
        let servers = builtin_servers();
        let ids: Vec<&str> = servers.iter().map(|s| s.id.as_str()).collect();
        // Original 4
        assert!(ids.contains(&"typescript"));
        assert!(ids.contains(&"rust"));
        assert!(ids.contains(&"go"));
        assert!(ids.contains(&"python"));
        // New servers
        assert!(ids.contains(&"deno"));
        assert!(ids.contains(&"vue"));
        assert!(ids.contains(&"svelte"));
        assert!(ids.contains(&"clangd"));
        assert!(ids.contains(&"bash"));
        assert!(ids.contains(&"haskell"));
        assert!(ids.contains(&"nixd"));
        assert!(ids.contains(&"zig"));
    }

    #[test]
    fn test_rust_server_extensions() {
        let servers = builtin_servers();
        let rust = servers.iter().find(|s| s.id == "rust").unwrap();
        assert_eq!(rust.extensions, vec!["rs"]);
        assert!(rust.root_markers.contains(&"Cargo.toml".to_string()));
    }

    #[test]
    fn test_all_servers_have_commands() {
        let servers = builtin_servers();
        for server in &servers {
            assert!(
                !server.command.is_empty(),
                "server '{}' has empty command",
                server.id
            );
        }
    }

    #[test]
    fn test_all_servers_have_extensions() {
        let servers = builtin_servers();
        for server in &servers {
            assert!(
                !server.extensions.is_empty(),
                "server '{}' has no extensions",
                server.id
            );
        }
    }

    #[test]
    fn test_find_root_with_marker() {
        let tmp = tempfile::tempdir().unwrap();
        let project = tmp.path().join("project");
        std::fs::create_dir_all(&project).unwrap();
        std::fs::write(project.join("Cargo.toml"), "").unwrap();

        let src = project.join("src");
        std::fs::create_dir_all(&src).unwrap();
        let file = src.join("main.rs");
        std::fs::write(&file, "").unwrap();

        let root = find_root(&file, &["Cargo.toml".to_string()]);
        assert_eq!(root, Some(project));
    }

    #[test]
    fn test_find_root_no_marker() {
        let tmp = tempfile::tempdir().unwrap();
        let file = tmp.path().join("orphan.rs");
        std::fs::write(&file, "").unwrap();

        let root = find_root(&file, &["Cargo.toml".to_string()]);
        // May find a Cargo.toml higher up or return None
        // The important thing is it doesn't panic
        let _ = root;
    }
}
