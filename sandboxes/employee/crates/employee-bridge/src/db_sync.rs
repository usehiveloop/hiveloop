use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::str::FromStr;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use flate2::write::GzEncoder;
use flate2::Compression;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use sqlx::sqlite::SqliteConnectOptions;
use sqlx::{Connection, SqliteConnection};
use storage::WriteNotifier;
use tempfile::TempDir;
use tokio::sync::mpsc;
use tracing::{debug, info, warn};

const DEFAULT_WRITE_THRESHOLD: u64 = 1000;
const DEFAULT_INTERVAL_SECONDS: u64 = 900;

#[derive(Clone)]
pub struct DbSyncConfig {
    pub db_path: PathBuf,
    pub presign_url: String,
    pub confirm_url: String,
    pub bridge_api_key: String,
    pub write_threshold: u64,
    pub interval: Duration,
}

impl DbSyncConfig {
    pub fn from_env(db_path: PathBuf) -> Option<Self> {
        if matches!(
            std::env::var("DB_SYNC_ENABLED")
                .unwrap_or_else(|_| "true".to_string())
                .to_ascii_lowercase()
                .as_str(),
            "0" | "false" | "no" | "off"
        ) {
            return None;
        }

        let control_plane_url = std::env::var("CLOUD_CONTROL_PLANE_URL").ok()?;
        let employee_id = std::env::var("EMPLOYEE_ID").ok()?;
        let bridge_api_key = std::env::var("BRIDGE_API_KEY").ok()?;
        let write_threshold =
            parse_u64_env("DB_SYNC_WRITE_THRESHOLD", DEFAULT_WRITE_THRESHOLD).max(1);
        let interval_seconds =
            parse_u64_env("DB_SYNC_INTERVAL_SECONDS", DEFAULT_INTERVAL_SECONDS).max(1);
        let base_url = format!(
            "{}/internal/employees/{}/sqlite-backup",
            control_plane_url.trim_end_matches('/'),
            employee_id
        );

        Some(Self {
            db_path,
            presign_url: format!("{base_url}/presign"),
            confirm_url: format!("{base_url}/confirm"),
            bridge_api_key,
            write_threshold,
            interval: Duration::from_secs(interval_seconds),
        })
    }
}

fn parse_u64_env(key: &str, default: u64) -> u64 {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(default)
}

pub struct DbSyncNotifier {
    pending_writes: AtomicU64,
    sync_running: AtomicBool,
    write_threshold: u64,
    trigger_tx: mpsc::Sender<TriggerReason>,
}

#[derive(Clone, Copy, Debug)]
enum TriggerReason {
    Threshold,
    Interval,
}

impl DbSyncNotifier {
    fn new(write_threshold: u64, trigger_tx: mpsc::Sender<TriggerReason>) -> Self {
        Self {
            pending_writes: AtomicU64::new(0),
            sync_running: AtomicBool::new(false),
            write_threshold,
            trigger_tx,
        }
    }

    fn pending(&self) -> u64 {
        self.pending_writes.load(Ordering::Relaxed)
    }
}

impl WriteNotifier for DbSyncNotifier {
    fn record_write(&self) {
        let pending = self.pending_writes.fetch_add(1, Ordering::Relaxed) + 1;
        if pending >= self.write_threshold {
            let _ = self.trigger_tx.try_send(TriggerReason::Threshold);
        }
    }
}

pub fn spawn_db_sync(config: DbSyncConfig) -> Arc<DbSyncNotifier> {
    let (trigger_tx, trigger_rx) = mpsc::channel(32);
    let notifier = Arc::new(DbSyncNotifier::new(
        config.write_threshold,
        trigger_tx.clone(),
    ));
    let worker = DbSyncWorker {
        config,
        notifier: notifier.clone(),
        trigger_rx,
    };
    tokio::spawn(worker.run());
    notifier
}

struct DbSyncWorker {
    config: DbSyncConfig,
    notifier: Arc<DbSyncNotifier>,
    trigger_rx: mpsc::Receiver<TriggerReason>,
}

impl DbSyncWorker {
    async fn run(mut self) {
        let mut interval = tokio::time::interval(self.config.interval);
        interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
        loop {
            let reason = tokio::select! {
                maybe_reason = self.trigger_rx.recv() => match maybe_reason {
                    Some(reason) => reason,
                    None => return,
                },
                _ = interval.tick() => TriggerReason::Interval,
            };

            if self.notifier.pending() == 0 {
                continue;
            }
            if self.notifier.sync_running.swap(true, Ordering::AcqRel) {
                continue;
            }

            let drained = self.notifier.pending_writes.swap(0, Ordering::AcqRel);
            let result = upload_sqlite_backup(&self.config, reason, drained).await;
            self.notifier.sync_running.store(false, Ordering::Release);
            match result {
                Ok(bytes) => {
                    info!(
                        reason = ?reason,
                        writes = drained,
                        compressed_bytes = bytes,
                        "sqlite backup uploaded"
                    );
                }
                Err(error) => {
                    self.notifier
                        .pending_writes
                        .fetch_add(drained.max(1), Ordering::AcqRel);
                    warn!(reason = ?reason, writes = drained, %error, "sqlite backup upload failed");
                    sentry::configure_scope(|scope| {
                        scope.set_tag("db_sync.reason", format!("{reason:?}"));
                        scope.set_tag("db_sync.writes", drained.to_string());
                    });
                    sentry::capture_error(error.root_cause());
                }
            }
        }
    }
}

async fn upload_sqlite_backup(
    config: &DbSyncConfig,
    reason: TriggerReason,
    writes: u64,
) -> Result<u64> {
    let temp = TempDir::new().context("create db sync temp dir")?;
    let snapshot_path = temp.path().join("employee-bridge.snapshot.db");
    let gzip_path = temp.path().join("employee-bridge.snapshot.db.gz");
    vacuum_into(&config.db_path, &snapshot_path).await?;
    gzip_file(&snapshot_path, &gzip_path).await?;
    let body = tokio::fs::read(&gzip_path)
        .await
        .with_context(|| format!("read compressed backup {}", gzip_path.display()))?;
    let compressed_bytes = body.len() as u64;
    let checksum = sha256_hex(&body);
    let http = Client::new();
    let presign = request_backup_presign(&http, config, reason, compressed_bytes).await?;

    let put_response = http
        .put(&presign.upload_url)
        .header(reqwest::header::CONTENT_TYPE, "application/gzip")
        .body(body)
        .send()
        .await
        .with_context(|| {
            format!(
                "upload sqlite backup to presigned URL for key {}",
                presign.key
            )
        })?;
    if !put_response.status().is_success() {
        let status = put_response.status();
        let text = put_response.text().await.unwrap_or_default();
        anyhow::bail!(
            "sqlite backup direct upload returned {status} for {reason:?} after {writes} writes: {text}"
        );
    }

    let confirm_response = http
        .post(&config.confirm_url)
        .bearer_auth(&config.bridge_api_key)
        .json(&BackupConfirmRequest {
            key: presign.key,
            bytes: compressed_bytes,
            sha256: checksum,
            reason: reason.as_str(),
            writes,
        })
        .send()
        .await
        .with_context(|| format!("confirm sqlite backup at {}", config.confirm_url))?;
    if !confirm_response.status().is_success() {
        let status = confirm_response.status();
        let text = confirm_response.text().await.unwrap_or_default();
        anyhow::bail!(
            "sqlite backup confirm returned {status} for {reason:?} after {writes} writes: {text}"
        );
    }
    Ok(compressed_bytes)
}

#[derive(Serialize)]
struct BackupPresignRequest<'a> {
    reason: &'a str,
    content_type: &'a str,
    compressed_bytes_hint: u64,
}

#[derive(Deserialize)]
struct BackupPresignResponse {
    key: String,
    upload_url: String,
}

#[derive(Serialize)]
struct BackupConfirmRequest<'a> {
    key: String,
    bytes: u64,
    sha256: String,
    reason: &'a str,
    writes: u64,
}

async fn request_backup_presign(
    http: &Client,
    config: &DbSyncConfig,
    reason: TriggerReason,
    compressed_bytes: u64,
) -> Result<BackupPresignResponse> {
    let response = http
        .post(&config.presign_url)
        .bearer_auth(&config.bridge_api_key)
        .json(&BackupPresignRequest {
            reason: reason.as_str(),
            content_type: "application/gzip",
            compressed_bytes_hint: compressed_bytes,
        })
        .send()
        .await
        .with_context(|| format!("request sqlite backup presign from {}", config.presign_url))?;
    if !response.status().is_success() {
        let status = response.status();
        let text = response.text().await.unwrap_or_default();
        anyhow::bail!("sqlite backup presign returned {status} for {reason:?}: {text}");
    }
    response
        .json::<BackupPresignResponse>()
        .await
        .context("decode sqlite backup presign response")
}

impl TriggerReason {
    fn as_str(self) -> &'static str {
        match self {
            TriggerReason::Threshold => "threshold",
            TriggerReason::Interval => "interval",
        }
    }
}

fn sha256_hex(body: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(body);
    hex::encode(hasher.finalize())
}

async fn vacuum_into(db_path: &Path, snapshot_path: &Path) -> Result<()> {
    let db_url = format!("sqlite://{}?mode=rw", db_path.display());
    let options = SqliteConnectOptions::from_str(&db_url)
        .with_context(|| format!("open sqlite options for {}", db_path.display()))?;
    let mut conn = SqliteConnection::connect_with(&options)
        .await
        .with_context(|| format!("connect sqlite {}", db_path.display()))?;
    let target = sql_quote_path(snapshot_path);
    let sql = format!("VACUUM main INTO {target}");
    debug!(target = %snapshot_path.display(), "creating sqlite backup snapshot");
    sqlx::query(&sql)
        .execute(&mut conn)
        .await
        .with_context(|| format!("vacuum sqlite into {}", snapshot_path.display()))?;
    Ok(())
}

fn sql_quote_path(path: &Path) -> String {
    let escaped = path.to_string_lossy().replace('\'', "''");
    format!("'{escaped}'")
}

async fn gzip_file(input: &Path, output: &Path) -> Result<()> {
    let input = input.to_path_buf();
    let output = output.to_path_buf();
    tokio::task::spawn_blocking(move || -> Result<()> {
        let mut reader = std::fs::File::open(&input)
            .with_context(|| format!("open snapshot {}", input.display()))?;
        let writer = std::fs::File::create(&output)
            .with_context(|| format!("create gzip {}", output.display()))?;
        let mut encoder = GzEncoder::new(writer, Compression::default());
        let mut buffer = [0u8; 64 * 1024];
        loop {
            let read = reader
                .read(&mut buffer)
                .with_context(|| format!("read snapshot {}", input.display()))?;
            if read == 0 {
                break;
            }
            encoder
                .write_all(&buffer[..read])
                .with_context(|| format!("write gzip {}", output.display()))?;
        }
        encoder
            .finish()
            .with_context(|| format!("finish gzip {}", output.display()))?;
        Ok(())
    })
    .await
    .context("join gzip task")?
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::body::Bytes;
    use axum::extract::State;
    use axum::http::{HeaderMap, StatusCode};
    use axum::routing::{post, put};
    use axum::{Json, Router};
    use flate2::read::GzDecoder;
    use serde_json::{json, Value};
    use sqlx::SqlitePool;
    use std::io::Read;
    use std::sync::Arc;
    use tokio::sync::Mutex;

    #[test]
    fn default_write_threshold_is_raised_for_production_backup_load() {
        assert_eq!(DEFAULT_WRITE_THRESHOLD, 1000);
    }

    #[tokio::test]
    async fn sqlite_snapshot_gzip_contains_recent_wal_writes() {
        let temp = TempDir::new().expect("temp dir");
        let db_path = temp.path().join("test.db");
        let url = format!("sqlite://{}?mode=rwc", db_path.display());
        let pool = SqlitePool::connect(&url).await.expect("connect sqlite");
        sqlx::query("PRAGMA journal_mode = WAL")
            .execute(&pool)
            .await
            .expect("wal");
        sqlx::query("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
            .execute(&pool)
            .await
            .expect("create table");
        sqlx::query("INSERT INTO items (name) VALUES ('recent')")
            .execute(&pool)
            .await
            .expect("insert");

        let snapshot = temp.path().join("snapshot.db");
        let gzip = temp.path().join("snapshot.db.gz");
        vacuum_into(&db_path, &snapshot).await.expect("vacuum");
        gzip_file(&snapshot, &gzip).await.expect("gzip");

        let mut decoded = Vec::new();
        let file = std::fs::File::open(&gzip).expect("open gzip");
        GzDecoder::new(file)
            .read_to_end(&mut decoded)
            .expect("decode gzip");
        let restored = temp.path().join("restored.db");
        std::fs::write(&restored, decoded).expect("write restored");
        let restored_url = format!("sqlite://{}?mode=ro", restored.display());
        let restored_pool = SqlitePool::connect(&restored_url)
            .await
            .expect("connect restored");
        let count: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM items WHERE name = 'recent'")
            .fetch_one(&restored_pool)
            .await
            .expect("count");
        assert_eq!(count, 1);
    }

    #[tokio::test]
    async fn notifier_triggers_after_threshold() {
        let (tx, mut rx) = mpsc::channel(4);
        let notifier = DbSyncNotifier::new(2, tx);
        notifier.record_write();
        assert!(rx.try_recv().is_err());
        notifier.record_write();
        assert!(matches!(rx.recv().await, Some(TriggerReason::Threshold)));
    }

    #[tokio::test]
    async fn backup_sync_uses_presigned_storage_upload_then_confirms() {
        let temp = TempDir::new().expect("temp dir");
        let db_path = temp.path().join("sync.db");
        let url = format!("sqlite://{}?mode=rwc", db_path.display());
        let pool = SqlitePool::connect(&url).await.expect("connect sqlite");
        sqlx::query("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
            .execute(&pool)
            .await
            .expect("create table");
        sqlx::query("INSERT INTO items (name) VALUES ('business-event')")
            .execute(&pool)
            .await
            .expect("insert");
        drop(pool);

        let state = TestBackupServerState::default();
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind test server");
        let base_url = format!("http://{}", listener.local_addr().expect("local addr"));
        state.base_url.lock().await.clone_from(&base_url);
        let app = Router::new()
            .route("/presign", post(test_presign))
            .route("/upload", put(test_upload))
            .route("/confirm", post(test_confirm))
            .with_state(state.clone());
        let server = tokio::spawn(async move {
            axum::serve(listener, app)
                .await
                .expect("serve test backup server");
        });

        let config = DbSyncConfig {
            db_path,
            presign_url: format!("{base_url}/presign"),
            confirm_url: format!("{base_url}/confirm"),
            bridge_api_key: "bridge-secret".to_string(),
            write_threshold: 1000,
            interval: Duration::from_secs(60),
        };
        let bytes = upload_sqlite_backup(&config, TriggerReason::Threshold, 1000)
            .await
            .expect("upload sqlite backup");
        assert!(bytes > 0);

        let events = state.events.lock().await.clone();
        assert_eq!(events, vec!["presign", "upload", "confirm"]);
        let confirm = state.confirm_body.lock().await.clone();
        assert_eq!(
            confirm["key"],
            "employee-sqlite-backups/org/agent/latest.db.gz"
        );
        assert_eq!(confirm["reason"], "threshold");
        assert_eq!(confirm["writes"], 1000);
        assert_eq!(confirm["bytes"].as_u64().unwrap(), bytes);
        assert!(confirm["sha256"].as_str().unwrap().len() == 64);
        assert!(state.uploaded_bytes.lock().await.len() as u64 == bytes);

        server.abort();
    }

    #[derive(Clone, Default)]
    struct TestBackupServerState {
        base_url: Arc<Mutex<String>>,
        events: Arc<Mutex<Vec<&'static str>>>,
        uploaded_bytes: Arc<Mutex<Vec<u8>>>,
        confirm_body: Arc<Mutex<Value>>,
    }

    async fn test_presign(
        State(state): State<TestBackupServerState>,
        headers: HeaderMap,
        Json(body): Json<Value>,
    ) -> Result<Json<Value>, StatusCode> {
        if headers.get("authorization").and_then(|v| v.to_str().ok())
            != Some("Bearer bridge-secret")
        {
            return Err(StatusCode::UNAUTHORIZED);
        }
        if body["compressed_bytes_hint"].as_u64().unwrap_or(0) == 0 {
            return Err(StatusCode::BAD_REQUEST);
        }
        state.events.lock().await.push("presign");
        let base_url = state.base_url.lock().await.clone();
        Ok(Json(json!({
            "status": "ok",
            "method": "PUT",
            "key": "employee-sqlite-backups/org/agent/latest.db.gz",
            "upload_url": format!("{base_url}/upload")
        })))
    }

    async fn test_upload(
        State(state): State<TestBackupServerState>,
        headers: HeaderMap,
        body: Bytes,
    ) -> Result<StatusCode, StatusCode> {
        if headers.get("content-type").and_then(|v| v.to_str().ok()) != Some("application/gzip") {
            return Err(StatusCode::BAD_REQUEST);
        }
        state.events.lock().await.push("upload");
        *state.uploaded_bytes.lock().await = body.to_vec();
        Ok(StatusCode::OK)
    }

    async fn test_confirm(
        State(state): State<TestBackupServerState>,
        headers: HeaderMap,
        Json(body): Json<Value>,
    ) -> Result<Json<Value>, StatusCode> {
        if headers.get("authorization").and_then(|v| v.to_str().ok())
            != Some("Bearer bridge-secret")
        {
            return Err(StatusCode::UNAUTHORIZED);
        }
        state.events.lock().await.push("confirm");
        *state.confirm_body.lock().await = body;
        Ok(Json(json!({"status": "ok"})))
    }
}
