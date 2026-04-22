//! SiliconFlow embedder tests.
//!
//! Two flavors:
//!   * `wiremock`-backed retry-on-429 test — validates retry middleware
//!     actually kicks in on the real transport stack. This is NOT mocking
//!     the embedder; it's mocking the upstream HTTP endpoint, which is
//!     the only way to exercise retry branches deterministically.
//!   * Real API roundtrip — runs only if `SILICONFLOW_API_KEY` is present
//!     in the env. Skipped otherwise (the sole sanctioned skip, per
//!     TESTING.md — paid external API).
//!
//! The `wiremock` usage is scoped to this test file and is an
//! intentional, narrow extension of the "mocks allowed for embedder"
//! rule: the wiremock instance IS a real HTTP server, controlled by us.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::Duration;

use rag_engine_embed::{
    build, EmbedKind, Embedder, EmbedderConfig, Provider, SiliconFlowConfig, SiliconFlowEmbedder,
};
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, Request, ResponseTemplate};

fn opts_for_mock(base_url: &str, dimension: u32) -> rag_engine_embed::SiliconFlowEmbedder {
    // We reach into the public `SiliconFlowEmbedder::new` via the
    // re-exported `new_options` helper. Since the config-level `build()`
    // reads the api_key from env, these low-level tests construct options
    // directly for clarity.
    use rag_engine_embed::SiliconFlowEmbedder;
    // SAFETY: we re-export `SiliconFlowOptions` from the crate root.
    let opts = rag_engine_embed::SiliconFlowOptions {
        id: "siliconflow:qwen3-embedding-4b".into(),
        model_name: "Qwen/Qwen3-Embedding-4B".into(),
        dimension,
        max_input_tokens: 8192,
        base_url: base_url.to_string(),
        api_key: "test-secret".into(),
        passage_prefix: Some("passage: ".into()),
        query_prefix: Some("query: ".into()),
        timeout: Duration::from_secs(5),
        concurrency: 4,
        batch_size: 32,
        max_retries: 3,
    };
    SiliconFlowEmbedder::new(opts).unwrap()
}

#[tokio::test]
async fn serializes_request_and_applies_passage_prefix() {
    let server = MockServer::start().await;
    let dim = 4u32;

    // Inspector mock: captures the request body.
    let captured: Arc<tokio::sync::Mutex<Option<serde_json::Value>>> =
        Arc::new(tokio::sync::Mutex::new(None));
    let captured_clone = Arc::clone(&captured);

    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .and(header("authorization", "Bearer test-secret"))
        .respond_with(move |req: &Request| {
            let body: serde_json::Value = serde_json::from_slice(&req.body).unwrap();
            // Stash the body for assertion after the call returns.
            let c = Arc::clone(&captured_clone);
            tokio::spawn(async move {
                *c.lock().await = Some(body);
            });
            ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "data": [
                    { "index": 0, "embedding": vec![0.1f32, 0.2, 0.3, 0.4] }
                ]
            }))
        })
        .mount(&server)
        .await;

    let emb = opts_for_mock(&server.uri(), dim);
    let out = emb
        .embed(vec!["hello".into()], EmbedKind::Passage)
        .await
        .unwrap();
    assert_eq!(out.len(), 1);
    assert_eq!(out[0], vec![0.1, 0.2, 0.3, 0.4]);

    // Yield so the spawned body-capture task finishes.
    tokio::time::sleep(Duration::from_millis(50)).await;
    let body = captured.lock().await.clone().expect("body captured");
    assert_eq!(body["model"], "Qwen/Qwen3-Embedding-4B");
    assert_eq!(body["encoding_format"], "float");
    assert_eq!(body["input"], serde_json::json!(["passage: hello"]));
}

#[tokio::test]
async fn applies_query_prefix() {
    let server = MockServer::start().await;

    let captured: Arc<tokio::sync::Mutex<Option<serde_json::Value>>> =
        Arc::new(tokio::sync::Mutex::new(None));
    let cap = Arc::clone(&captured);

    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(move |req: &Request| {
            let body: serde_json::Value = serde_json::from_slice(&req.body).unwrap();
            let c = Arc::clone(&cap);
            tokio::spawn(async move {
                *c.lock().await = Some(body);
            });
            ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "data": [
                    { "index": 0, "embedding": vec![0.0f32; 4] }
                ]
            }))
        })
        .mount(&server)
        .await;

    let emb = opts_for_mock(&server.uri(), 4);
    emb.embed(vec!["find this".into()], EmbedKind::Query)
        .await
        .unwrap();

    tokio::time::sleep(Duration::from_millis(50)).await;
    let body = captured.lock().await.clone().unwrap();
    assert_eq!(body["input"], serde_json::json!(["query: find this"]));
}

#[tokio::test]
async fn retries_on_429_then_succeeds() {
    let server = MockServer::start().await;
    let counter = Arc::new(AtomicUsize::new(0));
    let counter_clone = Arc::clone(&counter);

    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(move |_req: &Request| {
            let n = counter_clone.fetch_add(1, Ordering::SeqCst);
            if n < 2 {
                ResponseTemplate::new(429).set_body_string("slow down")
            } else {
                ResponseTemplate::new(200).set_body_json(serde_json::json!({
                    "data": [ { "index": 0, "embedding": vec![1.0f32, 0.0, 0.0, 0.0] } ]
                }))
            }
        })
        .mount(&server)
        .await;

    let emb = opts_for_mock(&server.uri(), 4);
    let out = emb
        .embed(vec!["retry me".into()], EmbedKind::Passage)
        .await
        .expect("should succeed after two 429s");
    assert_eq!(out[0], vec![1.0, 0.0, 0.0, 0.0]);
    assert!(
        counter.load(Ordering::SeqCst) >= 3,
        "expected ≥3 calls (2 failures + 1 success); saw {}",
        counter.load(Ordering::SeqCst)
    );
}

#[tokio::test]
async fn rate_limited_after_max_retries() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(ResponseTemplate::new(429).set_body_string("nope"))
        .mount(&server)
        .await;

    let emb = opts_for_mock(&server.uri(), 4);
    let err = emb
        .embed(vec!["hi".into()], EmbedKind::Passage)
        .await
        .expect_err("should return RateLimited after retries");
    assert!(
        matches!(err, rag_engine_embed::EmbedError::RateLimited),
        "expected RateLimited, got {err:?}"
    );
}

#[tokio::test]
async fn invalid_response_on_dim_mismatch() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            // Configured dim is 4, response has 5 → must be flagged.
            "data": [ { "index": 0, "embedding": vec![0.0, 0.1, 0.2, 0.3, 0.4] } ]
        })))
        .mount(&server)
        .await;

    let emb = opts_for_mock(&server.uri(), 4);
    let err = emb
        .embed(vec!["mismatch".into()], EmbedKind::Passage)
        .await
        .expect_err("dim mismatch must fail");
    assert!(matches!(
        err,
        rag_engine_embed::EmbedError::InvalidResponse(_)
    ));
}

#[tokio::test]
async fn non_retryable_4xx_surfaces_upstream() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(ResponseTemplate::new(400).set_body_string("bad input"))
        .mount(&server)
        .await;

    let emb = opts_for_mock(&server.uri(), 4);
    let err = emb
        .embed(vec!["x".into()], EmbedKind::Passage)
        .await
        .expect_err("400 must surface");
    match err {
        rag_engine_embed::EmbedError::Upstream { status, .. } => assert_eq!(status, 400),
        other => panic!("unexpected error: {other:?}"),
    }
}

#[tokio::test]
async fn batches_large_inputs_preserving_order() {
    let server = MockServer::start().await;
    // Echo each input's index into the first float slot so we can verify
    // ordering under parallel sub-batches.
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(|req: &Request| {
            let body: serde_json::Value = serde_json::from_slice(&req.body).unwrap();
            let inputs = body["input"].as_array().unwrap();
            let data: Vec<serde_json::Value> = inputs
                .iter()
                .enumerate()
                .map(|(i, s)| {
                    // Embed: [last 3 chars parsed as u32, 0, 0, 0]
                    let id_str = s.as_str().unwrap().rsplit('-').next().unwrap();
                    let id: f32 = id_str.parse().unwrap_or(-1.0);
                    serde_json::json!({
                        "index": i,
                        "embedding": vec![id, 0.0, 0.0, 0.0]
                    })
                })
                .collect();
            ResponseTemplate::new(200).set_body_json(serde_json::json!({ "data": data }))
        })
        .mount(&server)
        .await;

    // Force small batch_size so we actually split.
    let mut opts = rag_engine_embed::SiliconFlowOptions {
        id: "sf:test".into(),
        model_name: "Qwen/Qwen3-Embedding-4B".into(),
        dimension: 4,
        max_input_tokens: 8192,
        base_url: server.uri(),
        api_key: "test-secret".into(),
        passage_prefix: None,
        query_prefix: None,
        timeout: Duration::from_secs(5),
        concurrency: 4,
        batch_size: 7,
        max_retries: 2,
    };
    opts.batch_size = 7;
    let emb = SiliconFlowEmbedder::new(opts).unwrap();

    let texts: Vec<String> = (0..30).map(|i| format!("item-{i:03}")).collect();
    let out = emb.embed(texts, EmbedKind::Passage).await.unwrap();
    assert_eq!(out.len(), 30);
    for (i, v) in out.iter().enumerate() {
        assert_eq!(v[0] as u32, i as u32, "position {i} out of order");
    }
}

#[tokio::test]
async fn config_build_missing_env_errors() {
    // Ensure the var is NOT set for this test.
    std::env::remove_var("SILICONFLOW_TEST_MISSING_KEY");
    let cfg = EmbedderConfig {
        provider: Provider::SiliconFlow(SiliconFlowConfig {
            id: "sf:x".into(),
            model_name: "Qwen/Qwen3-Embedding-4B".into(),
            dimension: 2560,
            max_input_tokens: 8192,
            base_url: "https://api.siliconflow.cn/v1".into(),
            api_key_env: "SILICONFLOW_TEST_MISSING_KEY".into(),
            passage_prefix: None,
            query_prefix: None,
            timeout_secs: 10,
            concurrency: 4,
            batch_size: 32,
            max_retries: 4,
        }),
    };
    let res = build(&cfg);
    let err = match res {
        Ok(_) => panic!("missing env must error"),
        Err(e) => e,
    };
    assert!(matches!(err, rag_engine_embed::EmbedError::Config(_)));
}

#[tokio::test]
async fn config_build_fake_succeeds() {
    let cfg = EmbedderConfig {
        provider: Provider::Fake(rag_engine_embed::FakeConfig {
            id: "fake:built".into(),
            dimension: 128,
            max_input_tokens: 8192,
        }),
    };
    let emb = build(&cfg).expect("fake must build with no env");
    assert_eq!(emb.id(), "fake:built");
    assert_eq!(emb.dimension(), 128);
}

// ---------- real API roundtrip ----------

#[tokio::test]
async fn real_siliconflow_roundtrip() {
    let Ok(api_key) = std::env::var("SILICONFLOW_API_KEY") else {
        eprintln!("SKIP: SILICONFLOW_API_KEY not set (paid external API — only sanctioned skip)");
        return;
    };
    if api_key.trim().is_empty() {
        eprintln!("SKIP: SILICONFLOW_API_KEY empty");
        return;
    }

    let base_url = std::env::var("SILICONFLOW_BASE_URL")
        .unwrap_or_else(|_| "https://api.siliconflow.cn/v1".into());

    let opts = rag_engine_embed::SiliconFlowOptions {
        id: "siliconflow:qwen3-embedding-0.6b".into(),
        model_name: "Qwen/Qwen3-Embedding-0.6B".into(),
        dimension: 1024,
        max_input_tokens: 8192,
        base_url,
        api_key,
        passage_prefix: Some("passage: ".into()),
        query_prefix: Some("query: ".into()),
        timeout: Duration::from_secs(30),
        concurrency: 2,
        batch_size: 32,
        max_retries: 4,
    };
    let emb = SiliconFlowEmbedder::new(opts).unwrap();
    let out = emb
        .embed(
            vec!["hiveloop rag engine smoke test".into()],
            EmbedKind::Query,
        )
        .await
        .expect("real API call should succeed");
    assert_eq!(out.len(), 1);
    assert_eq!(out[0].len(), 1024);
    let norm: f32 = out[0].iter().map(|x| x * x).sum::<f32>().sqrt();
    assert!(norm > 0.1, "expected non-zero vector");
}
