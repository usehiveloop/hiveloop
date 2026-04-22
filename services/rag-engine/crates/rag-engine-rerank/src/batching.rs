//! Batch splitting for rerank calls.
//!
//! SiliconFlow's rerank endpoint has practical limits on payload size and
//! we cap per-call candidates at `MAX_CANDIDATES_PER_CALL`. This module
//! splits a long candidate list into windows, preserves the original
//! input index, and re-assembles scores back in input order.

/// Maximum candidates sent in a single upstream call.
pub const MAX_CANDIDATES_PER_CALL: usize = 100;

/// A window of the original candidate list together with the absolute
/// offset of the first element — used to map results back into the
/// caller's ordering.
#[derive(Debug, Clone)]
pub struct Batch<'a> {
    pub offset: usize,
    pub items: &'a [String],
}

/// Split `candidates` into windows of at most `batch_size` items.
///
/// `batch_size` of 0 is treated as `MAX_CANDIDATES_PER_CALL` to avoid
/// pathological misconfiguration.
pub fn split<'a>(candidates: &'a [String], batch_size: usize) -> Vec<Batch<'a>> {
    let size = if batch_size == 0 {
        MAX_CANDIDATES_PER_CALL
    } else {
        batch_size
    };
    let mut out = Vec::with_capacity(candidates.len().div_ceil(size));
    let mut offset = 0usize;
    for chunk in candidates.chunks(size) {
        out.push(Batch {
            offset,
            items: chunk,
        });
        offset += chunk.len();
    }
    out
}

/// Assemble per-batch score vectors back into a single vector that is
/// aligned with the caller's original input order.
///
/// Each `(offset, scores)` pair must have `scores.len() <=
/// total - offset`. Any gap left unfilled (shouldn't happen for
/// correctly-behaving implementations) is filled with `0.0`.
pub fn merge(total: usize, parts: Vec<(usize, Vec<f32>)>) -> Vec<f32> {
    let mut out = vec![0.0f32; total];
    for (offset, scores) in parts {
        for (i, s) in scores.into_iter().enumerate() {
            let idx = offset + i;
            if idx < total {
                out[idx] = s;
            }
        }
    }
    out
}
