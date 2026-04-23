use super::def::{server, server_with_init, ServerDef};

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
            &["svelteserver", "--stdio"],
            &["svelte"],
            &[
                "package.json",
                "package-lock.json",
                "yarn.lock",
                "pnpm-lock.yaml",
            ],
        ),
        server_with_init(
            "astro",
            &["astro-ls", "--stdio"],
            &["astro"],
            &[
                "package.json",
                "package-lock.json",
                "yarn.lock",
                "pnpm-lock.yaml",
            ],
            // astro-ls requires a path to the local TypeScript SDK; resolved
            // against the workspace root. Users must have `typescript` installed
            // in their project (via npm install).
            serde_json::json!({
                "typescript": {
                    "tsdk": "node_modules/typescript/lib"
                }
            }),
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
            &["ruby-lsp"],
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
        server("vimls", &["vim-language-server", "--stdio"], &["vim"], &[]),
        server(
            "graphql",
            &["graphql-lsp", "server", "--method=stream"],
            &["graphql", "gql"],
            &[
                ".graphqlrc",
                ".graphqlrc.yml",
                ".graphqlrc.json",
                "graphql.config.js",
            ],
        ),
        server(
            "cmake",
            &["cmake-language-server"],
            &["cmake"],
            &["CMakeLists.txt"],
        ),
    ]
}
