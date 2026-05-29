use std::path::Path;
use std::str::FromStr;
use std::sync::Arc;

use sqlx::sqlite::{SqliteConnectOptions, SqliteJournalMode, SqlitePoolOptions, SqliteSynchronous};
use sqlx::{Executor, SqlitePool};

use crate::repos::{Result, SharedWriteNotifier, StorageError};

use super::write_gateway::SqliteWriteGateway;

#[derive(Clone)]
pub struct SqliteStore {
    read_pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteStore {
    pub fn read_pool(&self) -> Arc<SqlitePool> {
        self.read_pool.clone()
    }

    pub fn writer(&self) -> Arc<SqliteWriteGateway> {
        self.writer.clone()
    }

    pub async fn flush_writes(&self) -> Result<()> {
        self.writer.flush().await
    }
}

pub async fn init_sqlite_store(
    database_path: impl AsRef<Path>,
    write_notifier: Option<SharedWriteNotifier>,
) -> Result<SqliteStore> {
    let path = database_path.as_ref();
    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            std::fs::create_dir_all(parent).ok();
        }
    }

    let write_options = sqlite_options(path, "rwc")?;
    let setup_pool = SqlitePoolOptions::new()
        .max_connections(1)
        .connect_with(write_options.clone())
        .await
        .map_err(StorageError::from)?;
    configure_setup_pool(&setup_pool).await?;
    sqlx::migrate!("./migrations")
        .run(&setup_pool)
        .await
        .map_err(|error| StorageError::Other(anyhow::anyhow!(error)))?;
    setup_pool.close().await;

    let writer = SqliteWriteGateway::spawn(write_options, write_notifier).await?;

    let read_options = sqlite_options(path, "ro")?;
    let read_pool = SqlitePoolOptions::new()
        .max_connections(16)
        .after_connect(|conn, _meta| {
            Box::pin(async move {
                conn.execute("PRAGMA foreign_keys = ON").await?;
                conn.execute("PRAGMA busy_timeout = 30000").await?;
                conn.execute("PRAGMA query_only = ON").await?;
                Ok(())
            })
        })
        .connect_with(read_options)
        .await
        .map_err(StorageError::from)?;

    Ok(SqliteStore {
        read_pool: Arc::new(read_pool),
        writer,
    })
}

fn sqlite_options(path: &Path, mode: &str) -> Result<SqliteConnectOptions> {
    let url = format!("sqlite://{}?mode={mode}", path.display());
    Ok(SqliteConnectOptions::from_str(&url)?
        .journal_mode(SqliteJournalMode::Wal)
        .synchronous(SqliteSynchronous::Normal)
        .busy_timeout(std::time::Duration::from_secs(30))
        .create_if_missing(mode == "rwc"))
}

async fn configure_setup_pool(pool: &SqlitePool) -> Result<()> {
    pool.execute("PRAGMA journal_mode = WAL").await?;
    pool.execute("PRAGMA synchronous = NORMAL").await?;
    pool.execute("PRAGMA foreign_keys = ON").await?;
    pool.execute("PRAGMA busy_timeout = 30000").await?;
    Ok(())
}
