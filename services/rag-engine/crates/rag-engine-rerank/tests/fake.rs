//! Tests for the deterministic `FakeReranker`.

use rag_engine_rerank::{FakeReranker, Reranker};

#[tokio::test]
async fn fake_reranker_is_deterministic_across_calls() {
    let r = FakeReranker::new();
    let q = "what is the capital of France?";
    let candidates = vec![
        "Paris is the capital of France.".to_string(),
        "Berlin is the capital of Germany.".to_string(),
        "Madrid is the capital of Spain.".to_string(),
    ];

    let first = r.rerank(q, candidates.clone()).await.unwrap();
    let second = r.rerank(q, candidates.clone()).await.unwrap();

    assert_eq!(
        first, second,
        "same query+candidates must yield same scores"
    );
    assert_eq!(first.len(), candidates.len());
    for s in &first {
        assert!((0.0..=1.0).contains(s), "score {s} out of [0,1]");
    }
}

#[tokio::test]
async fn fake_reranker_distinguishes_candidates() {
    let r = FakeReranker::new();
    let q = "query";
    let candidates: Vec<String> = (0..32).map(|i| format!("candidate-{i}")).collect();

    let scores = r.rerank(q, candidates.clone()).await.unwrap();
    assert_eq!(scores.len(), candidates.len());

    // Non-pathological variance: at least half the pairs differ.
    let mut distinct = 0usize;
    for i in 0..scores.len() {
        for j in (i + 1)..scores.len() {
            if (scores[i] - scores[j]).abs() > f32::EPSILON {
                distinct += 1;
            }
        }
    }
    let total_pairs = scores.len() * (scores.len() - 1) / 2;
    assert!(
        distinct * 2 > total_pairs,
        "expected variance: {distinct}/{total_pairs} pairs distinct"
    );
}

#[tokio::test]
async fn fake_reranker_preserves_input_order() {
    let r = FakeReranker::new();
    let q = "q";
    let candidates = vec![
        "alpha".to_string(),
        "beta".to_string(),
        "gamma".to_string(),
        "delta".to_string(),
    ];

    let s1 = r.rerank(q, candidates.clone()).await.unwrap();

    // Shuffle via reversal; scores must permute accordingly — index-N
    // score in the original must equal index-(len-1-N) score in the
    // reversed call. This proves the output is aligned positionally
    // rather than globally sorted.
    let reversed: Vec<String> = candidates.iter().rev().cloned().collect();
    let s2 = r.rerank(q, reversed).await.unwrap();

    for (i, v) in s1.iter().enumerate() {
        assert_eq!(*v, s2[s1.len() - 1 - i]);
    }
}

#[tokio::test]
async fn fake_reranker_empty_candidates_returns_empty() {
    let r = FakeReranker::new();
    let scores = r.rerank("anything", Vec::new()).await.unwrap();
    assert!(scores.is_empty());
}

#[tokio::test]
async fn fake_reranker_exposes_stable_id() {
    let r = FakeReranker::new();
    assert_eq!(r.id(), "fake-reranker");

    let r2 = FakeReranker::with_id("custom");
    assert_eq!(r2.id(), "custom");
}
