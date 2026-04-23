/// Installation method for an LSP server.
///
/// Only package managers that are broadly available on typical Linux/macOS
/// dev boxes are supported here. Servers whose only distribution is via
/// rare toolchains (opam, luarocks, stack, gem, dart pub, coursier, ...)
/// were dropped from `installable_servers()` because attempting to install
/// them reliably fails with `No such file or directory` on boxes that
/// haven't pre-installed the toolchain.
#[derive(Debug, Clone)]
pub enum InstallMethod {
    /// Install via npm: `npm install -g <package>`
    Npm { package: String },
    /// Install via cargo: `cargo install <crate>`
    Cargo { crate_name: String },
    /// Install via go: `go install <path>@latest`
    Go { path: String },
    /// Install via pip: `pip install <package>`
    Pip { package: String },
    /// Custom install command (usually a curl/wget + unpack script).
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
