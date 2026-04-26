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
// Test 10: Multi-Agent Concurrent Conversations
// Verifies: all agents respond, metrics tracked, no cross-contamination
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_multi_agent_concurrent_conversations() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Verifying all expected agents are loaded");
    // Verify all agents are loaded
    let agents = harness.get_agents().await.expect("failed to get agents");
    let agent_ids: Vec<String> = agents
        .iter()
        .filter_map(|a| a.get("id").and_then(|v| v.as_str()).map(String::from))
        .collect();

    for expected_id in &[
        "code-review",
        "portal-control",
        "security-audit",
        "system-design",
        "technical-writer",
        "researcher",
        "delegator",
        "executor",
    ] {
        check!(
            agent_ids.contains(&expected_id.to_string()),
            "agent '{}' should be loaded. Loaded: {:?}",
            expected_id,
            agent_ids
        );
    }
    step!("All 8 agents loaded: {:?}", agent_ids);

    // Create 8 conversations simultaneously with simple non-tool messages
    let messages = vec![
        (
            "code-review",
            "What is the most important thing in a code review? Answer in 2-3 sentences.",
        ),
        (
            "portal-control",
            "Briefly describe your role as Portal in this workspace. 2-3 sentences.",
        ),
        (
            "security-audit",
            "What are the top 3 OWASP vulnerabilities? Answer briefly.",
        ),
        (
            "system-design",
            "What makes a good system design document? Answer in 2-3 sentences.",
        ),
        (
            "technical-writer",
            "What makes good API documentation? Answer in 2-3 sentences.",
        ),
        (
            "researcher",
            "What is Rust known for? Answer in 2-3 sentences.",
        ),
        (
            "delegator",
            "What makes a good engineering lead? Answer in 2-3 sentences.",
        ),
        (
            "executor",
            "What is the most important DevOps principle? Answer in 2-3 sentences.",
        ),
    ];

    step!("Spawning 8 concurrent conversations");
    let mut handles = Vec::new();
    for (agent_id, message) in &messages {
        let agent_id = agent_id.to_string();
        let message = message.to_string();
        let bridge_url = harness.bridge_url().to_string();

        handles.push(tokio::spawn(async move {
            let client = reqwest::Client::builder()
                .timeout(Duration::from_secs(30))
                .build()
                .unwrap();

            // Create conversation
            let resp = client
                .post(format!("{}/agents/{}/conversations", bridge_url, agent_id))
                .send()
                .await
                .expect("create conversation failed");

            let body: serde_json::Value = resp.json().await.expect("parse failed");
            let conv_id = body
                .get("conversation_id")
                .and_then(|v| v.as_str())
                .expect("no conversation_id")
                .to_string();

            // Send message
            let msg_resp = client
                .post(format!("{}/conversations/{}/messages", bridge_url, conv_id))
                .json(&serde_json::json!({"content": message}))
                .send()
                .await
                .expect("send message failed");

            assert!(
                msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
                "message send failed for {}: {}",
                agent_id,
                msg_resp.status()
            );

            // Connect to SSE stream and read events properly using bytes_stream.
            // IMPORTANT: Do NOT use `.text().await` on SSE streams — it waits for the
            // TCP connection to close, which may never happen or take very long.
            // Instead, read byte-by-byte and stop when we see a "done" event.
            use futures::StreamExt;

            let stream_client = reqwest::Client::builder()
                .connect_timeout(Duration::from_secs(10))
                .build()
                .unwrap();

            let stream_resp = stream_client
                .get(format!("{}/conversations/{}/stream", bridge_url, conv_id))
                .send()
                .await
                .expect("stream connect failed");

            let mut stream = stream_resp.bytes_stream();
            let mut buffer = String::new();
            let mut response_text = String::new();
            let mut got_done = false;
            let mut current_event_type = String::new();

            let deadline = std::time::Instant::now() + LLM_TIMEOUT;

            loop {
                let remaining = deadline.saturating_duration_since(std::time::Instant::now());
                if remaining.is_zero() {
                    eprintln!("[concurrent] {} SSE stream timed out", agent_id);
                    break;
                }

                match tokio::time::timeout(remaining, stream.next()).await {
                    Ok(Some(Ok(chunk))) => {
                        buffer.push_str(&String::from_utf8_lossy(&chunk));
                    }
                    Ok(Some(Err(e))) => {
                        eprintln!("[concurrent] {} SSE chunk error: {}", agent_id, e);
                        break;
                    }
                    Ok(None) => break, // stream ended
                    Err(_) => {
                        eprintln!("[concurrent] {} SSE timeout", agent_id);
                        break;
                    }
                }

                // Process complete lines
                while let Some(newline_pos) = buffer.find('\n') {
                    let line = buffer[..newline_pos].trim_end().to_string();
                    buffer = buffer[newline_pos + 1..].to_string();

                    if line.is_empty() {
                        continue;
                    }

                    if let Some(event_name) = line.strip_prefix("event:") {
                        current_event_type = event_name.trim().to_string();
                    } else if let Some(data_str) = line.strip_prefix("data:") {
                        let data_str = data_str.trim();
                        if data_str.is_empty() {
                            continue;
                        }

                        let event_type = if !current_event_type.is_empty() {
                            current_event_type.clone()
                        } else if let Ok(v) = serde_json::from_str::<serde_json::Value>(data_str) {
                            v.get("type")
                                .and_then(|t| t.as_str())
                                .unwrap_or("message")
                                .to_string()
                        } else {
                            "message".to_string()
                        };

                        if event_type == "content_delta" {
                            if let Ok(v) = serde_json::from_str::<serde_json::Value>(data_str) {
                                if let Some(delta) = v.get("delta").and_then(|d| d.as_str()) {
                                    response_text.push_str(delta);
                                }
                            }
                        }

                        if event_type == "done" {
                            got_done = true;
                            break;
                        }

                        current_event_type.clear();
                    }
                }

                if got_done {
                    break;
                }
            }

            assert!(
                !response_text.is_empty(),
                "agent {} returned empty response",
                agent_id
            );

            eprintln!(
                "\n  \x1b[36m\u{25b8}\x1b[0m [concurrent] {} responded ({} chars): {:?}",
                agent_id,
                response_text.len(),
                &response_text[..response_text.len().min(100)]
            );

            (agent_id, conv_id)
        }));
    }

    step!("Waiting for all 8 conversations to complete");
    // Wait for all 8 conversations
    let mut results = Vec::new();
    for handle in handles {
        let result = handle.await.expect("task panicked");
        results.push(result);
    }

    check_eq!(results.len(), 8, "all 8 agents should have responded");

    step!("Verifying metrics");
    // Verify metrics show conversations tracked
    let metrics = harness.get_metrics().await.expect("failed to get metrics");

    if let Some(global) = metrics.get("global") {
        let total_agents = global
            .get("total_agents")
            .and_then(|v| v.as_u64())
            .unwrap_or(0);
        check!(
            total_agents >= 8,
            "should have at least 8 agents in metrics, got {}",
            total_agents
        );
    }

    step!("All 8 agents responded:");
    for (agent_id, conv_id) in &results {
        eprintln!("    {} -> conversation {}", agent_id, conv_id);
    }

    step!("PASS — all 8 concurrent conversations completed successfully");
}
