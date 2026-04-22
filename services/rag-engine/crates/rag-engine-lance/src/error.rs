//! Typed errors for the Lance storage layer.
//!
//! Callers (`rag-engine-server`) translate these into gRPC status codes.

use thiserror::Error;

#[derive(Debug, Error)]
pub enum LanceStoreError {
    #[error("dataset not found: {0}")]
    DatasetNotFound(String),

    #[error("dataset already exists with incompatible schema: {0}")]
    SchemaMismatch(String),

    #[error("invalid argument: {0}")]
    InvalidArgument(String),

    #[error("vector dimension mismatch: dataset expects {expected}, got {got}")]
    VectorDimMismatch { expected: usize, got: usize },

    #[error("upstream lancedb error: {0}")]
    LanceDb(#[from] lancedb::Error),

    #[error("arrow error: {0}")]
    Arrow(#[from] arrow_schema::ArrowError),

    #[error("storage connect error: {0}")]
    Connect(String),

    #[error("internal: {0}")]
    Internal(String),
}

pub type Result<T> = std::result::Result<T, LanceStoreError>;
