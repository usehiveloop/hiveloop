use super::super::methods::InstallableServer;
use super::{bash_cmd, cargo_crate, entry, npm, pip};

pub(super) fn append(servers: &mut Vec<InstallableServer>) {
    // YAML
    servers.push(entry(
        "yaml-ls",
        npm("yaml-language-server"),
        "yaml-language-server",
        "YAML language server",
    ));
    // Prisma
    servers.push(entry(
        "prisma",
        npm("@prisma/language-server"),
        "prisma-language-server",
        "Prisma language server",
    ));
    // Elm
    servers.push(entry(
        "elm",
        npm("@elm-tooling/elm-language-server"),
        "elm-language-server",
        "Elm language server",
    ));
    // Elixir
    servers.push(entry(
        "elixir-ls",
        bash_cmd(
            "cd /tmp && wget -q https://github.com/elixir-lsp/elixir-ls/releases/latest/download/elixir-ls.zip -O elixir-ls.zip && mkdir -p ~/.local/share/elixir-ls && unzip -q elixir-ls.zip -d ~/.local/share/elixir-ls && chmod +x ~/.local/share/elixir-ls/language_server.sh && ln -sf ~/.local/share/elixir-ls/language_server.sh ~/.local/bin/language_server.sh",
        ),
        "language_server.sh",
        "Elixir language server",
    ));
    // Clojure
    servers.push(entry(
        "clojure-lsp",
        bash_cmd(
            "cd /tmp && curl -sLO https://github.com/clojure-lsp/clojure-lsp/releases/latest/download/clojure-lsp-linux-amd64.zip && unzip -q clojure-lsp-linux-amd64.zip -d ~/.local/bin/ && chmod +x ~/.local/bin/clojure-lsp",
        ),
        "clojure-lsp",
        "Clojure language server",
    ));
    // Typst
    servers.push(entry(
        "tinymist",
        cargo_crate("tinymist"),
        "tinymist",
        "Typst language server",
    ));
    // Python - Ruff (very fast linter/formatter with LSP)
    servers.push(entry(
        "ruff",
        pip("ruff-lsp"),
        "ruff-lsp",
        "Ruff Python LSP (fast linter/formatter)",
    ));
    // Python - python-lsp-server (alternative to pyright)
    servers.push(entry(
        "pylsp",
        pip("python-lsp-server"),
        "pylsp",
        "Python LSP Server (alternative to pyright)",
    ));
    // Tailwind CSS
    servers.push(entry(
        "tailwindcss",
        npm("@tailwindcss/language-server"),
        "tailwindcss-language-server",
        "Tailwind CSS language server",
    ));
    // GraphQL
    servers.push(entry(
        "graphql",
        npm("graphql-language-service-cli"),
        "graphql-lsp",
        "GraphQL language server",
    ));
    // CMake
    servers.push(entry(
        "cmake",
        pip("cmake-language-server"),
        "cmake-language-server",
        "CMake language server",
    ));
    // Ansible
    servers.push(entry(
        "ansible",
        pip("ansible-language-server"),
        "ansible-language-server",
        "Ansible language server",
    ));
    // VimScript
    servers.push(entry(
        "vimls",
        npm("vim-language-server"),
        "vim-language-server",
        "VimScript language server",
    ));
}
