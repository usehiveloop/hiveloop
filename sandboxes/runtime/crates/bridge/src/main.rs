mod cli;
mod commands;
mod logging;
mod server;

use clap::Parser;

use cli::{Cli, Commands};
use commands::{handle_install_lsp_command, handle_tools_command};
use server::run_server;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Some(Commands::Tools { action }) => {
            handle_tools_command(action).await?;
            Ok(())
        }
        Some(Commands::InstallLsp { servers }) => handle_install_lsp_command(servers).await,
        None => run_server().await,
    }
}
