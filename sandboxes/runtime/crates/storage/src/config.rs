/// Configuration for the optional persistence layer.
///
/// Parsed from `BRIDGE_STORAGE_*` environment variables. If `BRIDGE_STORAGE_URL`
/// is not set, the entire persistence layer is disabled.
#[derive(Debug, Clone)]
pub struct StorageConfig {
    /// Remote libSQL / Turso URL (e.g. `libsql://mydb.turso.io`).
    pub url: String,
    /// Auth token for the remote database.
    pub auth_token: String,
    /// Path to the local replica file. Defaults to `bridge_state.db`.
    pub path: String,
    /// How often (in seconds) to sync with the remote. Defaults to 60.
    pub sync_interval_secs: u64,
    /// Optional AES-256 encryption key for the local file.
    pub encryption_key: Option<String>,
}

impl StorageConfig {
    /// Attempt to build config from environment variables.
    ///
    /// Returns `None` when `BRIDGE_STORAGE_URL` is absent, meaning persistence
    /// is disabled.
    pub fn from_env() -> Option<Self> {
        let url = std::env::var("BRIDGE_STORAGE_URL").ok()?;
        Some(Self {
            url,
            auth_token: std::env::var("BRIDGE_STORAGE_AUTH_TOKEN").unwrap_or_default(),
            path: std::env::var("BRIDGE_STORAGE_PATH")
                .unwrap_or_else(|_| "bridge_state.db".to_string()),
            sync_interval_secs: std::env::var("BRIDGE_STORAGE_SYNC_INTERVAL_SECS")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(60),
            encryption_key: std::env::var("BRIDGE_STORAGE_ENCRYPTION_KEY").ok(),
        })
    }
}
