use super::super::methods::InstallableServer;
use super::{bash_cmd, entry, go_path, npm};

pub(super) fn append(servers: &mut Vec<InstallableServer>) {
    // Go
    servers.push(entry(
        "go",
        go_path("golang.org/x/tools/gopls@latest"),
        "gopls",
        "Go language server",
    ));
    // Python
    servers.push(entry(
        "python",
        npm("pyright"),
        "pyright-langserver",
        "Pyright language server",
    ));
    // PHP
    servers.push(entry(
        "php",
        npm("intelephense"),
        "intelephense",
        "PHP language server",
    ));
    // Bash
    servers.push(entry(
        "bash",
        npm("bash-language-server"),
        "bash-language-server",
        "Bash language server",
    ));
    // Java/Kotlin — Eclipse JDT ships a platform-neutral tarball.
    servers.push(entry(
        "jdtls",
        bash_cmd(
            "set -eu; mkdir -p ~/.local/share/jdtls ~/.local/bin; \
             wget -qO /tmp/jdtls.tar.gz https://download.eclipse.org/jdtls/snapshots/jdt-language-server-latest.tar.gz; \
             tar -xzf /tmp/jdtls.tar.gz -C ~/.local/share/jdtls; \
             ln -sf ~/.local/share/jdtls/bin/jdtls ~/.local/bin/jdtls; \
             rm -f /tmp/jdtls.tar.gz",
        ),
        "jdtls",
        "Eclipse JDT Language Server",
    ));
    // C/C++ — apt install clangd. Requires the sandbox to run as root (or
    // passwordless sudo); the Dev-Box image ships with this.
    servers.push(entry(
        "clangd",
        bash_cmd(
            "set -eu; export DEBIAN_FRONTEND=noninteractive; \
             if command -v sudo >/dev/null 2>&1 && [ \"$(id -u)\" != 0 ]; then \
                 sudo apt-get update -qq && sudo apt-get install -y --no-install-recommends clangd; \
             else \
                 apt-get update -qq && apt-get install -y --no-install-recommends clangd; \
             fi",
        ),
        "clangd",
        "Clangd C/C++ language server",
    ));
    // Zig
    servers.push(entry(
        "zig",
        bash_cmd(
            "set -eu; mkdir -p ~/.local/share/zls ~/.local/bin; \
             wget -qO /tmp/zls.tar.gz https://github.com/zigtools/zls/releases/latest/download/zls-linux-x86_64.tar.gz; \
             tar -xzf /tmp/zls.tar.gz -C ~/.local/share/zls; \
             ln -sf ~/.local/share/zls/zls ~/.local/bin/zls; \
             rm -f /tmp/zls.tar.gz",
        ),
        "zls",
        "Zig language server",
    ));
    // Terraform
    servers.push(entry(
        "terraform",
        bash_cmd(
            // HashiCorp's /latest/ URL doesn't give the version directly,
            // so resolve it via the GitHub releases API.
            "set -eu; \
             version=$(curl -fsSL https://api.github.com/repos/hashicorp/terraform-ls/releases/latest | sed -n 's/.*\"tag_name\": *\"v\\{0,1\\}\\([^\"]*\\)\".*/\\1/p'); \
             [ -n \"$version\" ] || { echo 'could not resolve terraform-ls version' >&2; exit 1; }; \
             mkdir -p ~/.local/bin; \
             wget -qO /tmp/terraform-ls.zip \"https://releases.hashicorp.com/terraform-ls/${version}/terraform-ls_${version}_linux_amd64.zip\"; \
             unzip -q -o /tmp/terraform-ls.zip -d ~/.local/bin/; \
             chmod +x ~/.local/bin/terraform-ls; \
             rm -f /tmp/terraform-ls.zip",
        ),
        "terraform-ls",
        "Terraform language server",
    ));
    // Dockerfile
    servers.push(entry(
        "dockerfile",
        npm("dockerfile-language-server-nodejs"),
        "dockerfile-language-server-nodejs",
        "Dockerfile language server",
    ));
}
