pub mod backend;
pub mod compression;
pub mod config;
pub mod error;
pub mod libsql_backend;
pub mod schema;
pub mod writer;

pub use backend::StorageBackend;
pub use config::StorageConfig;
pub use error::StorageError;
pub use libsql_backend::LibSqlBackend;
pub use writer::StorageHandle;

use std::sync::Arc;
use tokio::sync::mpsc;
use tracing::info;

/// Initialise the storage layer from environment variables.
///
/// Returns `None` when `BRIDGE_STORAGE_URL` is not set, meaning persistence
/// is disabled. When configured, this:
/// 1. Opens an embedded libSQL replica with cloud sync
/// 2. Runs schema migrations
/// 3. Spawns the background writer task
/// 4. Returns a backend (for startup reads) and a handle (for fire-and-forget writes)
pub async fn init_storage() -> Result<Option<(Arc<dyn StorageBackend>, StorageHandle)>, StorageError>
{
    let config = match StorageConfig::from_env() {
        Some(c) => c,
        None => {
            info!("BRIDGE_STORAGE_URL not set — persistence disabled");
            return Ok(None);
        }
    };

    let backend = LibSqlBackend::new(&config).await?;
    let backend: Arc<dyn StorageBackend> = Arc::new(backend);

    let (tx, rx) = mpsc::unbounded_channel();
    let handle = StorageHandle::new(tx);

    // Spawn background writer
    let writer_backend = backend.clone();
    tokio::spawn(writer::run_writer(rx, writer_backend));

    info!("storage layer initialized with cloud sync");
    Ok(Some((backend, handle)))
}
