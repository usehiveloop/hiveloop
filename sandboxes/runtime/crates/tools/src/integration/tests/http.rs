use crate::integration::*;
use crate::ToolExecutor;

#[tokio::test]
async fn test_integration_tool_execute_success() {
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let server = MockServer::start().await;
    let response_body = serde_json::json!({
        "id": 42,
        "number": 123,
        "title": "Test PR",
        "state": "open"
    });

    Mock::given(method("POST"))
        .and(path("/integrations/github/actions/create_pull_request"))
        .respond_with(ResponseTemplate::new(200).set_body_json(&response_body))
        .expect(1)
        .mount(&server)
        .await;

    let executor = IntegrationToolExecutor::new(
        "github".to_string(),
        "create_pull_request".to_string(),
        "Create a PR".to_string(),
        serde_json::json!({}),
        server.uri(),
    );

    let result = executor
        .execute(serde_json::json!({"title": "Test PR"}))
        .await
        .expect("should succeed");

    let parsed: serde_json::Value =
        serde_json::from_str(&result).expect("result should be valid JSON");
    assert_eq!(parsed["number"], 123);
    assert_eq!(parsed["state"], "open");
}

#[tokio::test]
async fn test_integration_tool_execute_error_response() {
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let server = MockServer::start().await;
    let error_body = serde_json::json!({
        "error": "Unknown action 'bad_action' for integration 'github'",
        "available_actions": ["create_pull_request", "list_issues"]
    });

    Mock::given(method("POST"))
        .and(path("/integrations/github/actions/bad_action"))
        .respond_with(ResponseTemplate::new(404).set_body_json(&error_body))
        .expect(1)
        .mount(&server)
        .await;

    let executor = IntegrationToolExecutor::new(
        "github".to_string(),
        "bad_action".to_string(),
        "Bad action".to_string(),
        serde_json::json!({}),
        server.uri(),
    );

    // 404 is a client error — should be returned as-is (not retried)
    let result = executor.execute(serde_json::json!({})).await;
    assert!(
        result.is_ok(),
        "client errors should be returned as Ok (passthrough)"
    );
    let body = result.unwrap();
    assert!(body.contains("Unknown action"));
}

#[tokio::test]
async fn test_integration_tool_execute_retry_on_server_error() {
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let server = MockServer::start().await;

    // First two calls return 500, third returns 200
    Mock::given(method("POST"))
        .and(path("/integrations/github/actions/list_issues"))
        .respond_with(ResponseTemplate::new(500).set_body_string("internal error"))
        .up_to_n_times(2)
        .mount(&server)
        .await;

    Mock::given(method("POST"))
        .and(path("/integrations/github/actions/list_issues"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!([{"id": 1}])))
        .mount(&server)
        .await;

    let executor = IntegrationToolExecutor::new(
        "github".to_string(),
        "list_issues".to_string(),
        "List issues".to_string(),
        serde_json::json!({}),
        server.uri(),
    );

    let result = executor.execute(serde_json::json!({})).await;
    assert!(result.is_ok(), "should succeed after retries");
}

#[tokio::test]
async fn test_integration_tool_execute_retry_exhausted() {
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/integrations/github/actions/list_issues"))
        .respond_with(ResponseTemplate::new(500).set_body_string("internal error"))
        .mount(&server)
        .await;

    let executor = IntegrationToolExecutor::new(
        "github".to_string(),
        "list_issues".to_string(),
        "List issues".to_string(),
        serde_json::json!({}),
        server.uri(),
    );

    let result = executor.execute(serde_json::json!({})).await;
    assert!(result.is_err(), "should fail after retries exhausted");
    assert!(result.unwrap_err().contains("Server error"));
}

#[tokio::test]
async fn test_integration_tool_sends_correct_request_body() {
    use wiremock::matchers::{body_partial_json, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/integrations/slack/actions/send_message"))
        .and(body_partial_json(serde_json::json!({
            "params": {
                "channel": "#general",
                "text": "hello"
            }
        })))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({"ok": true})))
        .expect(1)
        .mount(&server)
        .await;

    let executor = IntegrationToolExecutor::new(
        "slack".to_string(),
        "send_message".to_string(),
        "Send message".to_string(),
        serde_json::json!({}),
        server.uri(),
    );

    let result = executor
        .execute(serde_json::json!({"channel": "#general", "text": "hello"}))
        .await;
    assert!(result.is_ok());
}
