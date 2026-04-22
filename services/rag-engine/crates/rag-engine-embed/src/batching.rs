//! Batching helper: splits a large input list into sub-batches, drives
//! them in parallel with bounded concurrency, and re-assembles results
//! in input order.
//!
//! Why here and not per-impl: callers (the gRPC handler, chunker pipeline)
//! take `Arc<dyn Embedder>` and shouldn't need to know the per-call size
//! cap of SiliconFlow vs. a real OpenAI endpoint. `FakeEmbedder` also
//! benefits from this shape in tests that feed thousands of chunks at
//! once.

use std::sync::Arc;

use crate::errors::EmbedError;
use crate::types::EmbedKind;
use crate::Embedder;

/// Default per-call batch size. Matches the SiliconFlow + OpenAI practical
/// ceiling for embeddings under most org tiers.
pub const DEFAULT_BATCH_SIZE: usize = 32;

/// Batch `texts` through `embedder` in chunks of `batch_size`, running up
/// to `concurrency` in-flight. Output order matches input order.
///
/// Empty input returns an empty `Vec<Vec<f32>>`.
///
/// `batch_size` of 0 is treated as 1 (no-op guard; a config-level
/// validator should reject this earlier).
pub async fn embed_batched(
    embedder: Arc<dyn Embedder>,
    texts: Vec<String>,
    kind: EmbedKind,
    batch_size: usize,
    concurrency: usize,
) -> Result<Vec<Vec<f32>>, EmbedError> {
    if texts.is_empty() {
        return Ok(Vec::new());
    }

    let batch_size = batch_size.max(1);
    let concurrency = concurrency.max(1);

    // Precompute each sub-batch's (start_index, texts) pair so we can
    // reassemble in order after parallel completion.
    let mut sub_batches: Vec<(usize, Vec<String>)> = Vec::new();
    let mut start = 0usize;
    let mut iter = texts.into_iter();
    loop {
        let chunk: Vec<String> = iter.by_ref().take(batch_size).collect();
        if chunk.is_empty() {
            break;
        }
        let len = chunk.len();
        sub_batches.push((start, chunk));
        start += len;
    }

    let total_output_len = start;
    let mut output: Vec<Option<Vec<f32>>> = (0..total_output_len).map(|_| None).collect();

    // Use a Semaphore to bound concurrency, drive sub-batches via
    // JoinSet. A JoinSet lets us await the first-finished task and
    // short-circuit on error.
    type BatchResult = Result<(usize, Vec<Vec<f32>>), EmbedError>;
    let sem = Arc::new(tokio::sync::Semaphore::new(concurrency));
    let mut set: tokio::task::JoinSet<BatchResult> = tokio::task::JoinSet::new();

    for (offset, batch) in sub_batches {
        let permit = Arc::clone(&sem);
        let emb = Arc::clone(&embedder);
        set.spawn(async move {
            // Hold the permit until this sub-batch call returns. Dropping
            // it at the end of the task releases capacity for the next
            // queued sub-batch.
            let _p = permit
                .acquire_owned()
                .await
                .map_err(|e| EmbedError::Transport(format!("semaphore closed: {e}")))?;
            let out = emb.embed(batch, kind).await?;
            Ok((offset, out))
        });
    }

    while let Some(joined) = set.join_next().await {
        let (offset, vectors) =
            joined.map_err(|e| EmbedError::Transport(format!("join error: {e}")))??;
        for (i, v) in vectors.into_iter().enumerate() {
            output[offset + i] = Some(v);
        }
    }

    // All slots filled or we propagated an error earlier. If any slot is
    // None at this point, the embedder returned a short batch — that's
    // an InvalidResponse.
    output
        .into_iter()
        .enumerate()
        .map(|(i, slot)| {
            slot.ok_or_else(|| {
                EmbedError::InvalidResponse(format!(
                    "missing vector at position {i} after batching"
                ))
            })
        })
        .collect()
}
