//! Domain → gRPC status code mapping.
//!
//! # Policy
//!
//! * `LanceStoreError` → the storage layer's typed errors translate
//!   directly into their gRPC equivalents. `DatasetNotFound` → `NOT_FOUND`,
//!   `SchemaMismatch` → `FAILED_PRECONDITION`, `VectorDimMismatch` /
//!   `InvalidArgument` → `INVALID_ARGUMENT`, `Connect` → `UNAVAILABLE`,
//!   `Arrow` / `Internal` → `INTERNAL`. The underlying `LanceDb` variant
//!   is a catch-all for anything upstream — we pattern-match on the
//!   `Display` string for a handful of well-known shapes (e.g. "not
//!   found", "table not found") before falling back to `INTERNAL`.
//!
//! * `EmbedError` → `RateLimited` → `RESOURCE_EXHAUSTED`, `Upstream` /
//!   `Transport` → `UNAVAILABLE` (the upstream provider is the problem,
//!   not our request shape), `InvalidResponse` → `INTERNAL` (we got a
//!   successful HTTP response whose body didn't parse — that's our bug
//!   or the provider's API drift, not something the caller can fix),
//!   `Config` → `FAILED_PRECONDITION` (server misconfigured).
//!
//! * `RerankError` → `Http` / `Timeout` → `UNAVAILABLE`, `UpstreamStatus`
//!   mapped by status class, `Decode` → `INTERNAL`, `Invalid` →
//!   `INVALID_ARGUMENT`, `Config` → `FAILED_PRECONDITION`.
//!
//! * Internal panics do NOT reach here — Tranche 2H installs a panic
//!   handler at the process level. Per-RPC `Internal` errors carry an
//!   opaque message ("internal error; see server logs") with the real
//!   detail dumped into the tracing span so ops can grep for it without
//!   leaking to untrusted callers.
//!
//! # Why a dedicated module?
//!
//! Keeping the mapping in one place means new error kinds added in 2B /
//! 2C / 2D land in exactly one match arm. Handlers call
//! `to_grpc_status(err)` and move on — they never construct a `Status`
//! directly, so the mapping can't drift between RPCs.

use rag_engine_embed::EmbedError;
use rag_engine_lance::LanceStoreError;
use rag_engine_rerank::RerankError;
use tonic::{Code, Status};
use tracing::{debug, warn};

/// Translate a `LanceStoreError` into a `tonic::Status`.
pub fn lance_to_status(err: LanceStoreError) -> Status {
    match err {
        LanceStoreError::DatasetNotFound(msg) => Status::new(Code::NotFound, msg),
        LanceStoreError::SchemaMismatch(msg) => Status::new(Code::FailedPrecondition, msg),
        LanceStoreError::InvalidArgument(msg) => Status::new(Code::InvalidArgument, msg),
        LanceStoreError::VectorDimMismatch { expected, got } => Status::new(
            Code::InvalidArgument,
            format!("vector dim mismatch: expected {expected}, got {got}"),
        ),
        LanceStoreError::Connect(msg) => {
            warn!(%msg, "lance backend unreachable");
            Status::new(Code::Unavailable, "storage backend unavailable")
        }
        LanceStoreError::Arrow(e) => {
            warn!(error = %e, "arrow serialization failure");
            Status::new(Code::Internal, "internal storage format error")
        }
        LanceStoreError::Internal(msg) => {
            warn!(%msg, "lance internal error");
            Status::new(Code::Internal, "internal storage error")
        }
        LanceStoreError::LanceDb(e) => {
            let s = e.to_string();
            let lower = s.to_ascii_lowercase();
            // LanceDB surfaces "table X was not found" and "dataset not
            // found" as typed errors in some paths but as untyped strings
            // in others. Do a best-effort match so callers get NOT_FOUND
            // for the obvious cases.
            if lower.contains("not found") || lower.contains("does not exist") {
                debug!(error = %s, "lance: treating as NOT_FOUND");
                return Status::new(Code::NotFound, format!("dataset or object not found: {s}"));
            }
            if lower.contains("timeout")
                || lower.contains("connection")
                || lower.contains("dispatch")
            {
                warn!(error = %s, "lance: treating as UNAVAILABLE");
                return Status::new(Code::Unavailable, "storage backend unavailable");
            }
            warn!(error = %s, "lance internal error");
            Status::new(Code::Internal, "internal storage error")
        }
    }
}

/// Translate an `EmbedError` into a `tonic::Status`.
///
/// This is used by the `Search` handler for query embedding; `IngestBatch`
/// reports these per-document rather than failing the RPC.
pub fn embed_to_status(err: EmbedError) -> Status {
    match err {
        EmbedError::RateLimited => Status::new(Code::ResourceExhausted, "embedder rate-limited"),
        EmbedError::Upstream { status, body } => {
            warn!(%status, %body, "embedder upstream error");
            // 4xx from the provider means the caller's model/key/input
            // is bad — that's a server config or a prompt-size issue;
            // we don't separate here because the provider usually tells
            // the story in `body`.
            Status::new(Code::Unavailable, "embedder upstream error")
        }
        EmbedError::Transport(msg) => {
            warn!(%msg, "embedder transport error");
            Status::new(Code::Unavailable, "embedder transport error")
        }
        EmbedError::InvalidResponse(msg) => {
            warn!(%msg, "embedder returned invalid response");
            Status::new(Code::Internal, "embedder returned invalid response")
        }
        EmbedError::Config(msg) => Status::new(Code::FailedPrecondition, msg),
    }
}

/// Translate a `RerankError` into a `tonic::Status`.
pub fn rerank_to_status(err: RerankError) -> Status {
    match err {
        RerankError::Http(msg) => {
            warn!(%msg, "reranker http error");
            Status::new(Code::Unavailable, "reranker upstream error")
        }
        RerankError::UpstreamStatus { status, body } => {
            warn!(%status, %body, "reranker upstream status");
            if status == 429 {
                Status::new(Code::ResourceExhausted, "reranker rate-limited")
            } else if status >= 500 {
                Status::new(Code::Unavailable, "reranker upstream error")
            } else {
                // 4xx — bad request shape (shouldn't happen, we control
                // it), or auth problem.
                Status::new(Code::FailedPrecondition, "reranker rejected request")
            }
        }
        RerankError::Decode(msg) => {
            warn!(%msg, "reranker response decode error");
            Status::new(Code::Internal, "reranker returned invalid response")
        }
        RerankError::Timeout => Status::new(Code::Unavailable, "reranker timeout"),
        RerankError::Invalid(msg) => Status::new(Code::InvalidArgument, msg),
        RerankError::Config(msg) => Status::new(Code::FailedPrecondition, msg),
    }
}

/// A per-document error classifier used by `IngestBatch` to fill in
/// `DocumentResult.error_code`. The proto lists the canonical set:
///   "empty_content" | "chunking_failed" | "embedding_api_error"
///   | "embedding_dim_mismatch" | "nan_vector" | "storage_write_failed"
///   | "internal"
#[derive(Debug, Clone, Copy)]
pub enum DocErrorCode {
    EmptyContent,
    ChunkingFailed,
    EmbeddingApiError,
    EmbeddingDimMismatch,
    NanVector,
    StorageWriteFailed,
    Internal,
}

impl DocErrorCode {
    pub fn as_proto_str(&self) -> &'static str {
        match self {
            Self::EmptyContent => "empty_content",
            Self::ChunkingFailed => "chunking_failed",
            Self::EmbeddingApiError => "embedding_api_error",
            Self::EmbeddingDimMismatch => "embedding_dim_mismatch",
            Self::NanVector => "nan_vector",
            Self::StorageWriteFailed => "storage_write_failed",
            Self::Internal => "internal",
        }
    }
}

/// Classify an `EmbedError` for a per-doc `error_code`.
pub fn classify_embed_error(err: &EmbedError) -> DocErrorCode {
    match err {
        EmbedError::InvalidResponse(msg) if msg.contains("dim") => {
            DocErrorCode::EmbeddingDimMismatch
        }
        EmbedError::RateLimited | EmbedError::Upstream { .. } | EmbedError::Transport(_) => {
            DocErrorCode::EmbeddingApiError
        }
        EmbedError::InvalidResponse(_) => DocErrorCode::EmbeddingApiError,
        EmbedError::Config(_) => DocErrorCode::Internal,
    }
}
