//! Integration tests for `verifier_client` using a fake OpenAI endpoint.
//!
//! Verifies that:
//! 1. The request body has `response_format.json_schema.strict = true`,
//!    `temperature = 0`, and the schema bytes we expect.
//! 2. `usage.prompt_tokens_details.cached_tokens` lands on
//!    `VerifierRawResponse.cached_input_tokens`.
//! 3. Non-success status codes surface as `VerifierError::Status`.

use std::time::Duration;

use llm::{VerifierBackend, VerifierClient, VerifierError, VerifierRequest};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, Request, ResponseTemplate};

fn schema_value() -> serde_json::Value {
    serde_json::from_str(llm::VERIFIER_VERDICT_SCHEMA).unwrap()
}

#[tokio::test]
async fn happy_path_strict_outputs_and_cached_tokens() {
    let server = MockServer::start().await;

    let received_body = std::sync::Arc::new(std::sync::Mutex::new(serde_json::Value::Null));
    let captured = received_body.clone();

    Mock::given(method("POST"))
        .and(path("/chat/completions"))
        .respond_with(move |req: &Request| {
            let body: serde_json::Value = serde_json::from_slice(&req.body).unwrap();
            *captured.lock().unwrap() = body.clone();
            ResponseTemplate::new(200).set_body_json(json!({
                "choices": [{
                    "message": {
                        "role": "assistant",
                        "content": r#"{"verdict":"users_turn","confidence":"high","instruction":""}"#,
                    }
                }],
                "usage": {
                    "prompt_tokens": 1234,
                    "completion_tokens": 17,
                    "prompt_tokens_details": { "cached_tokens": 800 }
                }
            }))
        })
        .mount(&server)
        .await;

    let client = VerifierClient::new(
        VerifierBackend::OpenAI {
            api_key: "<provider-api-key>".into(),
            base_url: server.uri(),
            model: "gpt-5-nano".into(),
        },
        Duration::from_secs(5),
        llm::VERIFIER_SYSTEM_PROMPT,
        llm::VERIFIER_VERDICT_SCHEMA,
    )
    .unwrap();

    let schema = schema_value();
    let resp = client
        .verify(VerifierRequest {
            system: llm::VERIFIER_SYSTEM_PROMPT,
            schema: &schema,
            user: "## Agent system prompt\nx\n\n## Conversation\n[]",
        })
        .await
        .expect("verify ok");

    assert_eq!(resp.input_tokens, 1234);
    assert_eq!(resp.cached_input_tokens, 800);
    assert_eq!(resp.output_tokens, 17);
    assert_eq!(resp.model_used, "gpt-5-nano");
    assert!(resp.raw_json.contains("users_turn"));

    let body = received_body.lock().unwrap().clone();
    assert_eq!(body["model"], "gpt-5-nano");
    assert_eq!(body["temperature"], 0);
    assert_eq!(body["response_format"]["type"], "json_schema");
    assert_eq!(body["response_format"]["json_schema"]["strict"], true);
    assert_eq!(
        body["response_format"]["json_schema"]["name"],
        "verifier_verdict"
    );
    assert_eq!(
        body["response_format"]["json_schema"]["schema"], schema,
        "schema bytes drift will break OpenAI prefix cache"
    );
}

#[tokio::test]
async fn non_success_status_surfaces_as_error() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/chat/completions"))
        .respond_with(ResponseTemplate::new(429).set_body_string("rate limited"))
        .mount(&server)
        .await;

    let client = VerifierClient::new(
        VerifierBackend::OpenAI {
            api_key: "<provider-api-key>".into(),
            base_url: server.uri(),
            model: "gpt-5-nano".into(),
        },
        Duration::from_secs(5),
        llm::VERIFIER_SYSTEM_PROMPT,
        llm::VERIFIER_VERDICT_SCHEMA,
    )
    .unwrap();

    let schema = schema_value();
    let err = client
        .verify(VerifierRequest {
            system: llm::VERIFIER_SYSTEM_PROMPT,
            schema: &schema,
            user: "x",
        })
        .await
        .expect_err("expected error");

    match err {
        VerifierError::Status { status, .. } => assert_eq!(status, 429),
        other => panic!("unexpected error variant: {other:?}"),
    }
}
