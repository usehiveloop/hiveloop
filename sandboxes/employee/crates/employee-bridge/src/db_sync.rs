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
    pub upload_url: String,
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
        let upload_url = format!(
            "{}/internal/employees/{}/sqlite-backup",
            control_plane_url.trim_end_matches('/'),
            employee_id
        );

        Some(Self {
            db_path,
            upload_url,
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
    let response = Client::new()
        .put(&config.upload_url)
        .bearer_auth(&config.bridge_api_key)
        .header(reqwest::header::CONTENT_TYPE, "application/gzip")
        .body(body)
        .send()
        .await
        .with_context(|| format!("upload sqlite backup to {}", config.upload_url))?;
    if !response.status().is_success() {
        let status = response.status();
        let text = response.text().await.unwrap_or_default();
        anyhow::bail!(
            "sqlite backup upload returned {status} for {reason:?} after {writes} writes: {text}"
        );
    }
    Ok(compressed_bytes)
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
    use flate2::read::GzDecoder;
    use sqlx::SqlitePool;
    use std::io::Read;

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
}
