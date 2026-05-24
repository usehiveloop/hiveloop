//! Pipe a child's stderr into the tracing subscriber so harness diagnostics
//! show up in bridge logs without us having to scrape `docker logs`.

use tokio::io::{AsyncBufReadExt, BufReader};
use tracing::info;

pub async fn pipe_opencode(stderr: tokio::process::ChildStderr) {
    let mut lines = BufReader::new(stderr).lines();
    while let Ok(Some(line)) = lines.next_line().await {
        info!(target: "opencode_acp", "{}", line);
    }
}
