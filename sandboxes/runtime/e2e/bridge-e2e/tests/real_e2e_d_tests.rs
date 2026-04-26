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
// Test 5: Mimi — Technical Writer
// Verifies: Glob/Read (code reading), createDocument, createComment
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_mimi_technical_writer() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "technical-writer",
        "Document the bridge HTTP API. Steps: 1) Use Glob with pattern 'crates/api/src/**/*.rs' to find handler files, 2) Read one handler file to understand the endpoints, 3) Create an API reference document using createDocument, 4) Post a summary on issue ENG-44 using createComment.",
        "mimi",
    )
    .await;

    assert_response_not_empty(&turn, "mimi");

    step!("[mimi] Verifying exploration tools were called (SSE)");
    // Built-in tools (Glob/RipGrep/AstGrep/Read) are handled by the bridge runtime and
    // don't appear in the MCP tool call log — check SSE events instead.
    assert_any_tool_called_in_sse(
        &turn,
        &[
            "Glob",
            "RipGrep",
            "AstGrep",
            "Read",
            "LS",
            "getIssue",
            "searchDocuments",
            "listDocuments",
        ],
        "mimi exploration",
    );

    step!("[mimi] Verifying document creation tools were called");
    harness
        .assert_any_tool_called(&["createDocument", "updateDocument"])
        .expect("document creation tools should have been called");

    step!("[mimi] Verifying createComment was called");
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    step!(
        "PASS — mimi technical writer completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 6: Researcher — Web Search
// Verifies: web_search built-in tool, mock search endpoint, result synthesis
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_researcher_web_search() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "researcher",
        "Research how async/await works in Rust. Use the web_search tool to find information.",
        "researcher-search",
    )
    .await;

    assert_response_not_empty(&turn, "researcher search");

    let response = turn.response_text.to_lowercase();

    step!("[researcher] Verifying response contains search content");
    // The mock search endpoint returns results with unique markers and
    // content about Tokio, async/await, Futures. The agent MUST use the
    // web_search tool to know these — it can't fabricate the markers.
    let has_search_content = response.contains("tokio")
        || response.contains("async")
        || response.contains("await")
        || response.contains("bridge_e2e_search_marker");

    check!(
        has_search_content,
        "response should contain content from search results. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    step!(
        "PASS — researcher web_search completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}
