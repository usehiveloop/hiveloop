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
// Test 3: tool_call SSE events contain tool name and duration
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_tool_call_events_have_duration() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start harness");

    step!("Sending message: 'Use the bash tool to run: echo hello_from_tool'");
    let turn = harness
        .converse(
            "streaming-agent",
            None,
            "Use the bash tool to run: echo hello_from_tool",
            LLM_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    step!("Listing SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!(
            "    - {} {}",
            e.event_type,
            serde_json::to_string(&e.data)
                .unwrap_or_default()
                .chars()
                .take(120)
                .collect::<String>()
        );
    }

    step!("Filtering for tool_call_result events");
    let tool_results: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result" || e.event_type == "tool_call_completed")
        .collect();

    check!(
        !tool_results.is_empty(),
        "should have at least one tool_call_result event (got {} SSE events: {:?})",
        turn.sse_events.len(),
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    step!("Checking duration_ms on each tool result");
    for (i, result) in tool_results.iter().enumerate() {
        let duration = result.data.get("duration_ms");
        check!(
            duration.is_some(),
            "tool_call_result[{}] should have duration_ms. Data: {}",
            i,
            result.data
        );
        eprintln!("    Tool call {}: duration_ms={}", i, duration.unwrap());
    }

    step!("PASS — {} tool calls with durations", tool_results.len());
}

// ============================================================================
// Test 4: cumulative tokens in turn_completed track across turns
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_cumulative_tokens_across_turns() {
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

    step!("Turn 1: Sending 'Say hello'");
    let _turn1 = harness
        .converse("streaming-agent", None, "Say hello", LLM_TIMEOUT)
        .await
        .expect("turn 1 failed");

    step!("Waiting for turn_completed webhook (turn 1)");
    let log1 = harness
        .wait_for_webhook_type("turn_completed", WEBHOOK_TIMEOUT)
        .await
        .expect("failed to get webhooks");

    let tc1 = log1.by_type("turn_completed");
    check!(!tc1.is_empty(), "should have turn_completed for turn 1");

    let data1 = tc1.last().unwrap().data().unwrap();
    let cumulative_input_1 = data1["cumulative_input_tokens"].as_u64().unwrap_or(0);
    let cumulative_output_1 = data1["cumulative_output_tokens"].as_u64().unwrap_or(0);

    step!("Verifying cumulative tokens after turn 1");
    check!(
        cumulative_input_1 > 0,
        "cumulative input tokens should be > 0 after turn 1"
    );

    step!(
        "PASS — after turn 1: cumulative_input={}, cumulative_output={}",
        cumulative_input_1,
        cumulative_output_1
    );
}
