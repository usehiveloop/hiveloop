/// Configuration for the optional persistence layer.
///
/// Parsed from `BRIDGE_STORAGE_PATH` environment variable. If not set,
/// the entire persistence layer is disabled.
#[derive(Debug, Clone)]
pub struct StorageConfig {
    /// Path to the SQLite database file. Defaults to `bridge_state.db`.
    pub path: String,
}

impl StorageConfig {
    /// Attempt to build config from environment variables.
    ///
    /// Returns `None` when `BRIDGE_STORAGE_PATH` is absent, meaning persistence
    /// is disabled.
    pub fn from_env() -> Option<Self> {
        let path = std::env::var("BRIDGE_STORAGE_PATH").ok()?;
        Some(Self { path })
    }
}
