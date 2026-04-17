use thiserror::Error;

/// Errors that can occur during LSP operations.
#[derive(Debug, Error)]
pub enum LspError {
    #[error("failed to spawn LSP server '{server}': {reason}")]
    SpawnFailed { server: String, reason: String },

    #[error("no LSP server registered for extension '{ext}' (file: {path})")]
    NoServerForExtension { ext: String, path: String },

    #[error("all matching LSP servers failed to start for file: {path} ({reason})")]
    AllSpawnsFailed { path: String, reason: String },

    #[error("LSP operation failed: {0}")]
    OperationFailed(String),

    #[error("LSP server binary not found: {binary}")]
    BinaryNotFound { binary: String },

    #[error("LSP configuration error: {0}")]
    Config(String),

    #[error("file not found: {0}")]
    FileNotFound(String),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
}

impl From<LspError> for String {
    fn from(e: LspError) -> Self {
        e.to_string()
    }
}
