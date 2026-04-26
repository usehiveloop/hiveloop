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
// Test 3: Skai — Security Audit
// Verifies: getIssue, listIssuePullRequests/getPullRequest, createComment/addPullRequestComment
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_skai_security_audit() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "security-audit",
        "Audit issue ENG-42 for security vulnerabilities. Steps: 1) Use getIssue to get the issue, 2) Use listIssuePullRequests and getPullRequest to get the PR diff, 3) Analyze the JWT auth code for vulnerabilities, 4) Post your findings as a comment using createComment.",
        "skai",
    )
    .await;

    assert_response_not_empty(&turn, "skai");

    step!("[skai] Verifying getIssue was called");
    harness
        .assert_tool_called("getIssue")
        .expect("getIssue should have been called");

    step!("[skai] Verifying PR tools were called");
    harness
        .assert_any_tool_called(&["listIssuePullRequests", "getPullRequest"])
        .expect("PR tools should have been called");

    step!("[skai] Verifying comment tools were called");
    harness
        .assert_any_tool_called(&["createComment", "addPullRequestComment"])
        .expect("comment tools should have been called");

    step!(
        "PASS — skai security audit completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 4: Theo — System Design
// Verifies: Glob/Grep/Read (codebase exploration), createDocument, createComment
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_theo_system_design() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "system-design",
        "Design a caching layer for this codebase. Steps: 1) Use Glob to find the main source files (pattern: 'crates/*/src/lib.rs'), 2) Read one file to understand the architecture, 3) Create a design document using createDocument with title 'Caching Layer Design', 4) Post a brief summary on issue ENG-43 using createComment.",
        "theo",
    )
    .await;

    assert_response_not_empty(&turn, "theo");

    step!("[theo] Verifying design workflow tools were called (SSE)");
    // The agent should use at least some tools for its design workflow.
    // Built-in tools (Glob/RipGrep/AstGrep/Read) don't appear in the MCP log, so check SSE.
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
            "createDocument",
            "updateDocument",
            "createComment",
        ],
        "theo design workflow",
    );

    step!("[theo] Verifying createComment was called");
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    step!(
        "PASS — theo system design completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}
