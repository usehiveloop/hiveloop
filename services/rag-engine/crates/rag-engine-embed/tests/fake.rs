//! `FakeEmbedder` tests — determinism, kind-sensitivity, unit norm,
//! dimension respect.

use rag_engine_embed::{EmbedKind, Embedder, FakeEmbedder};

#[tokio::test]
async fn determinism_same_process() {
    let e = FakeEmbedder::new("fake:det", 64);
    let a = e
        .embed(vec!["hello world".to_string()], EmbedKind::Query)
        .await
        .unwrap();
    let b = e
        .embed(vec!["hello world".to_string()], EmbedKind::Query)
        .await
        .unwrap();
    assert_eq!(a, b, "same input must produce byte-identical output");
}

#[tokio::test]
async fn distinct_inputs_produce_distinct_vectors() {
    let e = FakeEmbedder::new("fake:distinct", 64);
    let out = e
        .embed(
            vec!["alpha".to_string(), "beta".to_string()],
            EmbedKind::Passage,
        )
        .await
        .unwrap();
    assert_eq!(out.len(), 2);
    assert_ne!(
        out[0], out[1],
        "different inputs must yield different vectors"
    );
}

#[tokio::test]
async fn honors_dimension() {
    for dim in [32u32, 128, 1024, 2560] {
        let e = FakeEmbedder::new("fake:dim", dim);
        let v = e
            .embed(vec!["x".to_string()], EmbedKind::Passage)
            .await
            .unwrap();
        assert_eq!(v[0].len(), dim as usize, "dim {dim} mismatch");
    }
}

#[tokio::test]
async fn query_and_passage_differ_for_same_text() {
    let e = FakeEmbedder::new("fake:kind", 64);
    let q = e
        .embed(vec!["same text".to_string()], EmbedKind::Query)
        .await
        .unwrap();
    let p = e
        .embed(vec!["same text".to_string()], EmbedKind::Passage)
        .await
        .unwrap();
    assert_ne!(
        q, p,
        "query and passage embeddings must differ for the same text"
    );
}

#[tokio::test]
async fn unit_normalized() {
    let e = FakeEmbedder::new("fake:norm", 256);
    let v = e
        .embed(vec!["normalize me".to_string()], EmbedKind::Query)
        .await
        .unwrap();
    let norm: f32 = v[0].iter().map(|x| x * x).sum::<f32>().sqrt();
    assert!(
        (norm - 1.0).abs() < 1e-3,
        "expected L2-norm ≈ 1, got {norm}"
    );
}

#[tokio::test]
async fn metadata_accessors() {
    let e = FakeEmbedder::with_max_tokens("fake:meta", 512, 4096);
    assert_eq!(e.id(), "fake:meta");
    assert_eq!(e.dimension(), 512);
    assert_eq!(e.max_input_tokens(), 4096);
}

#[tokio::test]
async fn empty_input_returns_empty() {
    let e = FakeEmbedder::new("fake:empty", 64);
    let v = e.embed(Vec::new(), EmbedKind::Query).await.unwrap();
    assert!(v.is_empty());
}
