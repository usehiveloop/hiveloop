use super::super::methods::InstallableServer;
use super::{bash_cmd, entry, npm};

pub(super) fn append(servers: &mut Vec<InstallableServer>) {
    // JavaScript/TypeScript
    servers.push(entry(
        "typescript",
        npm("typescript-language-server"),
        "typescript-language-server",
        "TypeScript/JavaScript language server",
    ));
    servers.push(entry(
        "eslint",
        npm("eslint"),
        "eslint",
        "ESLint LSP server",
    ));
    servers.push(entry(
        "biome",
        npm("@biomejs/biome"),
        "biome",
        "Biome LSP server for JS/TS/JSON/CSS",
    ));
    // Deno — the LSP is built into the `deno` CLI (`deno lsp`). The official
    // install script honors DENO_INSTALL as a prefix, so the binary lands
    // at ~/.local/bin/deno to match the other self-contained downloads.
    servers.push(entry(
        "deno",
        bash_cmd(
            "set -eu; mkdir -p \"$HOME/.local/bin\"; \
             DENO_INSTALL=\"$HOME/.local\" \
             curl -fsSL https://deno.land/install.sh | sh",
        ),
        "deno",
        "Deno language server (built into the Deno CLI)",
    ));
    // Web frameworks
    servers.push(entry(
        "vue",
        npm("@vue/language-server"),
        "vue-language-server",
        "Vue language server",
    ));
    servers.push(entry(
        "svelte",
        npm("svelte-language-server"),
        "svelteserver",
        "Svelte language server",
    ));
    servers.push(entry(
        "astro",
        npm("@astrojs/language-server"),
        "astro-ls",
        "Astro language server",
    ));
    // Rust — download the prebuilt rust-analyzer binary. `cargo install`
    // builds from source and takes ~10 minutes; the release binary is a
    // few-megabyte download.
    servers.push(entry(
        "rust",
        bash_cmd(
            "set -eu; mkdir -p ~/.local/bin; \
             curl -fsSL https://github.com/rust-lang/rust-analyzer/releases/latest/download/rust-analyzer-x86_64-unknown-linux-gnu.gz \
               | gunzip > ~/.local/bin/rust-analyzer; \
             chmod +x ~/.local/bin/rust-analyzer",
        ),
        "rust-analyzer",
        "Rust analyzer",
    ));
}
