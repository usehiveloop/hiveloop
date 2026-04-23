use anyhow::{anyhow, Context, Result};
use std::io::{BufRead, BufReader};
use std::time::{Duration, Instant};

use super::TestHarness;

impl TestHarness {
    /// Read the PORT={port} line from the mock control plane stdout.
    ///
    /// Returns the port and a background thread that drains remaining stdout.
    /// The drain thread keeps the pipe alive so the child process doesn't get
    /// EPIPE when it writes after PORT=.
    pub(super) fn read_port_from_stdout(
        stdout: std::process::ChildStdout,
    ) -> Result<(u16, std::thread::JoinHandle<()>)> {
        let mut reader = BufReader::new(stdout);
        let start = Instant::now();
        let timeout = Duration::from_secs(30);

        let mut line_buf = String::new();
        loop {
            if start.elapsed() > timeout {
                return Err(anyhow!(
                    "timed out waiting for PORT= from mock-control-plane"
                ));
            }

            line_buf.clear();
            let bytes_read = reader
                .read_line(&mut line_buf)
                .context("failed to read stdout line")?;
            if bytes_read == 0 {
                return Err(anyhow!("mock-control-plane exited without printing PORT="));
            }

            let line = line_buf.trim();
            if let Some(port_str) = line.strip_prefix("PORT=") {
                let port: u16 = port_str
                    .trim()
                    .parse()
                    .context("failed to parse port number")?;

                // Drain remaining stdout in background to prevent EPIPE
                let drain_handle = std::thread::spawn(move || {
                    let mut sink = Vec::new();
                    let _ = std::io::Read::read_to_end(&mut reader, &mut sink);
                });

                return Ok((port, drain_handle));
            }
        }
    }

    /// Find a free TCP port by binding to port 0 and reading the assigned port.
    pub(super) fn find_free_port() -> Result<u16> {
        let listener = std::net::TcpListener::bind("127.0.0.1:0")
            .context("failed to bind to find free port")?;
        let port = listener.local_addr()?.port();
        drop(listener);
        Ok(port)
    }

    pub(super) async fn wait_for_bridge_healthy_with_timeout(
        &mut self,
        timeout: Duration,
    ) -> Result<()> {
        let start = Instant::now();
        let poll_interval = Duration::from_millis(100);

        loop {
            if start.elapsed() > timeout {
                // Check if bridge process is still alive
                if let Some(ref mut proc) = self.bridge_process {
                    match proc.try_wait() {
                        Ok(Some(status)) => {
                            return Err(anyhow!(
                                "bridge process exited with status {} before becoming healthy",
                                status
                            ));
                        }
                        Ok(None) => {} // still running
                        Err(e) => {
                            return Err(anyhow!("failed to check bridge process status: {}", e));
                        }
                    }
                }
                return Err(anyhow!(
                    "timed out waiting for bridge /health ({:.0}s elapsed)",
                    timeout.as_secs_f64()
                ));
            }

            match self
                .client
                .get(format!("{}/health", self.bridge_base_url))
                .send()
                .await
            {
                Ok(resp) if resp.status().is_success() => {
                    tracing::info!(
                        elapsed_ms = start.elapsed().as_millis() as u64,
                        "bridge is healthy"
                    );
                    return Ok(());
                }
                _ => {
                    tokio::time::sleep(poll_interval).await;
                }
            }
        }
    }

    /// Wait until at least `min_count` agents are loaded in the bridge.
    pub(super) async fn wait_for_agents_loaded(&self, min_count: usize) -> Result<()> {
        let start = Instant::now();
        let timeout = Duration::from_secs(60);
        let poll_interval = Duration::from_secs(2);

        loop {
            if start.elapsed() > timeout {
                return Err(anyhow!(
                    "timed out waiting for {} agents to load",
                    min_count
                ));
            }

            match self.get_agents().await {
                Ok(agents) if agents.len() >= min_count => {
                    tracing::info!(
                        count = agents.len(),
                        elapsed_ms = start.elapsed().as_millis() as u64,
                        "agents loaded"
                    );
                    return Ok(());
                }
                Ok(agents) => {
                    tracing::debug!(
                        count = agents.len(),
                        target = min_count,
                        "waiting for agents to load..."
                    );
                }
                Err(_) => {}
            }

            tokio::time::sleep(poll_interval).await;
        }
    }

    /// Stop the harness — gracefully terminate processes so logs are flushed.
    pub fn stop(&mut self) {
        for (name, proc_opt) in [
            ("bridge", &mut self.bridge_process),
            ("mock-cp", &mut self.mock_cp_process),
        ] {
            if let Some(ref mut proc) = proc_opt {
                // Send SIGTERM first for graceful shutdown (flushes logs)
                #[cfg(unix)]
                {
                    let graceful_timeout =
                        if name == "bridge" && std::env::var("BRIDGE_STORAGE_PATH").is_ok() {
                            std::time::Duration::from_secs(45)
                        } else {
                            std::time::Duration::from_secs(5)
                        };

                    unsafe {
                        libc::kill(proc.id() as i32, libc::SIGTERM);
                    }
                    // Give the process time to flush and exit cleanly.
                    let deadline = std::time::Instant::now() + graceful_timeout;
                    loop {
                        match proc.try_wait() {
                            Ok(Some(_)) => break,
                            _ if std::time::Instant::now() >= deadline => {
                                eprintln!("[harness] {} did not exit after SIGTERM, killing", name);
                                let _ = proc.kill();
                                let _ = proc.wait();
                                break;
                            }
                            _ => {
                                std::thread::sleep(std::time::Duration::from_millis(100));
                            }
                        }
                    }
                }
                #[cfg(not(unix))]
                {
                    let _ = name;
                    let _ = proc.kill();
                    let _ = proc.wait();
                }
            }
            *proc_opt = None;
        }
    }
}
