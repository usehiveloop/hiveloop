use clap::{Parser, Subcommand};

/// Bridge - AI Agent Runtime
#[derive(Parser)]
#[command(name = "bridge")]
#[command(about = "AI Agent Runtime with tool execution and MCP support")]
#[command(version = "0.6.2")]
pub(crate) struct Cli {
    #[command(subcommand)]
    pub(crate) command: Option<Commands>,
}

#[derive(Subcommand)]
pub(crate) enum Commands {
    /// List available tools
    Tools {
        #[command(subcommand)]
        action: Option<ToolCommands>,
    },
    /// Install LSP servers (comma-separated list of IDs, or "all")
    InstallLsp {
        /// Comma-separated server IDs (e.g. "rust,go,typescript") or "all"
        #[arg(value_name = "SERVERS")]
        servers: String,
    },
}

#[derive(Subcommand)]
pub(crate) enum ToolCommands {
    /// List all available tools
    List {
        /// Output as JSON
        #[arg(long, default_value_t = true)]
        json: bool,
        /// Show only read-only tools (tools that don't modify state)
        #[arg(long)]
        read_only: bool,
    },
}
