#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! Observability E2E tests — verify that webhook payloads contain
//! enriched fields (model, timestamps, tokens) and that subagent
//! trace events fire correctly.
//!
//! These tests use a real LLM (Fireworks) and are `#[ignore]` by default:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test observability_e2e_tests -- --ignored
//! ```

use bridge_e2e::{check, step, TestHarness};
use std::time::Duration;

const LLM_TIMEOUT: Duration = Duration::from_secs(120);
const WEBHOOK_TIMEOUT: Duration = Duration::from_secs(15);

fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping");
        return false;
    }
    true
}

// ============================================================================
// Test 1: turn_completed webhook contains enriched token/model data
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_turn_completed_webhook_has_token_data() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start harness");

    step!("Clearing webhook log");
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    step!("Sending message: 'Reply with exactly: hello'");
    let turn = harness
        .converse(
            "streaming-agent",
            None,
            "Reply with exactly: hello",
            LLM_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    step!("Verifying response text is non-empty");
    check!(
        !turn.response_text.is_empty(),
        "response should not be empty"
    );
    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(200)]
    );

    step!(
        "Listing SSE events received ({} total)",
        turn.sse_events.len()
    );
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    step!(
        "Waiting for turn_completed webhook (timeout: {:?})",
        WEBHOOK_TIMEOUT
    );
    let log = harness
        .wait_for_webhook_type("turn_completed", WEBHOOK_TIMEOUT)
        .await
        .expect("failed to get webhooks");

    let turn_completed = log.by_type("turn_completed");
    check!(
        !turn_completed.is_empty(),
        "should have at least one turn_completed webhook"
    );

    let data = turn_completed[0]
        .data()
        .expect("turn_completed should have data");

    step!("Checking turn_completed webhook data fields");
    eprintln!(
        "    Full data: {}",
        serde_json::to_string_pretty(data).unwrap_or_default()
    );

    check!(
        data.get("input_tokens").is_some(),
        "turn_completed should have input_tokens"
    );
    check!(
        data.get("output_tokens").is_some(),
        "turn_completed should have output_tokens"
    );
    check!(
        data.get("model").is_some(),
        "turn_completed should have model"
    );
    check!(
        data.get("timestamp").is_some(),
        "turn_completed should have timestamp"
    );
    check!(
        data.get("turn_number").is_some(),
        "turn_completed should have turn_number"
    );

    let input_tokens = data["input_tokens"].as_u64().unwrap_or(0);
    let output_tokens = data["output_tokens"].as_u64().unwrap_or(0);
    let model = data["model"].as_str().unwrap_or("");

    step!("Verifying token counts are sensible");
    check!(
        input_tokens > 0,
        "input_tokens should be > 0, got {}",
        input_tokens
    );
    check!(
        output_tokens > 0,
        "output_tokens should be > 0, got {}",
        output_tokens
    );
    check!(!model.is_empty(), "model should not be empty");

    step!(
        "PASS — input_tokens={}, output_tokens={}, model={}, turn={}",
        input_tokens,
        output_tokens,
        model,
        data["turn_number"]
    );
}

// ============================================================================
// Test 2: response_completed webhook contains model and timestamp
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_response_completed_webhook_has_model_and_timestamp() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start harness");

    step!("Clearing webhook log");
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    step!("Sending message: 'Reply with exactly: test'");
    let turn = harness
        .converse(
            "streaming-agent",
            None,
            "Reply with exactly: test",
            LLM_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    check!(
        !turn.response_text.is_empty(),
        "response should not be empty"
    );
    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(200)]
    );

    step!("Waiting for response_completed webhook");
    let log = harness
        .wait_for_webhook_type("response_completed", WEBHOOK_TIMEOUT)
        .await
        .expect("failed to get webhooks");

    let response_completed = log.by_type("response_completed");
    check!(
        !response_completed.is_empty(),
        "should have response_completed webhook"
    );

    let data = response_completed[0]
        .data()
        .expect("response_completed should have data");

    step!("Checking response_completed webhook data fields");
    eprintln!(
        "    Full data: {}",
        serde_json::to_string_pretty(data).unwrap_or_default()
    );

    check!(
        data.get("model").is_some(),
        "response_completed should have model"
    );
    check!(
        data.get("timestamp").is_some(),
        "response_completed should have timestamp"
    );
    check!(
        data.get("input_tokens").is_some(),
        "response_completed should have input_tokens"
    );
    check!(
        data.get("output_tokens").is_some(),
        "response_completed should have output_tokens"
    );

    step!(
        "PASS — model={}, timestamp={}, tokens={}+{}",
        data["model"],
        data["timestamp"],
        data["input_tokens"],
        data["output_tokens"]
    );
}
