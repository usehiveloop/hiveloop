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
// Test: Compaction — triggers compaction and fires webhook
// Verifies: conversation_compacted webhook with summary, token counts
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_compaction_triggers_and_fires_webhook() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Clearing webhook log");
    // Clear any prior webhooks
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    step!("Creating conversation for compaction-agent (token_budget=150)");
    // Create a conversation on the compaction agent (token_budget=150)
    let create_resp = harness
        .create_conversation("compaction-agent")
        .await
        .expect("create_conversation failed");

    check_eq!(
        create_resp.status().as_u16(),
        201,
        "create conversation returns 201"
    );

    let create_body: serde_json::Value = create_resp
        .json()
        .await
        .expect("failed to parse create body");
    let conv_id = create_body["conversation_id"]
        .as_str()
        .expect("missing conversation_id")
        .to_string();

    step!("Conversation created: {}", conv_id);

    // Register for conversation logging
    harness
        .register_conversation(&conv_id, "compaction-agent")
        .await;

    // Real-world messages that build up context quickly
    let messages: Vec<&str> = vec![
        "Draft a formal email to the VP of Engineering about the Q3 roadmap planning session scheduled for next Thursday",
        "Actually, also CC the product team leads and mention that we need their input on the resource allocation proposals",
        "Add a paragraph about the budget constraints we discussed in last week's leadership meeting",
        "Now draft a separate follow-up email to the design team about the UI refresh project timeline and deliverables",
        "One more thing — include a reminder about the Friday design review and ask them to prepare their mockups",
        "Thanks, that looks great. Send it off.",
    ];

    let total_messages = messages.len();

    step!("Sending {} messages to build up context", total_messages);
    // Send the first message so the conversation loop starts processing
    let msg_resp = harness
        .send_message(&conv_id, messages[0])
        .await
        .expect("send_message failed");
    check_eq!(
        msg_resp.status().as_u16(),
        202,
        "first message accepted (202)"
    );
    step!(
        "Sent message 1/{}: '{}'",
        total_messages,
        &messages[0][..messages[0].len().min(60)]
    );

    // Spawn a background task to send remaining messages after each turn completes.
    // The conversation loop processes one message at a time from a buffered channel,
    // so we queue them with small delays to let each turn start.
    let bridge_url = harness.bridge_url().to_string();
    let conv_id_bg = conv_id.clone();
    let remaining: Vec<String> = messages[1..].iter().map(|s| s.to_string()).collect();
    let remaining_count = remaining.len();
    tokio::spawn(async move {
        let client = reqwest::Client::new();
        for (i, msg) in remaining.iter().enumerate() {
            // Each Fireworks round-trip takes ~5-10s; with compaction it may
            // need two calls (summary + chat). Wait long enough for the
            // previous turn to fully complete before queuing the next message.
            tokio::time::sleep(Duration::from_secs(15)).await;
            eprintln!(
                "\n  \x1b[36m\u{25b8}\x1b[0m Sent message {}/{}: '{}'",
                i + 2,
                remaining_count + 1,
                &msg[..msg.len().min(60)]
            );
            let _ = client
                .post(format!(
                    "{}/conversations/{}/messages",
                    bridge_url, conv_id_bg
                ))
                .json(&serde_json::json!({"content": msg}))
                .send()
                .await;
        }
    });

    step!(
        "Streaming SSE events until all {} turns complete",
        total_messages
    );
    // Read the SSE stream until all turns complete (one `done` per turn)
    let (_events, _text) = harness
        .stream_sse_until_done_count(&conv_id, total_messages, LLM_TIMEOUT)
        .await
        .expect("stream_sse_until_done_count failed");

    step!("Waiting for conversation_compacted webhook (timeout: 30s)");
    // Wait for the conversation_compacted webhook
    let log = harness
        .wait_for_webhook_type("conversation_compacted", Duration::from_secs(30))
        .await
        .expect("conversation_compacted webhook never arrived");

    let compacted = log.by_type("conversation_compacted");
    check!(
        !compacted.is_empty(),
        "should have at least one conversation_compacted webhook"
    );

    step!("Checking conversation_compacted webhook payload");
    // Verify the webhook payload
    let entry = compacted[0];
    check_eq!(
        entry.agent_id(),
        Some("compaction-agent"),
        "webhook agent_id is compaction-agent"
    );
    check_eq!(
        entry.conversation_id(),
        Some(conv_id.as_str()),
        "webhook conversation_id matches"
    );

    let data = entry
        .data()
        .expect("conversation_compacted should have data");

    eprintln!(
        "    Webhook data: {}",
        serde_json::to_string_pretty(data).unwrap_or_default()
    );

    check!(
        data.get("summary")
            .and_then(|v| v.as_str())
            .map(|s| !s.is_empty())
            .unwrap_or(false),
        "summary should be a non-empty string; got: {:?}",
        data.get("summary")
    );
    check!(
        data.get("messages_compacted")
            .and_then(|v| v.as_u64())
            .unwrap_or(0)
            > 0,
        "messages_compacted should be > 0"
    );
    let pre = data
        .get("pre_compaction_tokens")
        .and_then(|v| v.as_u64())
        .expect("pre_compaction_tokens missing");
    let post = data
        .get("post_compaction_tokens")
        .and_then(|v| v.as_u64())
        .expect("post_compaction_tokens missing");
    check!(
        pre > 1500,
        "pre_compaction_tokens ({}) should exceed budget (1500)",
        pre
    );
    // After compacting many turns into a summary, post should be notably smaller
    check!(
        post < pre,
        "post_compaction_tokens ({}) should be less than pre ({})",
        post,
        pre
    );

    step!("Ending conversation");
    // End the conversation
    let end_resp = harness
        .end_conversation(&conv_id)
        .await
        .expect("end_conversation failed");
    check_eq!(
        end_resp.status().as_u16(),
        200,
        "end conversation returns 200"
    );

    step!(
        "PASS — compaction triggered: pre={} post={}, summary present",
        pre,
        post
    );
}
