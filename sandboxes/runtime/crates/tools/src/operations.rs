use std::path::{Path, PathBuf};
use std::time::Duration;

use async_trait::async_trait;
use tokio::io::AsyncReadExt;

#[derive(Debug, thiserror::Error)]
pub enum FsError {
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
}

#[derive(Debug, thiserror::Error)]
pub enum BashError {
    #[error("spawn failed: {0}")]
    Spawn(String),
    #[error("timed out after {0}s")]
    Timeout(u64),
    #[error("non-zero exit: {0}")]
    ExitCode(i32),
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
}

#[async_trait]
pub trait ReadOperations: Send + Sync + 'static {
    async fn read_file(&self, path: &Path) -> Result<Vec<u8>, FsError>;
    async fn access(&self, path: &Path) -> Result<(), FsError>;
    async fn detect_image_mime(&self, path: &Path) -> Option<String>;
}

#[async_trait]
pub trait WriteOperations: Send + Sync + 'static {
    async fn write_file(&self, path: &Path, content: &[u8]) -> Result<(), FsError>;
    async fn mkdir_all(&self, path: &Path) -> Result<(), FsError>;
}

#[async_trait]
pub trait EditOperations: Send + Sync + 'static {
    async fn read_file(&self, path: &Path) -> Result<Vec<u8>, FsError>;
    async fn write_file(&self, path: &Path, content: &[u8]) -> Result<(), FsError>;
    async fn access(&self, path: &Path) -> Result<(), FsError>;
}

pub struct BashExecOptions {
    pub workdir: PathBuf,
    pub env: std::collections::HashMap<String, String>,
    pub timeout: Option<Duration>,
    pub max_output_bytes: u64,
}

pub struct BashExecResult {
    pub stdout_combined: Vec<u8>,
    pub exit_code: Option<i32>,
    pub timed_out: bool,
    pub truncated: bool,
}

#[async_trait]
pub trait BashOperations: Send + Sync + 'static {
    async fn exec(
        &self,
        command: &str,
        options: BashExecOptions,
    ) -> Result<BashExecResult, BashError>;
}

#[derive(Default)]
pub struct LocalFsOperations;

#[async_trait]
impl ReadOperations for LocalFsOperations {
    async fn read_file(&self, path: &Path) -> Result<Vec<u8>, FsError> {
        Ok(tokio::fs::read(path).await?)
    }
    async fn access(&self, path: &Path) -> Result<(), FsError> {
        tokio::fs::metadata(path).await?;
        Ok(())
    }
    async fn detect_image_mime(&self, path: &Path) -> Option<String> {
        let extension = path.extension()?.to_str()?.to_lowercase();
        match extension.as_str() {
            "jpg" | "jpeg" => Some("image/jpeg".into()),
            "png" => Some("image/png".into()),
            "gif" => Some("image/gif".into()),
            "webp" => Some("image/webp".into()),
            _ => None,
        }
    }
}

#[async_trait]
impl WriteOperations for LocalFsOperations {
    async fn write_file(&self, path: &Path, content: &[u8]) -> Result<(), FsError> {
        tokio::fs::write(path, content).await?;
        Ok(())
    }
    async fn mkdir_all(&self, path: &Path) -> Result<(), FsError> {
        tokio::fs::create_dir_all(path).await?;
        Ok(())
    }
}

#[async_trait]
impl EditOperations for LocalFsOperations {
    async fn read_file(&self, path: &Path) -> Result<Vec<u8>, FsError> {
        Ok(tokio::fs::read(path).await?)
    }
    async fn write_file(&self, path: &Path, content: &[u8]) -> Result<(), FsError> {
        tokio::fs::write(path, content).await?;
        Ok(())
    }
    async fn access(&self, path: &Path) -> Result<(), FsError> {
        tokio::fs::metadata(path).await?;
        Ok(())
    }
}

#[derive(Default)]
pub struct LocalBashOperations;

#[async_trait]
impl BashOperations for LocalBashOperations {
    async fn exec(
        &self,
        command: &str,
        options: BashExecOptions,
    ) -> Result<BashExecResult, BashError> {
        let mut child = tokio::process::Command::new("bash")
            .arg("-lc")
            .arg(command)
            .current_dir(&options.workdir)
            .envs(&options.env)
            .stdin(std::process::Stdio::null())
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .kill_on_drop(true)
            .spawn()
            .map_err(|e| BashError::Spawn(e.to_string()))?;

        let mut stdout = child
            .stdout
            .take()
            .ok_or_else(|| BashError::Spawn("stdout".into()))?;
        let mut stderr = child
            .stderr
            .take()
            .ok_or_else(|| BashError::Spawn("stderr".into()))?;
        let max_bytes = options.max_output_bytes as usize;

        let stdout_task = tokio::spawn(async move {
            let mut buffer = Vec::with_capacity(8 * 1024);
            let mut chunk = [0u8; 4096];
            loop {
                match stdout.read(&mut chunk).await {
                    Ok(0) => break,
                    Ok(n) => {
                        if buffer.len() + n > max_bytes {
                            let remaining = max_bytes.saturating_sub(buffer.len());
                            buffer.extend_from_slice(&chunk[..remaining]);
                            break;
                        }
                        buffer.extend_from_slice(&chunk[..n]);
                    }
                    Err(_) => break,
                }
            }
            buffer
        });
        let stderr_task = tokio::spawn(async move {
            let mut buffer = Vec::with_capacity(8 * 1024);
            let mut chunk = [0u8; 4096];
            loop {
                match stderr.read(&mut chunk).await {
                    Ok(0) => break,
                    Ok(n) => {
                        if buffer.len() + n > max_bytes {
                            let remaining = max_bytes.saturating_sub(buffer.len());
                            buffer.extend_from_slice(&chunk[..remaining]);
                            break;
                        }
                        buffer.extend_from_slice(&chunk[..n]);
                    }
                    Err(_) => break,
                }
            }
            buffer
        });

        let wait_result = match options.timeout {
            Some(duration) => {
                // Use tokio::select! to race child.wait() against a timeout.
                // This is more reliable than tokio::time::timeout which can fail
                // to interrupt child.wait() when bash has backgrounded children.
                tokio::select! {
                    result = child.wait() => Ok(result),
                    _ = tokio::time::sleep(duration) => {
                        let _ = child.kill().await;
                        let _ = child.wait().await;
                        Err(BashError::Timeout(duration.as_secs()))
                    }
                }
            }
            None => Ok(child.wait().await),
        };

        // After the child exits, backgrounded processes may still hold stdout/stderr
        // pipes open (e.g. `nohup server &`). Give a short flush window then return
        // whatever we have. This prevents the tool from hanging forever.
        let flush_deadline = Duration::from_secs(5);
        let stdout_bytes = tokio::time::timeout(flush_deadline, stdout_task)
            .await
            .unwrap_or(Ok(Vec::new()))
            .unwrap_or_default();
        let stderr_bytes = tokio::time::timeout(flush_deadline, stderr_task)
            .await
            .unwrap_or(Ok(Vec::new()))
            .unwrap_or_default();
        let mut combined = stdout_bytes;
        combined.extend_from_slice(&stderr_bytes);
        let truncated = combined.len() >= max_bytes;

        let (exit_code, timed_out) = match wait_result {
            Ok(Ok(status)) => (status.code(), false),
            Ok(Err(error)) => return Err(BashError::Io(error)),
            Err(BashError::Timeout(_)) => (None, true),
            Err(other) => return Err(other),
        };

        Ok(BashExecResult {
            stdout_combined: combined,
            exit_code,
            timed_out,
            truncated,
        })
    }
}
