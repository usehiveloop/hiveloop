//! Deterministic in-memory embedder used by tests and offline flows.
//!
//! The ONLY mock-like construct permitted in the codebase (per
//! `TESTING.md`). Every fake embedding is a pure function of
//! `(text, kind, dimension)` — same input, same bytes, across processes.
//!
//! Implementation:
//!   1. Domain-separate `kind` by prepending a single byte to the input
//!      before hashing. `Query` vs `Passage` therefore yield different
//!      vectors for the same text — mirrors the production behaviour
//!      where prefixes change the embedding.
//!   2. SHA-256 the (kind_byte || text) to produce 32 deterministic bytes.
//!   3. Seed `ChaCha20Rng` from the first 8 bytes of the digest (as a
//!      `u64`), sample `dimension` floats uniform in [-1, 1).
//!   4. Unit-normalize the resulting vector so cosine-similarity tests
//!      behave identically to the real embedder's output (the real
//!      embedder returns normalized vectors).
//!
//! No network, no locks, no async work — the `async` in the trait is
//! purely for compatibility with the real impl.

use async_trait::async_trait;
// DEVIATION (wave2 consolidation): rand 0.9 API — `gen_range` was renamed
// to `random_range` and moved onto the `Rng` trait.
use rand::Rng;
use rand::SeedableRng;
use rand_chacha::ChaCha20Rng;
use sha2::{Digest, Sha256};

use crate::errors::EmbedError;
use crate::types::EmbedKind;
use crate::Embedder;

/// Deterministic, in-memory `Embedder`. See module docs.
#[derive(Debug, Clone)]
pub struct FakeEmbedder {
    id: String,
    dimension: u32,
    max_input_tokens: u32,
}

impl FakeEmbedder {
    /// Construct at the given dimension. `id` is purely informational.
    /// `max_input_tokens` defaults to `8192` (the most common Qwen value)
    /// via [`FakeEmbedder::new`]; use [`FakeEmbedder::with_max_tokens`]
    /// to override.
    pub fn new(id: impl Into<String>, dimension: u32) -> Self {
        Self {
            id: id.into(),
            dimension,
            max_input_tokens: 8192,
        }
    }

    /// Construct with an explicit `max_input_tokens`. Useful for tests
    /// that exercise the token-budget branch.
    pub fn with_max_tokens(id: impl Into<String>, dimension: u32, max_tokens: u32) -> Self {
        Self {
            id: id.into(),
            dimension,
            max_input_tokens: max_tokens,
        }
    }

    /// Pure function exposing the deterministic vector generator. Kept
    /// pub(crate) so tests in-crate can pin the algorithm.
    pub(crate) fn deterministic_vector(text: &str, kind: EmbedKind, dimension: u32) -> Vec<f32> {
        let kind_byte: u8 = match kind {
            EmbedKind::Passage => 0x00,
            EmbedKind::Query => 0x01,
        };

        let mut hasher = Sha256::new();
        hasher.update([kind_byte]);
        hasher.update(text.as_bytes());
        let digest = hasher.finalize();

        let mut seed_bytes = [0u8; 32];
        seed_bytes.copy_from_slice(&digest);
        let mut rng = ChaCha20Rng::from_seed(seed_bytes);

        let mut v = Vec::with_capacity(dimension as usize);
        for _ in 0..dimension {
            // Uniform in [-1, 1). (rand 0.9: `random_range` replaces `gen_range`.)
            let f: f32 = rng.random_range(-1.0f32..1.0f32);
            v.push(f);
        }

        // Unit-normalize (L2). If the (astronomically unlikely) all-zero
        // vector shows up, leave it alone — the test suite never exercises
        // that branch and the real embedder can't produce it either.
        let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
        if norm > 0.0 {
            for x in v.iter_mut() {
                *x /= norm;
            }
        }
        v
    }
}

#[async_trait]
impl Embedder for FakeEmbedder {
    fn id(&self) -> &str {
        &self.id
    }

    fn dimension(&self) -> u32 {
        self.dimension
    }

    fn max_input_tokens(&self) -> u32 {
        self.max_input_tokens
    }

    async fn embed(
        &self,
        texts: Vec<String>,
        kind: EmbedKind,
    ) -> Result<Vec<Vec<f32>>, EmbedError> {
        Ok(texts
            .into_iter()
            .map(|t| Self::deterministic_vector(&t, kind, self.dimension))
            .collect())
    }
}
