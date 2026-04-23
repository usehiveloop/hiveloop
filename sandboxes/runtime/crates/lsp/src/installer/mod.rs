//! LSP Server Installer
//!
//! Handles installation of LSP servers via various package managers.
//! Runs asynchronously in the background when bridge starts with --install-lsp-servers.

mod methods;
mod runner;
mod servers;
#[cfg(test)]
mod tests;

pub use methods::{InstallMethod, InstallableServer};
pub use runner::LspInstaller;
pub use servers::installable_servers;
