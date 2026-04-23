use crate::providers::retry::is_retryable_error;
use rig::completion::{CompletionError, PromptError};
use rig::message::Message;

fn http_error(err: rig::http_client::Error) -> PromptError {
    PromptError::CompletionError(CompletionError::HttpError(err))
}

#[test]
fn test_retryable_502_bad_gateway() {
    let err = http_error(rig::http_client::Error::InvalidStatusCodeWithMessage(
        http::StatusCode::BAD_GATEWAY,
        "upstream unreachable".into(),
    ));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_retryable_503_service_unavailable() {
    let err = http_error(rig::http_client::Error::InvalidStatusCode(
        http::StatusCode::SERVICE_UNAVAILABLE,
    ));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_retryable_500_internal_server_error() {
    let err = http_error(rig::http_client::Error::InvalidStatusCode(
        http::StatusCode::INTERNAL_SERVER_ERROR,
    ));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_retryable_429_rate_limit() {
    let err = http_error(rig::http_client::Error::InvalidStatusCode(
        http::StatusCode::TOO_MANY_REQUESTS,
    ));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_retryable_stream_ended() {
    let err = http_error(rig::http_client::Error::StreamEnded);
    assert!(is_retryable_error(&err));
}

#[test]
fn test_retryable_network_error() {
    let network_err: Box<dyn std::error::Error + Send + Sync> =
        "connection reset".to_string().into();
    let err = http_error(rig::http_client::Error::Instance(network_err));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_retryable_provider_error_upstream() {
    let err = PromptError::CompletionError(CompletionError::ProviderError(
        "upstream unreachable".into(),
    ));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_retryable_provider_error_overloaded() {
    let err =
        PromptError::CompletionError(CompletionError::ProviderError("model is overloaded".into()));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_not_retryable_401_unauthorized() {
    let err = http_error(rig::http_client::Error::InvalidStatusCodeWithMessage(
        http::StatusCode::UNAUTHORIZED,
        "invalid api key".into(),
    ));
    assert!(!is_retryable_error(&err));
}

#[test]
fn test_not_retryable_400_bad_request() {
    let err = http_error(rig::http_client::Error::InvalidStatusCode(
        http::StatusCode::BAD_REQUEST,
    ));
    assert!(!is_retryable_error(&err));
}

#[test]
fn test_not_retryable_403_forbidden() {
    let err = http_error(rig::http_client::Error::InvalidStatusCode(
        http::StatusCode::FORBIDDEN,
    ));
    assert!(!is_retryable_error(&err));
}

#[test]
fn test_retryable_json_error() {
    let json_err = serde_json::from_str::<String>("not json").unwrap_err();
    let err = PromptError::CompletionError(CompletionError::JsonError(json_err));
    assert!(is_retryable_error(&err));
}

#[test]
fn test_not_retryable_tool_error() {
    let err = PromptError::ToolError(rig::tool::ToolSetError::ToolNotFoundError("x".into()));
    assert!(!is_retryable_error(&err));
}

#[test]
fn test_not_retryable_max_turns() {
    let err = PromptError::MaxTurnsError {
        max_turns: 10,
        chat_history: Box::new(vec![]),
        prompt: Box::new(Message::from("test")),
    };
    assert!(!is_retryable_error(&err));
}

#[test]
fn test_not_retryable_prompt_cancelled() {
    let err = PromptError::PromptCancelled {
        chat_history: Box::new(vec![]),
        reason: "user cancelled".into(),
    };
    assert!(!is_retryable_error(&err));
}

#[test]
fn test_not_retryable_provider_error_generic() {
    let err =
        PromptError::CompletionError(CompletionError::ProviderError("invalid model name".into()));
    assert!(!is_retryable_error(&err));
}
