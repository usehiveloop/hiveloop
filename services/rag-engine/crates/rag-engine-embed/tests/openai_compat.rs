//! OpenAI-compatible embedder tests.
//!
//! The impl delegates transport + retry to `async-openai`. These tests
//! stand up `wiremock` as a stand-in for any OpenAI-compatible provider
//! (SiliconFlow, OpenRouter, Groq, OpenAI, Together, ...) and assert:
//!
//!   * wire format: we send the shape `async-openai` emits and the
//!     prefix is prepended at the wire layer;
//!   * retry-on-429: async-openai's built-in `backoff` retries;
//!   * terminal `RateLimited` after retry budget;
//!   * dim validation catches server-side corruption;
//!   * non-retryable 4xx surfaces as `Upstream`;
//!   * batch order preserved across parallel sub-batches;
//!   * **provider-agnostic smoke**: swap the base URL to simulate
//!     OpenRouter and assert nothing in this crate cares about which
//!     provider we point at.
//!   * config loader: required env validation + `LLM_PROVIDER=fake`
//!     path.
//!   * real-provider roundtrip (sanctioned skip if no `LLM_API_KEY`).
//!
//! The `wiremock` usage is scoped to this test file: it IS a real HTTP
//! server, controlled by us — see TESTING.md for the narrow exception
//! that applies to paid external APIs.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::Duration;

use rag_engine_embed::{
    build, load_from_env_and_file, EmbedKind, Embedder, EmbedderConfig, FakeConfig,
    OpenAICompatConfig, OpenAICompatEmbedder, OpenAICompatOptions, Provider,
};
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, Request, ResponseTemplate};

// Helper: build an embedder pointing at the given mock server.
fn embedder_for_mock(base_url: &str, dimension: u32) -> OpenAICompatEmbedder {
    let opts = OpenAICompatOptions {
        id: "openai-compat:test".into(),
        model_name: "Qwen/Qwen3-Embedding-4B".into(),
        dimension,
        max_input_tokens: 8192,
        api_url: base_url.to_string(),
        api_key: "test-secret".into(),
        passage_prefix: Some("passage: ".into()),
        query_prefix: Some("query: ".into()),
        timeout: Duration::from_secs(5),
        concurrency: 4,
        batch_size: 32,
        max_retries: 3,
    };
    OpenAICompatEmbedder::new(opts).unwrap()
}

// Helper: wrap a free-form message in the OpenAI error-body shape so
// `async-openai` can parse the 4xx/429 response into an `ApiError`.
fn error_body(message: &str, type_: Option<&str>) -> serde_json::Value {
    serde_json::json!({
        "error": {
            "message": message,
            "type": type_,
            "param": null,
            "code": null,
        }
    })
}

// -------- wire format + prefix --------

#[tokio::test]
async fn serializes_request_and_applies_passage_prefix() {
    let server = MockServer::start().await;

    // Inspector mock: captures the request body.
    let captured: Arc<tokio::sync::Mutex<Option<serde_json::Value>>> =
        Arc::new(tokio::sync::Mutex::new(None));
    let captured_clone = Arc::clone(&captured);

    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .and(header("authorization", "Bearer test-secret"))
        .respond_with(move |req: &Request| {
            let body: serde_json::Value = serde_json::from_slice(&req.body).unwrap();
            let c = Arc::clone(&captured_clone);
            tokio::spawn(async move {
                *c.lock().await = Some(body);
            });
            // `async-openai`'s `CreateEmbeddingResponse` requires
            // `object`, `model`, `data`, `usage` — give the minimum
            // viable payload.
            ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "object": "list",
                "model": "Qwen/Qwen3-Embedding-4B",
                "data": [
                    { "object": "embedding", "index": 0, "embedding": [0.1f32, 0.2, 0.3, 0.4] }
                ],
                "usage": { "prompt_tokens": 1, "total_tokens": 1 }
            }))
        })
        .mount(&server)
        .await;

    let emb = embedder_for_mock(&server.uri(), 4);
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
                "object": "list",
                "model": "Qwen/Qwen3-Embedding-4B",
                "data": [
                    { "object": "embedding", "index": 0, "embedding": [0.0f32, 0.0, 0.0, 0.0] }
                ],
                "usage": { "prompt_tokens": 1, "total_tokens": 1 }
            }))
        })
        .mount(&server)
        .await;

    let emb = embedder_for_mock(&server.uri(), 4);
    emb.embed(vec!["find this".into()], EmbedKind::Query)
        .await
        .unwrap();

    tokio::time::sleep(Duration::from_millis(50)).await;
    let body = captured.lock().await.clone().unwrap();
    assert_eq!(body["input"], serde_json::json!(["query: find this"]));
}

// -------- retry + rate-limit --------

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
                // async-openai parses a 429 body as `WrappedError` JSON
                // and retries on `429` unless `type=insufficient_quota`.
                ResponseTemplate::new(429)
                    .set_body_json(error_body("rate limited: slow down", Some("rate_limit")))
            } else {
                ResponseTemplate::new(200).set_body_json(serde_json::json!({
                    "object": "list",
                    "model": "Qwen/Qwen3-Embedding-4B",
                    "data": [
                        { "object": "embedding", "index": 0, "embedding": [1.0f32, 0.0, 0.0, 0.0] }
                    ],
                    "usage": { "prompt_tokens": 1, "total_tokens": 1 }
                }))
            }
        })
        .mount(&server)
        .await;

    let emb = embedder_for_mock(&server.uri(), 4);
    let out = emb
        .embed(vec!["retry me".into()], EmbedKind::Passage)
        .await
        .expect("should succeed after two 429s");
    assert_eq!(out[0], vec![1.0, 0.0, 0.0, 0.0]);
    assert!(
        counter.load(Ordering::SeqCst) >= 3,
        "expected >=3 calls (2 failures + 1 success); saw {}",
        counter.load(Ordering::SeqCst)
    );
}

#[tokio::test]
async fn rate_limited_after_max_retries() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(
            ResponseTemplate::new(429)
                .set_body_json(error_body("rate limit exceeded", Some("rate_limit"))),
        )
        .mount(&server)
        .await;

    let emb = embedder_for_mock(&server.uri(), 4);
    let err = emb
        .embed(vec!["hi".into()], EmbedKind::Passage)
        .await
        .expect_err("should return RateLimited after retries");
    assert!(
        matches!(err, rag_engine_embed::EmbedError::RateLimited),
        "expected RateLimited, got {err:?}"
    );
}

// -------- validation --------

#[tokio::test]
async fn invalid_response_on_dim_mismatch() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "object": "list",
            "model": "Qwen/Qwen3-Embedding-4B",
            // Configured dim is 4, response has 5 -> must be flagged.
            "data": [
                { "object": "embedding", "index": 0, "embedding": [0.0, 0.1, 0.2, 0.3, 0.4] }
            ],
            "usage": { "prompt_tokens": 1, "total_tokens": 1 }
        })))
        .mount(&server)
        .await;

    let emb = embedder_for_mock(&server.uri(), 4);
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
        .respond_with(
            ResponseTemplate::new(400).set_body_json(error_body("bad input", Some("invalid"))),
        )
        .mount(&server)
        .await;

    let emb = embedder_for_mock(&server.uri(), 4);
    let err = emb
        .embed(vec!["x".into()], EmbedKind::Passage)
        .await
        .expect_err("400 must surface");
    // async-openai loses the numeric status code. The refactor maps to
    // `Upstream { status: 0, body }` with the upstream error message
    // preserved — terminal, non-retryable, diagnosable.
    match err {
        rag_engine_embed::EmbedError::Upstream { body, .. } => {
            assert!(
                body.to_ascii_lowercase().contains("bad input"),
                "expected upstream body to preserve the message, got {body:?}"
            );
        }
        other => panic!("unexpected error: {other:?}"),
    }
}

// -------- batching + order --------

#[tokio::test]
async fn batches_large_inputs_preserving_order() {
    let server = MockServer::start().await;
    // Echo each input's trailing number into the first float slot so we
    // can verify ordering under parallel sub-batches.
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .respond_with(|req: &Request| {
            let body: serde_json::Value = serde_json::from_slice(&req.body).unwrap();
            let inputs = body["input"].as_array().unwrap();
            let data: Vec<serde_json::Value> = inputs
                .iter()
                .enumerate()
                .map(|(i, s)| {
                    let id_str = s.as_str().unwrap().rsplit('-').next().unwrap();
                    let id: f32 = id_str.parse().unwrap_or(-1.0);
                    serde_json::json!({
                        "object": "embedding",
                        "index": i,
                        "embedding": vec![id, 0.0, 0.0, 0.0]
                    })
                })
                .collect();
            ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "object": "list",
                "model": "Qwen/Qwen3-Embedding-4B",
                "data": data,
                "usage": { "prompt_tokens": 1, "total_tokens": 1 }
            }))
        })
        .mount(&server)
        .await;

    // Force small batch_size so we split into multiple sub-batches.
    let opts = OpenAICompatOptions {
        id: "sf:test".into(),
        model_name: "Qwen/Qwen3-Embedding-4B".into(),
        dimension: 4,
        max_input_tokens: 8192,
        api_url: server.uri(),
        api_key: "test-secret".into(),
        passage_prefix: None,
        query_prefix: None,
        timeout: Duration::from_secs(5),
        concurrency: 4,
        batch_size: 7,
        max_retries: 2,
    };
    let emb = OpenAICompatEmbedder::new(opts).unwrap();

    let texts: Vec<String> = (0..30).map(|i| format!("item-{i:03}")).collect();
    let out = emb.embed(texts, EmbedKind::Passage).await.unwrap();
    assert_eq!(out.len(), 30);
    for (i, v) in out.iter().enumerate() {
        assert_eq!(v[0] as u32, i as u32, "position {i} out of order");
    }
}

// -------- provider-agnostic smoke --------

/// Proves the user's load-bearing "works with any OpenAI-compatible
/// provider" claim. Two wiremock servers stand up, one pretending to be
/// SiliconFlow and one pretending to be OpenRouter (different `model` +
/// `api_url` only, identical wire format since the protocol is shared).
/// We point the exact same `OpenAICompatEmbedder` type at each, swapping
/// only the three env-driven knobs, and assert both produce correct
/// vectors.
#[tokio::test]
async fn provider_agnostic_siliconflow_and_openrouter() {
    // "SiliconFlow" mock server.
    let sf = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .and(header("authorization", "Bearer sf-secret"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "object": "list",
            "model": "Qwen/Qwen3-Embedding-4B",
            "data": [{
                "object": "embedding", "index": 0,
                "embedding": [0.11f32, 0.22, 0.33, 0.44]
            }],
            "usage": { "prompt_tokens": 1, "total_tokens": 1 }
        })))
        .mount(&sf)
        .await;

    // "OpenRouter" mock server — deliberately responds at a different
    // base URL, with a different `model` identifier and vector values,
    // to confirm the embedder doesn't bake in provider-specific
    // assumptions.
    let or_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/embeddings"))
        .and(header("authorization", "Bearer or-secret"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "object": "list",
            "model": "openai/text-embedding-3-small",
            "data": [{
                "object": "embedding", "index": 0,
                "embedding": [0.9f32, 0.8, 0.7, 0.6]
            }],
            "usage": { "prompt_tokens": 1, "total_tokens": 1 }
        })))
        .mount(&or_server)
        .await;

    // SiliconFlow-flavored config.
    let sf_emb = OpenAICompatEmbedder::new(OpenAICompatOptions {
        id: "siliconflow:qwen3-embedding-4b".into(),
        model_name: "Qwen/Qwen3-Embedding-4B".into(),
        dimension: 4,
        max_input_tokens: 8192,
        api_url: sf.uri(),
        api_key: "sf-secret".into(),
        passage_prefix: Some("passage: ".into()),
        query_prefix: Some("query: ".into()),
        timeout: Duration::from_secs(5),
        concurrency: 2,
        batch_size: 32,
        max_retries: 2,
    })
    .unwrap();

    // OpenRouter-flavored config (different prefixes, no Qwen prefix).
    let or_emb = OpenAICompatEmbedder::new(OpenAICompatOptions {
        id: "openrouter:openai/text-embedding-3-small".into(),
        model_name: "openai/text-embedding-3-small".into(),
        dimension: 4,
        max_input_tokens: 8191,
        api_url: or_server.uri(),
        api_key: "or-secret".into(),
        passage_prefix: None,
        query_prefix: None,
        timeout: Duration::from_secs(5),
        concurrency: 2,
        batch_size: 32,
        max_retries: 2,
    })
    .unwrap();

    let sf_out = sf_emb
        .embed(vec!["hi".into()], EmbedKind::Query)
        .await
        .unwrap();
    let or_out = or_emb
        .embed(vec!["hi".into()], EmbedKind::Query)
        .await
        .unwrap();

    assert_eq!(sf_out[0], vec![0.11, 0.22, 0.33, 0.44]);
    assert_eq!(or_out[0], vec![0.9, 0.8, 0.7, 0.6]);
}

// -------- config loader --------

/// Required-env validation. We clear the env first, then call the
/// loader; it must produce `EmbedError::Config`. This test is NOT
/// parallel-safe (it mutates process env) — the `serial_test` crate
/// would be overkill for a single test, so we scope the env mutation
/// tightly and restore on exit.
#[tokio::test]
async fn config_loader_missing_env_errors() {
    let keys = [
        "LLM_PROVIDER",
        "LLM_API_URL",
        "LLM_API_KEY",
        "LLM_MODEL",
        "LLM_EMBEDDING_DIM",
    ];
    let prior: Vec<_> = keys.iter().map(|k| (*k, std::env::var(k).ok())).collect();
    for k in &keys {
        std::env::remove_var(k);
    }
    let got = load_from_env_and_file(None);
    // Restore env before assertion so a test-failure panic doesn't
    // leak state to siblings.
    for (k, v) in prior {
        if let Some(v) = v {
            std::env::set_var(k, v);
        } else {
            std::env::remove_var(k);
        }
    }
    let err = got.expect_err("empty env must produce a Config error");
    assert!(matches!(err, rag_engine_embed::EmbedError::Config(_)));
}

#[tokio::test]
async fn config_build_fake_succeeds() {
    let cfg = EmbedderConfig {
        provider: Provider::Fake(FakeConfig {
            id: "fake:built".into(),
            dimension: 128,
            max_input_tokens: 8192,
        }),
    };
    let emb = build(&cfg).expect("fake must build with no env");
    assert_eq!(emb.id(), "fake:built");
    assert_eq!(emb.dimension(), 128);
}

#[tokio::test]
async fn config_build_openai_compat_direct() {
    // Direct construction via the typed config bypasses env loading;
    // exercises `build()` for the openai_compat variant.
    let cfg = EmbedderConfig {
        provider: Provider::OpenAiCompat(OpenAICompatConfig {
            id: Some("oc:direct".into()),
            model: "Qwen/Qwen3-Embedding-4B".into(),
            dimension: 2560,
            api_url: "https://api.example.com/v1".into(),
            api_key: "k".into(),
            passage_prefix: None,
            query_prefix: None,
            max_input_tokens: 8192,
            request_timeout_secs: 30,
            concurrency: 4,
            batch_size: 32,
            max_retries: 4,
        }),
    };
    let emb = build(&cfg).expect("valid config builds");
    assert_eq!(emb.id(), "oc:direct");
    assert_eq!(emb.dimension(), 2560);
}

// -------- real API roundtrip --------

/// Calls the real upstream configured via env. Skipped when
/// `LLM_API_KEY` is absent (sanctioned skip per TESTING.md for paid
/// external APIs). Any OpenAI-compatible endpoint works — set
/// `LLM_API_URL`, `LLM_API_KEY`, `LLM_MODEL`, `LLM_EMBEDDING_DIM`.
#[tokio::test]
async fn real_openai_compat_roundtrip() {
    let Ok(api_key) = std::env::var("LLM_API_KEY") else {
        eprintln!("SKIP: LLM_API_KEY not set (paid external API — sanctioned skip)");
        return;
    };
    if api_key.trim().is_empty() {
        eprintln!("SKIP: LLM_API_KEY empty");
        return;
    }

    let api_url =
        std::env::var("LLM_API_URL").unwrap_or_else(|_| "https://api.siliconflow.cn/v1".into());
    let model = std::env::var("LLM_MODEL").unwrap_or_else(|_| "Qwen/Qwen3-Embedding-0.6B".into());
    let dim: u32 = std::env::var("LLM_EMBEDDING_DIM")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(1024);

    let opts = OpenAICompatOptions {
        id: "openai-compat:roundtrip".into(),
        model_name: model,
        dimension: dim,
        max_input_tokens: 8192,
        api_url,
        api_key,
        passage_prefix: Some("passage: ".into()),
        query_prefix: Some("query: ".into()),
        timeout: Duration::from_secs(30),
        concurrency: 2,
        batch_size: 32,
        max_retries: 4,
    };
    let emb = OpenAICompatEmbedder::new(opts).unwrap();
    let out = emb
        .embed(
            vec!["hiveloop rag engine smoke test".into()],
            EmbedKind::Query,
        )
        .await
        .expect("real API call should succeed");
    assert_eq!(out.len(), 1);
    assert_eq!(out[0].len(), dim as usize);
    let norm: f32 = out[0].iter().map(|x| x * x).sum::<f32>().sqrt();
    assert!(norm > 0.1, "expected non-zero vector");
}
