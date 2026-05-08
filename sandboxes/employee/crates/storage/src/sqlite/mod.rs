mod config;
mod cron;
mod dedupe;
mod event;
mod outbox;
mod session;

use std::path::Path;
use std::str::FromStr;
use std::sync::Arc;

use sqlx::sqlite::{SqliteConnectOptions, SqliteJournalMode, SqliteSynchronous};
use sqlx::SqlitePool;

pub use config::SqliteConfigRepo;
pub use cron::SqliteCronJobRepo;
pub use dedupe::SqliteInboundDedupeRepo;
pub use event::SqliteEventRepo;
pub use outbox::SqliteOutboxRepo;
pub use session::SqliteSessionRepo;

pub async fn init_sqlite_pool(database_path: impl AsRef<Path>) -> Result<Arc<SqlitePool>, sqlx::Error> {
    let path = database_path.as_ref();
    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            std::fs::create_dir_all(parent).ok();
        }
    }

    let url = format!("sqlite://{}?mode=rwc", path.display());
    let options = SqliteConnectOptions::from_str(&url)?
        .journal_mode(SqliteJournalMode::Wal)
        .synchronous(SqliteSynchronous::Normal)
        .busy_timeout(std::time::Duration::from_secs(5))
        .create_if_missing(true);

    let pool = sqlx::sqlite::SqlitePoolOptions::new()
        .max_connections(8)
        .connect_with(options)
        .await?;

    sqlx::migrate!("./migrations").run(&pool).await?;
    Ok(Arc::new(pool))
}
