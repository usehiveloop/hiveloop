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
// Test: Abort — Per-Conversation Cancellation
// Verifies: POST /conversations/{conv_id}/abort cancels the in-flight turn,
// SSE stream receives error(aborted) + done, conversation remains usable.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_abort_conversation() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Creating conversation for researcher agent");
    // Use researcher agent — it has simple tools (web_search, web_fetch) that
    // reliably work with Fireworks and take enough time for the abort to fire.
    let resp = harness
        .create_conversation("researcher")
        .await
        .expect("create conversation");
    let body: serde_json::Value = resp.json().await.expect("parse create response");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("conversation_id")
        .to_string();

    step!("Conversation created: {}", conv_id);
    harness.register_conversation(&conv_id, "researcher").await;

    step!("Sending long research message to trigger tool calls");
    // Send a message that will trigger tool calls (web_search takes time)
    let msg_resp = harness
        .send_message(
            &conv_id,
            "Research the history of the Rust programming language in depth. Use the web_search tool to search for 'Rust programming language history timeline'. Then search for 'Rust borrow checker design'. Then search for 'Rust async await RFC history'. Give me a comprehensive report.",
        )
        .await
        .expect("send message");
    check!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "message send returned success/202 (got {})",
        msg_resp.status()
    );

    step!("Starting SSE reader for both turns (abort + second turn)");
    // Start a single SSE connection that reads across BOTH turns (abort + second turn).
    // The SSE stream endpoint removes the receiver on first connect, so we must keep
    // this single connection alive for the entire test.
    let bridge_url = harness.bridge_url().to_string();
    let conv_id_clone = conv_id.clone();
    let (abort_events_tx, abort_events_rx) = tokio::sync::oneshot::channel();
    let sse_reader = tokio::spawn(async move {
        use futures::StreamExt;

        let stream_client = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .build()
            .unwrap();

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                bridge_url, conv_id_clone
            ))
            .send()
            .await
            .expect("stream connect failed");

        let mut stream = resp.bytes_stream();
        let mut buffer = String::new();
        let mut all_events: Vec<(String, serde_json::Value)> = Vec::new();
        let mut current_event_type = String::new();
        let deadline = std::time::Instant::now() + Duration::from_secs(180);
        let mut done_count = 0usize;
        let mut abort_events_tx = Some(abort_events_tx);

        loop {
            let remaining = deadline.saturating_duration_since(std::time::Instant::now());
            if remaining.is_zero() {
                break;
            }

            match tokio::time::timeout(remaining, stream.next()).await {
                Ok(Some(Ok(chunk))) => {
                    buffer.push_str(&String::from_utf8_lossy(&chunk));
                }
                Ok(Some(Err(_))) | Ok(None) | Err(_) => break,
            }

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
                    let data: serde_json::Value = serde_json::from_str(data_str)
                        .unwrap_or_else(|_| serde_json::Value::String(data_str.to_string()));
                    let event_type = if !current_event_type.is_empty() {
                        current_event_type.clone()
                    } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                        t.to_string()
                    } else {
                        "message".to_string()
                    };

                    all_events.push((event_type.clone(), data));

                    if event_type == "done" {
                        done_count += 1;
                        if done_count == 1 {
                            // First done = abort turn finished. Send abort events back.
                            if let Some(tx) = abort_events_tx.take() {
                                let _ = tx.send(all_events.clone());
                            }
                        }
                        if done_count >= 2 {
                            // Second done = second turn finished. We're done.
                            return all_events;
                        }
                    }
                    current_event_type.clear();
                }
            }
        }

        // If we exit the loop without 2 dones, send abort events if not yet sent
        if let Some(tx) = abort_events_tx.take() {
            let _ = tx.send(all_events.clone());
        }
        all_events
    });

    step!("Waiting 3s for LLM call to begin, then aborting");
    // Wait for the LLM call to begin processing, then abort.
    // 3 seconds is enough for the message to be received and the LLM call to start.
    tokio::time::sleep(Duration::from_secs(3)).await;

    let abort_start = std::time::Instant::now();
    step!("Sending abort request for conversation {}", conv_id);
    let abort_resp = harness
        .abort_conversation(&conv_id)
        .await
        .expect("abort request failed");

    check_eq!(abort_resp.status().as_u16(), 200, "abort should return 200");

    let abort_body: serde_json::Value = abort_resp.json().await.expect("parse abort body");
    check_eq!(
        abort_body.get("status").and_then(|s| s.as_str()),
        Some("aborted"),
        "abort response should have status: aborted"
    );

    step!("Waiting for abort events from SSE reader");
    // Wait for abort events from the SSE reader
    let abort_events = abort_events_rx.await.expect("abort events channel closed");
    let abort_latency = abort_start.elapsed();

    step!(
        "Abort latency: {:?}, collected {} events",
        abort_latency,
        abort_events.len()
    );
    for (t, d) in &abort_events {
        let s = d.to_string();
        eprintln!("    - {}:{}", t, &s[..s.len().min(120)]);
    }

    step!("Verifying error event with code 'aborted'");
    // Verify we got an error event with code "aborted"
    let abort_event = abort_events
        .iter()
        .find(|(t, d)| t == "error" && d.get("code").and_then(|c| c.as_str()) == Some("aborted"));
    check!(
        abort_event.is_some(),
        "expected an error event with code 'aborted'. Events: {:?}",
        abort_events
            .iter()
            .map(|(t, d)| format!("{}:{}", t, d))
            .collect::<Vec<_>>()
    );

    step!("Verifying done event after abort");
    // Verify we got a done event
    let done_event = abort_events.iter().find(|(t, _)| t == "done");
    check!(done_event.is_some(), "expected a 'done' event after abort");

    step!("Verifying abort resolved quickly (< 10s)");
    // The abort should resolve quickly (not wait for the full LLM response).
    // Allow up to 10 seconds — the key point is it should NOT take the full
    // LLM response time (which would be 15-40+ seconds for tool-calling agents).
    check!(
        abort_latency < Duration::from_secs(10),
        "abort should resolve quickly, but took {:?}",
        abort_latency
    );

    step!("Sending second message to verify conversation is still usable");
    // Now verify the conversation is still usable — send another message
    // and expect a normal response. The SSE reader is still connected and
    // will collect events for the second turn.
    let msg2_resp = harness
        .send_message(&conv_id, "What is Rust known for? Answer in one sentence.")
        .await
        .expect("send second message after abort");

    check!(
        msg2_resp.status().is_success() || msg2_resp.status().as_u16() == 202,
        "second message after abort returned success/202 (got {})",
        msg2_resp.status()
    );

    step!("Waiting for SSE reader to collect second turn events");
    // Wait for the SSE reader to collect the second turn's events (until 2nd done)
    let all_events = sse_reader.await.expect("SSE reader task panicked");

    // Find events after the first done (second turn's events)
    let first_done_idx = all_events
        .iter()
        .position(|(t, _)| t == "done")
        .expect("should have at least one done event");
    let turn2_events: Vec<_> = all_events[first_done_idx + 1..].to_vec();

    // Extract response text from second turn's content_delta events
    let response2: String = turn2_events
        .iter()
        .filter(|(t, _)| t == "content_delta")
        .filter_map(|(_, d)| d.get("delta").and_then(|d| d.as_str()))
        .collect();

    step!("Second turn response ({} chars)", response2.len());
    eprintln!("    Response: {:?}", &response2[..response2.len().min(200)]);

    // Log the second turn's response in the same format as other conversation tests
    let turn2_elapsed = abort_start.elapsed();
    eprintln!(
        "[researcher] \n\
         [researcher] ================================================================================\n\
         [researcher] ASSISTANT RESPONSE (complete)\n\
         [researcher] ================================================================================\n\
         [researcher] {}\n\
         [researcher] \n\
         [researcher] ================================================================================\n\
         [researcher] TURN COMPLETED ({:.1}s)\n\
         [researcher] ================================================================================\n",
        if response2.is_empty() { "[empty response]" } else { &response2 },
        turn2_elapsed.as_secs_f64()
    );

    check!(
        !response2.is_empty(),
        "conversation should still work after abort — second turn returned empty response. Events: {:?}",
        turn2_events
            .iter()
            .map(|(t, d)| {
                let s = d.to_string();
                format!("{}:{}", t, &s[..s.floor_char_boundary(200.min(s.len()))])
            })
            .collect::<Vec<_>>()
    );

    step!(
        "PASS — abort worked (latency {:?}) and conversation remained usable",
        abort_latency
    );
}
