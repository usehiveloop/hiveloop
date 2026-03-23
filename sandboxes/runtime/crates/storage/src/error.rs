/// Errors produced by storage operations.
#[derive(Debug, thiserror::Error)]
pub enum StorageError {
    #[error("database error: {0}")]
    Database(String),

    #[error("serialization error: {0}")]
    Serialization(String),

    #[error("compression error: {0}")]
    Compression(String),

    #[error("not configured")]
    NotConfigured,
}

impl From<libsql::Error> for StorageError {
    fn from(e: libsql::Error) -> Self {
        Self::Database(e.to_string())
    }
}

impl From<serde_json::Error> for StorageError {
    fn from(e: serde_json::Error) -> Self {
        Self::Serialization(e.to_string())
    }
}

impl From<std::io::Error> for StorageError {
    fn from(e: std::io::Error) -> Self {
        Self::Compression(e.to_string())
    }
}
