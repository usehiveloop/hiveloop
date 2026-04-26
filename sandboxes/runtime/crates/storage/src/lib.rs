pub mod backend;
pub mod compression;
pub mod config;
pub mod error;
pub mod schema;
pub mod sqlite_backend;
pub mod writer;

pub use backend::{ArtifactUploadRow, ChainLinkRow, JournalEntryRow, StorageBackend};
pub use config::StorageConfig;
pub use error::StorageError;
pub use sqlite_backend::SqliteBackend;
pub use writer::StorageHandle;

use std::sync::Arc;
use tokio::sync::mpsc;
use tracing::info;

/// Initialise the storage layer from environment variables.
///
/// Returns `None` when `BRIDGE_STORAGE_PATH` is not set, meaning persistence
/// is disabled. When configured, this:
/// 1. Opens a local SQLite database
/// 2. Runs schema migrations
/// 3. Spawns the background writer task
/// 4. Returns a backend (for startup reads) and a handle (for fire-and-forget writes)
pub async fn init_storage() -> Result<Option<(Arc<dyn StorageBackend>, StorageHandle)>, StorageError>
{
    let config = match StorageConfig::from_env() {
        Some(c) => c,
        None => {
            info!("BRIDGE_STORAGE_PATH not set — persistence disabled");
            return Ok(None);
        }
    };

    let backend = SqliteBackend::new(&config).await?;
    let backend: Arc<dyn StorageBackend> = Arc::new(backend);

    let (tx, rx) = mpsc::unbounded_channel();
    let handle = StorageHandle::new(tx);

    // Spawn background writer
    let writer_backend = backend.clone();
    tokio::spawn(writer::run_writer(rx, writer_backend));

    info!(path = %config.path, "storage layer initialized");
    Ok(Some((backend, handle)))
}
