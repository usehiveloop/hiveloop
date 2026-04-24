use std::time::Duration;
use tokio::io::AsyncReadExt;

use super::args::BashResult;
use super::rtk;
use super::truncate::truncate_output;

/// Kill the entire process group on Unix to prevent orphaned children.
#[cfg(unix)]
fn kill_process_tree(child: &tokio::process::Child) {
    if let Some(pid) = child.id() {
        // Kill the entire process group (negative pid)
        unsafe {
            libc::kill(-(pid as i32), libc::SIGKILL);
        }
    }
}

/// Execute a bash command and return the result.
/// Public so it can be called from the hook layer for background execution.
pub async fn run_command(
    command: &str,
    workdir: &str,
    timeout_ms: u64,
) -> Result<BashResult, String> {
    // If rtk is installed and enabled, route through `rtk rewrite` so commands
    // like `git status`, `composer install`, `php artisan test`, etc. get
    // wrapped in the rtk filter pipeline. Falls back to the original command
    // on any failure — the bash tool must never refuse to run because rtk did.
    let effective = rtk::rewrite(command).await;
    let command_ref: &str = &effective;

    let mut cmd = tokio::process::Command::new("sh");
    cmd.arg("-c").arg(command_ref);
    cmd.current_dir(workdir);
    cmd.stdin(std::process::Stdio::null());
    cmd.stdout(std::process::Stdio::piped());
    cmd.stderr(std::process::Stdio::piped());

    // Make child a process group leader so we can kill the whole tree
    #[cfg(unix)]
    cmd.process_group(0);

    let mut child = cmd
        .spawn()
        .map_err(|e| format!("Failed to spawn command: {e}"))?;

    let stdout = child.stdout.take();
    let stderr = child.stderr.take();

    // Read stdout and stderr concurrently using tokio::join! to prevent deadlock
    let read_output = async {
        let (stdout_buf, stderr_buf) = tokio::join!(
            async {
                let mut buf = Vec::new();
                if let Some(mut out) = stdout {
                    let _ = out.read_to_end(&mut buf).await;
                }
                buf
            },
            async {
                let mut buf = Vec::new();
                if let Some(mut err) = stderr {
                    let _ = err.read_to_end(&mut buf).await;
                }
                buf
            }
        );

        let mut combined = Vec::new();
        combined.extend_from_slice(&stdout_buf);
        if !stderr_buf.is_empty() {
            if !combined.is_empty() && !combined.ends_with(b"\n") {
                combined.push(b'\n');
            }
            combined.extend_from_slice(&stderr_buf);
        }

        combined
    };

    let timeout_duration = Duration::from_millis(timeout_ms);

    match tokio::time::timeout(timeout_duration, async {
        let output = read_output.await;
        let status = child.wait().await;
        (output, status)
    })
    .await
    {
        Ok((output, status)) => {
            let exit_code = status.ok().and_then(|s| s.code());
            let output_str = truncate_output(&output);

            Ok(BashResult {
                output: output_str,
                exit_code,
                timed_out: false,
            })
        }
        Err(_) => {
            // Timeout — kill the process group, then the process
            #[cfg(unix)]
            kill_process_tree(&child);
            let _ = child.kill().await;

            Ok(BashResult {
                output: "[timed out]".to_string(),
                exit_code: None,
                timed_out: true,
            })
        }
    }
}
