//! `upload_to_workspace` — streams a sandbox file to the control plane via
//! resumable tus.io chunks. Tolerates network failures (backoff retry),
//! server offset drift (re-HEAD + realign), and bridge crashes (state
//! persisted in sqlite, resumed on next call by idempotency key).

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use backon::{ExponentialBuilder, Retryable};
use bridge_core::artifacts::ArtifactsConfig;
use bytes::{Bytes, BytesMut};
use schemars::JsonSchema;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use storage::{ArtifactUploadRow, StorageBackend};
use tokio::fs::File;
use tokio::io::{AsyncReadExt, AsyncSeekExt, SeekFrom};
use tokio::sync::Semaphore;
use tracing::{debug, warn};

use crate::boundary::ProjectBoundary;
use crate::ToolExecutor;

mod tus;

#[cfg(test)]
mod tests;

use tus::{TusClient, TusError};

/// Public tool name. Stable; baked into the LLM prompt.
pub const TOOL_NAME: &str = "upload_to_workspace";

const DEFAULT_RETRY_MAX: usize = 6;
const DEFAULT_RETRY_MIN_DELAY: Duration = Duration::from_millis(250);
const DEFAULT_RETRY_MAX_DELAY: Duration = Duration::from_secs(30);

#[derive(Debug, Deserialize, JsonSchema)]
pub struct UploadArgs {
    /// Absolute path to the file inside the agent's sandbox.
    #[schemars(description = "Absolute path to the file inside the agent's sandbox")]
    pub path: String,
    /// Optional MIME type override. If omitted, bridge guesses from the
    /// file extension.
    #[schemars(description = "Optional MIME type override (e.g. 'text/csv', 'video/mp4')")]
    pub content_type: Option<String>,
    /// Optional free-form metadata forwarded to the control plane as
    /// upload-metadata. Keys must be ASCII without spaces or commas.
    #[schemars(description = "Optional free-form key/value metadata sent with the upload")]
    pub metadata: Option<HashMap<String, String>>,
}

pub struct UploadToWorkspaceTool {
    config: ArtifactsConfig,
    agent_id: String,
    boundary: Option<ProjectBoundary>,
    storage: Option<Arc<dyn StorageBackend>>,
    tus: TusClient,
    semaphore: Arc<Semaphore>,
    description: String,
}

impl UploadToWorkspaceTool {
    pub fn new(
        config: ArtifactsConfig,
        agent_id: String,
        bearer_token: Option<String>,
        boundary: Option<ProjectBoundary>,
        storage: Option<Arc<dyn StorageBackend>>,
    ) -> Self {
        let semaphore = Arc::new(Semaphore::new(
            config.effective_max_concurrent_uploads() as usize,
        ));
        let tus = TusClient::new(config.headers.clone(), bearer_token);
        let description = build_description(&config);
        Self {
            config,
            agent_id,
            boundary,
            storage,
            tus,
            semaphore,
            description,
        }
    }
}

fn build_description(cfg: &ArtifactsConfig) -> String {
    let mut accepted = cfg.accepted_file_types.clone();
    accepted.sort();
    format!(
        "Upload a file from your sandbox to the workspace. The file is streamed in resumable \
         chunks; bridge handles retry and resume on transient failures. Accepted types: {}. \
         Max size: {} bytes.",
        accepted.join(", "),
        cfg.max_size_bytes
    )
}

#[async_trait]
impl ToolExecutor for UploadToWorkspaceTool {
    fn name(&self) -> &str {
        TOOL_NAME
    }

    fn description(&self) -> &str {
        &self.description
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(UploadArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: UploadArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let _permit = self
            .semaphore
            .clone()
            .acquire_owned()
            .await
            .map_err(|e| format!("upload semaphore closed: {e}"))?;

        self.run(args).await
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

impl UploadToWorkspaceTool {
    async fn run(&self, args: UploadArgs) -> Result<String, String> {
        // 1. Path resolution + sandbox boundary check.
        let resolved = if let Some(boundary) = &self.boundary {
            boundary.check(&args.path)?
        } else {
            std::path::PathBuf::from(&args.path)
        };
        let abs_path = resolved.to_string_lossy().to_string();

        // 2. Stat + size validation.
        let meta = tokio::fs::metadata(&resolved)
            .await
            .map_err(|e| format!("File '{}' is not readable: {e}", abs_path))?;
        if !meta.is_file() {
            return Err(format!("'{}' is not a regular file", abs_path));
        }
        let size = meta.len();
        if size == 0 {
            return Err(format!("'{}' is empty; refusing to upload", abs_path));
        }
        if size > self.config.max_size_bytes {
            return Err(format!(
                "'{}' is {} bytes, exceeds artifacts.max_size_bytes ({} bytes)",
                abs_path, size, self.config.max_size_bytes
            ));
        }

        // 3. MIME + extension validation.
        let extension = std::path::Path::new(&abs_path)
            .extension()
            .and_then(|s| s.to_str())
            .map(|s| s.to_string());
        let content_type = args.content_type.clone().unwrap_or_else(|| {
            mime_guess::from_path(&resolved)
                .first_raw()
                .unwrap_or("application/octet-stream")
                .to_string()
        });
        if !self
            .config
            .is_accepted(&content_type, extension.as_deref())
        {
            return Err(format!(
                "File type rejected: content_type='{}', extension={:?}, accepted={:?}",
                content_type, extension, self.config.accepted_file_types
            ));
        }

        // 4. Streaming SHA-256 of the full file (used for idempotency +
        //    integrity guard against the file mutating between calls).
        let file_sha256 = hash_file(&resolved).await?;

        // 5. Idempotency key. Scoped to (agent, path, sha256) so the same
        //    file uploaded from any conversation deduplicates to a single
        //    server-side artifact and any in-flight crash is resumable.
        let idempotency_key = derive_idempotency_key(&self.agent_id, &abs_path, &file_sha256);

        // 6. Resume-or-create flow against storage + control plane.
        let (location, mut bytes_sent) = self
            .resume_or_create(
                &idempotency_key,
                size,
                &file_sha256,
                &content_type,
                &abs_path,
                &args.metadata,
            )
            .await?;

        // Already finished on a previous call — return the cached response.
        if bytes_sent >= size {
            if let Some(cached) = self.cached_response(&idempotency_key).await {
                return Ok(cached);
            }
        }

        // 8. Chunk loop with retry + offset realign.
        let chunk_size = self.config.effective_chunk_size().max(1);
        let mut file = File::open(&resolved)
            .await
            .map_err(|e| format!("open '{}': {e}", abs_path))?;

        while bytes_sent < size {
            let remaining = size - bytes_sent;
            let this_chunk_len = std::cmp::min(chunk_size, remaining);
            let chunk = read_chunk(&mut file, bytes_sent, this_chunk_len).await?;

            let location_for_retry = location.clone();
            let tus = self.tus.clone();
            let current_offset = bytes_sent;

            let attempt = (move || {
                let chunk = chunk.clone();
                let location = location_for_retry.clone();
                let tus = tus.clone();
                async move { tus.patch_chunk(&location, current_offset, chunk).await }
            })
            .retry(
                ExponentialBuilder::default()
                    .with_min_delay(DEFAULT_RETRY_MIN_DELAY)
                    .with_max_delay(DEFAULT_RETRY_MAX_DELAY)
                    .with_max_times(DEFAULT_RETRY_MAX)
                    .with_jitter(),
            )
            .when(|e: &TusError| e.is_transient())
            .await;

            match attempt {
                Ok(ack) => {
                    bytes_sent = ack.upload_offset;
                    self.persist_offset(&idempotency_key, bytes_sent).await;
                    debug!(
                        idempotency_key = %idempotency_key,
                        bytes_sent,
                        size,
                        "artifact chunk acked"
                    );
                }
                Err(TusError::Mismatch { server, .. }) => {
                    warn!(
                        idempotency_key = %idempotency_key,
                        client_offset = current_offset,
                        server_offset = server,
                        "TUS offset mismatch — realigning"
                    );
                    let authoritative = match self.tus.head(&location).await {
                        Ok(v) => v,
                        Err(e) => {
                            self.persist_failure(&idempotency_key, &e.to_string()).await;
                            return Err(format!("HEAD after mismatch failed: {e}"));
                        }
                    };
                    bytes_sent = authoritative;
                    self.persist_offset(&idempotency_key, bytes_sent).await;
                    // Reseek and retry from the new offset on the next loop iteration.
                    if bytes_sent >= size {
                        break;
                    }
                }
                Err(e) => {
                    self.persist_failure(&idempotency_key, &e.to_string()).await;
                    return Err(format!("upload failed: {e}"));
                }
            }
        }

        // 9. Build the result we hand back to the agent.
        let result = serde_json::json!({
            "artifact_id": idempotency_key,
            "upload_url": location,
            "download_url": self.config.download_url,
            "size": size,
            "content_type": content_type,
            "sha256": file_sha256,
        });
        let result_str = result.to_string();
        self.persist_completion(&idempotency_key, bytes_sent, &result_str).await;
        Ok(result_str)
    }

    async fn resume_or_create(
        &self,
        idempotency_key: &str,
        total_size: u64,
        file_sha256: &str,
        _content_type: &str,
        abs_path: &str,
        metadata: &Option<HashMap<String, String>>,
    ) -> Result<(String, u64), String> {
        // Fast path: do we already have a row?
        if let Some(storage) = &self.storage {
            if let Ok(Some(existing)) = storage.get_artifact_upload(idempotency_key).await {
                if existing.status == "completed" && existing.total_size == total_size {
                    // Caller will check `bytes_sent >= size` and return cached response.
                    return Ok((existing.location, existing.bytes_sent));
                }
                if existing.status == "in_progress"
                    && existing.total_size == total_size
                    && existing.file_sha256 == file_sha256
                {
                    let server_offset = self
                        .tus
                        .head(&existing.location)
                        .await
                        .map_err(|e| format!("HEAD on resume failed: {e}"))?;
                    let bytes_sent = std::cmp::min(server_offset, total_size);
                    self.persist_offset(idempotency_key, bytes_sent).await;
                    return Ok((existing.location, bytes_sent));
                }
                // Stale or mismatched row — fall through and create a fresh upload.
            }
        }

        // Create on the control plane.
        let mut meta_map: HashMap<String, String> = metadata.clone().unwrap_or_default();
        meta_map.insert("filename".into(), file_name_from_path(abs_path));
        meta_map.insert("sha256".into(), file_sha256.to_string());

        let location = self
            .tus
            .create(&self.config.upload_url, total_size, &meta_map)
            .await
            .map_err(|e| format!("create upload failed: {e}"))?;

        if let Some(storage) = &self.storage {
            let row = ArtifactUploadRow {
                idempotency_key: idempotency_key.to_string(),
                agent_id: self.agent_id.clone(),
                conversation_id: String::new(),
                location: location.clone(),
                total_size,
                file_sha256: file_sha256.to_string(),
                bytes_sent: 0,
                status: "in_progress".into(),
                response_json: None,
                last_error: None,
                created_at: String::new(),
                updated_at: String::new(),
            };
            if let Err(e) = storage.upsert_artifact_upload_in_progress(row).await {
                warn!(error = %e, "persist artifact upload row failed (continuing without)");
            }
        }
        Ok((location, 0))
    }

    async fn cached_response(&self, idempotency_key: &str) -> Option<String> {
        let storage = self.storage.as_ref()?;
        let row = storage.get_artifact_upload(idempotency_key).await.ok()??;
        row.response_json
    }

    async fn persist_offset(&self, idempotency_key: &str, bytes_sent: u64) {
        if let Some(storage) = &self.storage {
            if let Err(e) = storage
                .update_artifact_upload_offset(idempotency_key, bytes_sent)
                .await
            {
                warn!(error = %e, "persist artifact offset failed");
            }
        }
    }

    async fn persist_failure(&self, idempotency_key: &str, error: &str) {
        if let Some(storage) = &self.storage {
            if let Err(e) = storage
                .mark_artifact_upload_failed(idempotency_key, error)
                .await
            {
                warn!(error = %e, "persist artifact failure failed");
            }
        }
    }

    async fn persist_completion(
        &self,
        idempotency_key: &str,
        bytes_sent: u64,
        response_json: &str,
    ) {
        if let Some(storage) = &self.storage {
            if let Err(e) = storage
                .mark_artifact_upload_completed(idempotency_key, bytes_sent, response_json)
                .await
            {
                warn!(error = %e, "persist artifact completion failed");
            }
        }
    }
}

async fn read_chunk(file: &mut File, offset: u64, len: u64) -> Result<Bytes, String> {
    file.seek(SeekFrom::Start(offset))
        .await
        .map_err(|e| format!("seek({offset}): {e}"))?;
    let mut buf = BytesMut::with_capacity(len as usize);
    buf.resize(len as usize, 0);
    file.read_exact(&mut buf[..])
        .await
        .map_err(|e| format!("read_exact at {offset} for {len} bytes: {e}"))?;
    Ok(buf.freeze())
}

async fn hash_file(path: &std::path::Path) -> Result<String, String> {
    let mut file = File::open(path)
        .await
        .map_err(|e| format!("open '{}' for hashing: {e}", path.display()))?;
    let mut hasher = Sha256::new();
    let mut buf = vec![0u8; 64 * 1024];
    loop {
        let n = file
            .read(&mut buf)
            .await
            .map_err(|e| format!("hash read: {e}"))?;
        if n == 0 {
            break;
        }
        hasher.update(&buf[..n]);
    }
    Ok(hex::encode(hasher.finalize()))
}

/// Idempotency key used both as the primary key in sqlite and as the
/// `artifact_id` returned to the agent. Stable across process restarts.
fn derive_idempotency_key(agent_id: &str, abs_path: &str, file_sha256: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(agent_id.as_bytes());
    hasher.update(b"\0");
    hasher.update(abs_path.as_bytes());
    hasher.update(b"\0");
    hasher.update(file_sha256.as_bytes());
    hex::encode(hasher.finalize())
}

fn file_name_from_path(abs_path: &str) -> String {
    std::path::Path::new(abs_path)
        .file_name()
        .and_then(|s| s.to_str())
        .map(str::to_string)
        .unwrap_or_else(|| "artifact".to_string())
}

