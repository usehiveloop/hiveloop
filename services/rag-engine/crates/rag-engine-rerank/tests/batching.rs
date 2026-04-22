//! Tests for `batching::split` / `batching::merge`.
//!
//! We verify that: (1) a 250-candidate input with batch size 100 splits
//! into 3 windows of [100, 100, 50]; (2) per-batch scores re-assemble
//! into the original input order.

use rag_engine_rerank::batching::{self, MAX_CANDIDATES_PER_CALL};

#[test]
fn split_250_at_100_produces_100_100_50() {
    let candidates: Vec<String> = (0..250).map(|i| format!("c{i}")).collect();
    let batches = batching::split(&candidates, 100);
    assert_eq!(batches.len(), 3);
    assert_eq!(batches[0].offset, 0);
    assert_eq!(batches[0].items.len(), 100);
    assert_eq!(batches[1].offset, 100);
    assert_eq!(batches[1].items.len(), 100);
    assert_eq!(batches[2].offset, 200);
    assert_eq!(batches[2].items.len(), 50);
}

#[test]
fn split_zero_batch_size_falls_back_to_max() {
    let candidates: Vec<String> = (0..150).map(|i| format!("c{i}")).collect();
    let batches = batching::split(&candidates, 0);
    // 150 / 100 = 2 windows
    assert_eq!(batches.len(), 2);
    assert_eq!(batches[0].items.len(), MAX_CANDIDATES_PER_CALL);
    assert_eq!(batches[1].items.len(), 50);
}

#[test]
fn split_smaller_than_batch_produces_single_window() {
    let candidates: Vec<String> = (0..42).map(|i| format!("c{i}")).collect();
    let batches = batching::split(&candidates, 100);
    assert_eq!(batches.len(), 1);
    assert_eq!(batches[0].offset, 0);
    assert_eq!(batches[0].items.len(), 42);
}

#[test]
fn split_empty_produces_no_batches() {
    let candidates: Vec<String> = Vec::new();
    let batches = batching::split(&candidates, 100);
    assert!(batches.is_empty());
}

#[test]
fn merge_reassembles_scores_in_input_order() {
    // Imagine 250 candidates, three batches each producing i-as-score.
    // After merge, scores[k] must equal k as f32.
    let candidates: Vec<String> = (0..250).map(|i| format!("c{i}")).collect();
    let batches = batching::split(&candidates, 100);

    let parts: Vec<(usize, Vec<f32>)> = batches
        .into_iter()
        .map(|b| {
            let scores: Vec<f32> = (0..b.items.len()).map(|i| (b.offset + i) as f32).collect();
            (b.offset, scores)
        })
        .collect();

    let merged = batching::merge(candidates.len(), parts);
    assert_eq!(merged.len(), candidates.len());
    for (i, v) in merged.iter().enumerate() {
        assert_eq!(*v, i as f32, "position {i} should equal {i}.0");
    }
}

#[test]
fn merge_fills_gaps_with_zero() {
    // Only fill in offsets 0 and 150 of a 200-length vector; 50..150
    // stays zero, 175..200 stays zero.
    let parts = vec![(0usize, vec![1.0f32; 50]), (150usize, vec![2.0f32; 25])];
    let merged = batching::merge(200, parts);
    assert_eq!(merged.len(), 200);
    assert!(merged[..50].iter().all(|&v| v == 1.0));
    assert!(merged[50..150].iter().all(|&v| v == 0.0));
    assert!(merged[150..175].iter().all(|&v| v == 2.0));
    assert!(merged[175..].iter().all(|&v| v == 0.0));
}
