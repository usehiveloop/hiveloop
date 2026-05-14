mod cli;
mod logging;
mod observability;
mod server;

use clap::Parser;

use cli::Cli;
use server::run_server;

fn main() -> anyhow::Result<()> {
    // Sentry guard is held for the lifetime of the process. Drop on exit
    // flushes any in-flight events. Init runs *before* the tokio runtime
    // so the panic handler is wired before any async task can panic.
    let _sentry_guard = observability::init_sentry();

    let _cli = Cli::parse();

    tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()?
        .block_on(run_server())
}
