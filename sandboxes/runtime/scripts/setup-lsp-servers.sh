#!/usr/bin/env bash
set -euo pipefail

# Install 5 LSP servers locally into .lsp-servers/ for integration tests.
# Requires: node, npm, go (for gopls), curl (for rust-analyzer).
# Idempotent — re-running updates to latest versions.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LSP_DIR="$PROJECT_ROOT/.lsp-servers"
BIN_DIR="$LSP_DIR/bin"

mkdir -p "$BIN_DIR"

ok()   { echo "  ✓ $1"; }
skip() { echo "  ⊘ $1 (skipped: $2)"; }
fail() { echo "  ✗ $1 (failed: $2)"; }

# ---------- rust-analyzer ----------
install_rust_analyzer() {
    echo "→ rust-analyzer"
    if ! command -v curl &>/dev/null; then
        skip "rust-analyzer" "curl not found"
        return
    fi

    local arch
    arch="$(uname -m)"
    local os
    os="$(uname -s)"

    local target
    if [[ "$os" == "Darwin" ]]; then
        if [[ "$arch" == "arm64" || "$arch" == "aarch64" ]]; then
            target="aarch64-apple-darwin"
        else
            target="x86_64-apple-darwin"
        fi
    elif [[ "$os" == "Linux" ]]; then
        if [[ "$arch" == "aarch64" ]]; then
            target="aarch64-unknown-linux-gnu"
        else
            target="x86_64-unknown-linux-gnu"
        fi
    else
        skip "rust-analyzer" "unsupported OS: $os"
        return
    fi

    local url="https://github.com/rust-lang/rust-analyzer/releases/latest/download/rust-analyzer-${target}.gz"
    if curl -fsSL "$url" | gunzip > "$BIN_DIR/rust-analyzer.tmp"; then
        mv "$BIN_DIR/rust-analyzer.tmp" "$BIN_DIR/rust-analyzer"
        chmod +x "$BIN_DIR/rust-analyzer"
        ok "rust-analyzer ($target)"
    else
        rm -f "$BIN_DIR/rust-analyzer.tmp"
        fail "rust-analyzer" "download failed"
    fi
}

# ---------- gopls ----------
install_gopls() {
    echo "→ gopls"
    if ! command -v go &>/dev/null; then
        skip "gopls" "go not found"
        return
    fi

    if GOBIN="$BIN_DIR" go install golang.org/x/tools/gopls@latest; then
        ok "gopls"
    else
        fail "gopls" "go install failed"
    fi
}

# ---------- npm-based servers ----------
install_npm_servers() {
    echo "→ npm servers (typescript-language-server, pyright-langserver, vue-language-server)"
    if ! command -v npm &>/dev/null; then
        skip "npm servers" "npm not found"
        return
    fi

    cd "$LSP_DIR"
    if npm install --save \
        typescript-language-server \
        typescript \
        pyright \
        @vue/language-server \
        2>&1 | tail -1; then
        ok "typescript-language-server"
        ok "pyright-langserver"
        ok "vue-language-server (@vue/language-server)"
    else
        fail "npm servers" "npm install failed"
    fi
    cd "$PROJECT_ROOT"
}

echo "Installing LSP servers into $LSP_DIR"
echo ""

install_rust_analyzer
install_gopls
install_npm_servers

echo ""
echo "Done. Installed servers:"
for bin in rust-analyzer gopls; do
    if [[ -x "$BIN_DIR/$bin" ]]; then
        echo "  $BIN_DIR/$bin"
    fi
done
for bin in typescript-language-server pyright-langserver vue-language-server; do
    local_bin="$LSP_DIR/node_modules/.bin/$bin"
    if [[ -x "$local_bin" ]]; then
        echo "  $local_bin"
    fi
done
