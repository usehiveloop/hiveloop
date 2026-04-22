//! Batching helper tests — order preservation under parallel sub-batches,
//! single-text path, empty input, batch-size edge cases.
//!
//! Uses `FakeEmbedder` (no network) per the testing rules.

use std::sync::Arc;

use rag_engine_embed::{embed_batched, EmbedKind, Embedder, FakeEmbedder};

#[tokio::test]
async fn preserves_order_across_sub_batches() {
    let emb: Arc<dyn Embedder> = Arc::new(FakeEmbedder::new("fake:batch", 64));
    let texts: Vec<String> = (0..100).map(|i| format!("text-{i:03}")).collect();

    // Batch size 7, concurrency 4 — forces heavy interleaving.
    let batched = embed_batched(Arc::clone(&emb), texts.clone(), EmbedKind::Passage, 7, 4)
        .await
        .unwrap();

    // Compare against a single-shot (deterministic) call.
    let single = emb.embed(texts, EmbedKind::Passage).await.unwrap();

    assert_eq!(batched.len(), single.len());
    for (i, (a, b)) in batched.iter().zip(single.iter()).enumerate() {
        assert_eq!(a, b, "position {i} out of order");
    }
}

#[tokio::test]
async fn single_text_single_batch() {
    let emb: Arc<dyn Embedder> = Arc::new(FakeEmbedder::new("fake:one", 32));
    let out = embed_batched(emb, vec!["solo".to_string()], EmbedKind::Query, 32, 4)
        .await
        .unwrap();
    assert_eq!(out.len(), 1);
    assert_eq!(out[0].len(), 32);
}

#[tokio::test]
async fn empty_input_returns_empty() {
    let emb: Arc<dyn Embedder> = Arc::new(FakeEmbedder::new("fake:empty", 32));
    let out = embed_batched(emb, Vec::new(), EmbedKind::Query, 32, 4)
        .await
        .unwrap();
    assert!(out.is_empty());
}

#[tokio::test]
async fn batch_size_larger_than_inputs() {
    let emb: Arc<dyn Embedder> = Arc::new(FakeEmbedder::new("fake:small", 16));
    let texts = vec!["a".to_string(), "b".to_string(), "c".to_string()];
    let out = embed_batched(Arc::clone(&emb), texts.clone(), EmbedKind::Passage, 1000, 4)
        .await
        .unwrap();
    let direct = emb.embed(texts, EmbedKind::Passage).await.unwrap();
    assert_eq!(out, direct);
}

#[tokio::test]
async fn zero_batch_and_concurrency_are_clamped() {
    // The helper clamps 0 to 1 defensively. We exercise the clamp here to
    // pin the guard; a config validator upstream should reject these.
    let emb: Arc<dyn Embedder> = Arc::new(FakeEmbedder::new("fake:clamp", 8));
    let texts = vec!["x".into(), "y".into()];
    let out = embed_batched(emb, texts, EmbedKind::Passage, 0, 0)
        .await
        .unwrap();
    assert_eq!(out.len(), 2);
    assert_eq!(out[0].len(), 8);
}
