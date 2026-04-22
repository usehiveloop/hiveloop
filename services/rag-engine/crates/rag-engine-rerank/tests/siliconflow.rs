//! SiliconFlow reranker tests.
//!
//! * `real_api_roundtrip` — hits the real SiliconFlow `/rerank` endpoint;
//!   skipped if `SILICONFLOW_API_KEY` is unset.
//! * `retries_on_429` — local `wiremock` server verifies the retry
//!   middleware honours transient 429s.
//!
//! The wiremock-backed test is fully hermetic: no outbound network, no
//! paid API calls.

use std::time::Duration;

use rag_engine_rerank::siliconflow::{SiliconFlowReranker, SiliconFlowRerankerConfig};
use rag_engine_rerank::Reranker;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

fn make_reranker(base_url: &str, api_key: &str, max_retries: u32) -> SiliconFlowReranker {
    SiliconFlowReranker::new(SiliconFlowRerankerConfig {
        base_url: base_url.to_string(),
        model: "Qwen/Qwen3-Reranker-0.6B".to_string(),
        api_key: api_key.to_string(),
        timeout: Duration::from_secs(10),
        max_retries,
        batch_size: 100,
    })
    .expect("reranker must construct")
}

#[tokio::test]
async fn real_api_roundtrip() {
    let Ok(api_key) = std::env::var("SILICONFLOW_API_KEY") else {
        eprintln!("SILICONFLOW_API_KEY not set — skipping real-API test");
        return;
    };

    let base_url = std::env::var("SILICONFLOW_BASE_URL")
        .unwrap_or_else(|_| "https://api.siliconflow.com/v1".to_string());

    let r = make_reranker(&base_url, &api_key, 2);
    let query = "What is the capital of France?";
    let candidates = vec![
        "Paris is the capital of France.".to_string(),
        "The Eiffel Tower is in Paris.".to_string(),
        "Bananas are yellow fruits.".to_string(),
    ];

    let scores = r
        .rerank(query, candidates.clone())
        .await
        .expect("real rerank must succeed");

    assert_eq!(scores.len(), candidates.len());
    for s in &scores {
        assert!((0.0..=1.0).contains(s), "score {s} out of [0,1]");
    }
    // Paris sentence MUST outscore the banana sentence for a working
    // reranker. This is the business-behaviour assertion.
    assert!(
        scores[0] > scores[2],
        "expected Paris > banana (got {:?})",
        scores
    );
}

#[tokio::test]
async fn retries_on_429_then_succeeds() {
    let server = MockServer::start().await;

    // First: 429. Second: 200 with a valid payload.
    Mock::given(method("POST"))
        .and(path("/rerank"))
        .respond_with(ResponseTemplate::new(429))
        .up_to_n_times(1)
        .mount(&server)
        .await;

    Mock::given(method("POST"))
        .and(path("/rerank"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "id": "r1",
            "results": [
                { "index": 0, "relevance_score": 0.9 },
                { "index": 1, "relevance_score": 0.1 }
            ],
            "tokens": { "input_tokens": 1, "output_tokens": 1 }
        })))
        .mount(&server)
        .await;

    let r = make_reranker(&server.uri(), "test-key", 3);
    let scores = r
        .rerank("q", vec!["relevant".to_string(), "junk".to_string()])
        .await
        .expect("retry must succeed");

    assert_eq!(scores.len(), 2);
    assert!((scores[0] - 0.9).abs() < 1e-6);
    assert!((scores[1] - 0.1).abs() < 1e-6);
}

#[tokio::test]
async fn response_is_reassembled_to_input_order() {
    let server = MockServer::start().await;

    // Upstream returns results in non-input order; we must put them
    // back on index[i].
    Mock::given(method("POST"))
        .and(path("/rerank"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "id": "r2",
            "results": [
                { "index": 2, "relevance_score": 0.2 },
                { "index": 0, "relevance_score": 0.8 },
                { "index": 1, "relevance_score": 0.5 }
            ],
            "tokens": { "input_tokens": 1, "output_tokens": 1 }
        })))
        .mount(&server)
        .await;

    let r = make_reranker(&server.uri(), "test-key", 0);
    let scores = r
        .rerank("q", vec!["a".to_string(), "b".to_string(), "c".to_string()])
        .await
        .expect("ok");

    assert!((scores[0] - 0.8).abs() < 1e-6, "scores[0] = {}", scores[0]);
    assert!((scores[1] - 0.5).abs() < 1e-6, "scores[1] = {}", scores[1]);
    assert!((scores[2] - 0.2).abs() < 1e-6, "scores[2] = {}", scores[2]);
}

#[tokio::test]
async fn splits_large_input_across_batches() {
    use wiremock::matchers::body_partial_json;
    let server = MockServer::start().await;

    // Batch 1: 100 documents.
    Mock::given(method("POST"))
        .and(path("/rerank"))
        .and(body_partial_json(serde_json::json!({
            "documents": (0..100).map(|i| format!("c{i}")).collect::<Vec<_>>()
        })))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "id": "r3a",
            "results": (0..100)
                .map(|i| serde_json::json!({ "index": i, "relevance_score": 1.0 }))
                .collect::<Vec<_>>(),
            "tokens": { "input_tokens": 1, "output_tokens": 1 }
        })))
        .mount(&server)
        .await;

    // Batch 2: 50 documents — indices 0..50 of the second batch.
    Mock::given(method("POST"))
        .and(path("/rerank"))
        .and(body_partial_json(serde_json::json!({
            "documents": (100..150).map(|i| format!("c{i}")).collect::<Vec<_>>()
        })))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "id": "r3b",
            "results": (0..50)
                .map(|i| serde_json::json!({ "index": i, "relevance_score": 1.0 }))
                .collect::<Vec<_>>(),
            "tokens": { "input_tokens": 1, "output_tokens": 1 }
        })))
        .mount(&server)
        .await;

    let r = make_reranker(&server.uri(), "test-key", 0);
    let candidates: Vec<String> = (0..150).map(|i| format!("c{i}")).collect();

    let scores = r.rerank("q", candidates.clone()).await.expect("ok");
    assert_eq!(scores.len(), candidates.len());
    for (i, s) in scores.iter().enumerate() {
        assert!((*s - 1.0).abs() < 1e-6, "index {i} expected 1.0, got {s}");
    }
}

#[tokio::test]
async fn non_retryable_4xx_surfaces_as_upstream_status() {
    let server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/rerank"))
        .respond_with(ResponseTemplate::new(401).set_body_string("unauthorized"))
        .mount(&server)
        .await;

    let r = make_reranker(&server.uri(), "bad-key", 0);
    let err = r
        .rerank("q", vec!["x".to_string()])
        .await
        .expect_err("must fail");

    match err {
        rag_engine_rerank::RerankError::UpstreamStatus { status, .. } => {
            assert_eq!(status, 401);
        }
        other => panic!("expected UpstreamStatus, got {:?}", other),
    }
}

#[tokio::test]
async fn empty_candidates_short_circuits_without_http_call() {
    // No mock registered — if the reranker made an HTTP call, we'd hit
    // a 404 and fail. The empty-candidates path must not touch HTTP.
    let server = MockServer::start().await;

    let r = make_reranker(&server.uri(), "test-key", 0);
    let scores = r.rerank("q", Vec::new()).await.expect("ok");
    assert!(scores.is_empty());
}
