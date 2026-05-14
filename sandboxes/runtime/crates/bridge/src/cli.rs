use clap::Parser;

/// Bridge - HTTP translation layer for coding agents.
#[derive(Parser)]
#[command(name = "bridge")]
#[command(about = "HTTP translation layer for coding agents (Claude Code via ACP, OpenCode, ...)")]
#[command(version = env!("CARGO_PKG_VERSION"))]
pub(crate) struct Cli {}
