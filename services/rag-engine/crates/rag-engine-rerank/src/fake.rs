//! Deterministic `FakeReranker`.
//!
//! Scores each candidate by `sha256(query || "\n" || candidate)` mapped to
//! `[0.0, 1.0]` via the first 4 bytes of the digest. Same inputs always
//! produce identical scores; different candidates almost always produce
//! different scores (2^-32 collision probability), which gives non-
//! pathological variance for tests that sort/top-K.
//!
//! This is the only mock permitted for the reranker surface (see
//! `internal/rag/doc/TESTING.md`).

use async_trait::async_trait;
use sha2::{Digest, Sha256};

use crate::{RerankError, Reranker};

#[derive(Debug, Clone)]
pub struct FakeReranker {
    id: String,
}

impl FakeReranker {
    pub fn new() -> Self {
        Self {
            id: "fake-reranker".to_string(),
        }
    }

    pub fn with_id(id: impl Into<String>) -> Self {
        Self { id: id.into() }
    }

    fn score(query: &str, candidate: &str) -> f32 {
        let mut h = Sha256::new();
        h.update(query.as_bytes());
        h.update(b"\n");
        h.update(candidate.as_bytes());
        let digest = h.finalize();
        let bytes = [digest[0], digest[1], digest[2], digest[3]];
        let v = u32::from_be_bytes(bytes);
        // Map to [0.0, 1.0). Using f64 intermediate to avoid precision
        // artifacts near the top of the range.
        let f = (v as f64) / (u32::MAX as f64 + 1.0);
        f as f32
    }
}

impl Default for FakeReranker {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Reranker for FakeReranker {
    fn id(&self) -> &str {
        &self.id
    }

    async fn rerank(&self, query: &str, candidates: Vec<String>) -> Result<Vec<f32>, RerankError> {
        Ok(candidates.iter().map(|c| Self::score(query, c)).collect())
    }
}
