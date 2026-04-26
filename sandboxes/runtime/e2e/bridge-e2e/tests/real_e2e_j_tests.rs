#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! Real E2E tests that use actual LLM calls via Fireworks.
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test real_e2e_tests -- --ignored
//! ```
//!
//! Tests run serially to avoid Fireworks rate limits.

use bridge_e2e::{check, check_eq, step, ConversationTurn, TestHarness};
use std::time::Duration;

/// Default timeout for LLM responses (real model with tool loops).
/// With max_turns=5, each Fireworks round trip can be 15-40s (depending on
/// context size and tool count), so worst case ~240s for a full 5-turn loop.
const LLM_TIMEOUT: Duration = Duration::from_secs(300);

/// Maximum retries for a conversation turn that returns an empty/error response.
/// Real LLM APIs can have transient failures (rate limits, empty responses).
const MAX_RETRIES: usize = 2;

/// Skip test if FIREWORKS_API_KEY is not set.
fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping real E2E test");
        return false;
    }
    true
}

/// Send a conversation turn with retries on empty/error responses.
/// Real LLM APIs can intermittently return empty responses or transient errors.
async fn converse_with_retry(
    harness: &TestHarness,
    agent_id: &str,
    message: &str,
    label: &str,
) -> ConversationTurn {
    let mut last_turn = None;
    for attempt in 0..MAX_RETRIES {
        if attempt > 0 {
            step!(
                "[{}] Retrying (attempt {}/{})",
                label,
                attempt + 1,
                MAX_RETRIES
            );
            tokio::time::sleep(Duration::from_secs(2)).await;
        }

        step!(
            "[{}] Sending message to '{}': '{}'",
            label,
            agent_id,
            &message[..message.len().min(80)]
        );
        let turn = harness
            .converse(agent_id, None, message, LLM_TIMEOUT)
            .await
            .expect("conversation failed");

        let has_error = turn.sse_events.iter().any(|e| e.event_type == "error");

        if !turn.response_text.is_empty() && !has_error {
            step!(
                "[{}] Got response ({} chars)",
                label,
                turn.response_text.len()
            );
            eprintln!(
                "    Response: {:?}",
                &turn.response_text[..turn.response_text.len().min(200)]
            );

            step!(
                "[{}] SSE events received ({} total)",
                label,
                turn.sse_events.len()
            );
            for e in &turn.sse_events {
                eprintln!("    - {}", e.event_type);
            }
            return turn;
        }

        eprintln!(
            "[{}] attempt {} got empty/error response. Events: {:?}",
            label,
            attempt + 1,
            turn.sse_events
                .iter()
                .map(|e| format!("{}:{}", e.event_type, {
                    let s = e.data.to_string();
                    &s[..s.floor_char_boundary(120.min(s.len()))].to_string()
                }))
                .collect::<Vec<_>>()
        );
        last_turn = Some(turn);
    }

    // Return the last turn — the test assertion will fail with diagnostics
    last_turn.unwrap()
}

/// Assert that at least one of the given tools was called, checking SSE events.
/// Unlike `harness.assert_any_tool_called`, this catches built-in tools (Glob,
/// Grep, Read, etc.) that are handled by the bridge runtime and don't appear in
/// the MCP tool call log.
fn assert_any_tool_called_in_sse(turn: &ConversationTurn, tool_names: &[&str], label: &str) {
    let called_tools: Vec<String> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| {
            e.data
                .get("name")
                .and_then(|n| n.as_str())
                .map(String::from)
        })
        .collect();

    let found = called_tools
        .iter()
        .any(|t| tool_names.contains(&t.as_str()));
    check!(
        found,
        "[{}] at least one of {:?} should be called. Tools called (from SSE): {:?}",
        label,
        tool_names,
        called_tools
    );
}

/// Assert response is non-empty with diagnostic output on failure.
fn assert_response_not_empty(turn: &ConversationTurn, label: &str) {
    check!(
        !turn.response_text.is_empty(),
        "[{}] response should not be empty. SSE events received: {:?}",
        label,
        turn.sse_events
            .iter()
            .map(|e| format!("{}:{}", e.event_type, {
                let s = e.data.to_string();
                &s[..s.floor_char_boundary(200.min(s.len()))].to_string()
            }))
            .collect::<Vec<_>>()
    );
}

// ============================================================================
// Test: Executor — Background Bash
// Verifies: bash tool called with background: true, notification round-trip
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_executor_background_bash() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Creating conversation for executor agent");
    // Create conversation and send message manually so we can read multiple turns.
    // Background bash produces two turns:
    //   Turn 1: LLM calls bash(background:true) -> gets immediate "running" -> responds
    //   Turn 2: Background command completes -> notification -> LLM reports output
    let resp = harness
        .create_conversation("executor")
        .await
        .expect("create conversation");
    let body: serde_json::Value = resp.json().await.expect("parse create response");
    let conversation_id = body["conversation_id"]
        .as_str()
        .expect("conversation_id")
        .to_string();

    step!("Conversation created: {}", conversation_id);

    // Register for conversation logging
    harness
        .register_conversation(&conversation_id, "executor")
        .await;

    step!("Sending background bash command");
    let msg_resp = harness
        .send_message(
            &conversation_id,
            "Run `echo 'background_task_complete_marker_12345'` in the background using the bash tool with background set to true. After it completes, report the output.",
        )
        .await
        .expect("send message");
    check!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "message send returned success/202 (got {})",
        msg_resp.status()
    );

    step!("Streaming SSE events across 2 turns (background launch + completion)");
    // Read SSE events across 2 turns (2 "done" events).
    // Turn 1: agent acknowledges background launch.
    // Turn 2: agent processes the background completion notification and reports output.
    let (events, response_text) = harness
        .stream_sse_until_done_count(&conversation_id, 2, LLM_TIMEOUT)
        .await
        .expect("stream SSE events");

    step!(
        "Collected {} SSE events, response: {} chars",
        events.len(),
        response_text.len()
    );
    for e in &events {
        eprintln!("    - {}", e.event_type);
    }

    // LLM responses are non-deterministic; a missing text response is acceptable
    // as long as the tool calls executed correctly.
    if response_text.is_empty() {
        step!("Warning: empty response text, checking tool calls only");
    } else {
        eprintln!(
            "    Response: {:?}",
            &response_text[..response_text.len().min(200)]
        );
    }

    step!("Verifying bash tool was called");
    // Verify the bash tool was called with background: true in SSE events
    let bash_starts: Vec<_> = events
        .iter()
        .filter(|e| {
            e.event_type == "tool_call_start"
                && e.data.get("name").and_then(|n| n.as_str()) == Some("bash")
        })
        .collect();

    check!(!bash_starts.is_empty(), "bash tool should have been called");

    step!("Verifying bash was called with background: true");
    // Check that at least one bash call had background: true
    let used_background = bash_starts.iter().any(|e| {
        let args = e.data.get("arguments");
        // Handle both object and string-encoded arguments
        match args {
            Some(serde_json::Value::Object(obj)) => {
                obj.get("background") == Some(&serde_json::json!(true))
            }
            Some(serde_json::Value::String(s)) => s.contains("background") && s.contains("true"),
            _ => false,
        }
    });

    check!(
        used_background,
        "bash should have been called with background: true"
    );

    step!("Verifying response contains background command output marker");
    // Verify the response contains the marker from the background command output.
    // This proves the full round-trip: command ran -> notification sent -> agent received it.
    check!(
        response_text.contains("background_task_complete_marker_12345"),
        "response should contain the background command output marker, got: {}",
        &response_text[..response_text.len().min(500)]
    );

    step!("PASS — background bash executed, notification round-trip completed");
}
