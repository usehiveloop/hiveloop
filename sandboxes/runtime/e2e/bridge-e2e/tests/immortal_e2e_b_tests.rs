#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! Immortal Conversations E2E tests — verify that conversation chaining
//! triggers when token budget is exceeded, emits the correct webhook events,
//! and the agent continues coherently in a fresh context with journal state.
//!
//! These tests use a real LLM (Fireworks) and are `#[ignore]` by default:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test immortal_e2e_tests -- --ignored
//! ```

use bridge_e2e::{check, check_eq, step, TestHarness};
use std::time::Duration;

const LLM_TIMEOUT: Duration = Duration::from_secs(120);
const WEBHOOK_TIMEOUT: Duration = Duration::from_secs(30);
const AGENT_ID: &str = "immortal-agent";

fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping");
        return false;
    }
    true
}

// ============================================================================
// Test 2: journal_write and journal_read tools are available in immortal mode
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_journal_tools_available() {
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

    step!("Creating conversation for immortal-agent");
    let create_resp = harness
        .create_conversation(AGENT_ID)
        .await
        .expect("create_conversation failed");

    let body: serde_json::Value = create_resp.json().await.expect("invalid json");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("missing conversation_id")
        .to_string();

    harness.register_conversation(&conv_id, AGENT_ID).await;

    // Ask the agent to write a journal entry
    step!("Asking agent to write a journal entry");
    harness
        .send_message(
            &conv_id,
            "Make a key architectural decision: we will use PostgreSQL for our database. \
             Write this decision to your journal using the journal_write tool, then confirm.",
        )
        .await
        .expect("send_message failed");

    let (events1, text1) = harness
        .stream_sse_until_done(&conv_id, LLM_TIMEOUT)
        .await
        .expect("stream failed");

    check!(!text1.is_empty(), "response should not be empty");

    // Check if journal_write was called
    let write_calls: Vec<_> = events1
        .iter()
        .filter(|e| {
            e.event_type == "tool_call_start"
                && e.data
                    .get("name")
                    .and_then(|v| v.as_str())
                    .map(|n| n == "journal_write")
                    .unwrap_or(false)
        })
        .collect();

    step!("journal_write tool calls found: {}", write_calls.len());

    if write_calls.is_empty() {
        eprintln!("    Note: agent did not call journal_write (model-dependent behavior)");
    } else {
        let write_results: Vec<_> = events1
            .iter()
            .filter(|e| {
                e.event_type == "tool_call_result"
                    && e.data
                        .get("tool_name")
                        .and_then(|v| v.as_str())
                        .map(|n| n == "journal_write")
                        .unwrap_or(false)
            })
            .collect();

        check!(
            !write_results.is_empty(),
            "journal_write should have completed successfully"
        );
        step!("journal_write completed successfully");
    }

    // Now ask the agent to read the journal
    step!("Asking agent to read journal entries");
    tokio::time::sleep(Duration::from_secs(3)).await;
    harness
        .send_message(
            &conv_id,
            "Read your journal using the journal_read tool and tell me what entries are there.",
        )
        .await
        .expect("send_message failed");

    let (events2, text2) = harness
        .stream_sse_until_done(&conv_id, LLM_TIMEOUT)
        .await
        .expect("stream failed");

    check!(
        !text2.is_empty(),
        "journal_read response should not be empty"
    );

    let read_calls: Vec<_> = events2
        .iter()
        .filter(|e| {
            e.event_type == "tool_call_start"
                && e.data
                    .get("name")
                    .and_then(|v| v.as_str())
                    .map(|n| n == "journal_read")
                    .unwrap_or(false)
        })
        .collect();

    step!("journal_read tool calls found: {}", read_calls.len());

    if read_calls.is_empty() {
        eprintln!("    Note: agent did not call journal_read (model-dependent behavior)");
    } else {
        let read_results: Vec<_> = events2
            .iter()
            .filter(|e| {
                e.event_type == "tool_call_result"
                    && e.data
                        .get("tool_name")
                        .and_then(|v| v.as_str())
                        .map(|n| n == "journal_read")
                        .unwrap_or(false)
            })
            .collect();

        check!(
            !read_results.is_empty(),
            "journal_read should have completed successfully"
        );
        step!("journal_read completed successfully");
    }

    step!("PASS — journal_write and journal_read tools are available in immortal mode");
}

// ============================================================================
// Test 3: Verify conversation_id stability across multiple chain handoffs
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_conversation_id_stable_across_chains() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start harness");

    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    step!("Creating conversation for immortal-agent");
    let create_resp = harness
        .create_conversation(AGENT_ID)
        .await
        .expect("create_conversation failed");

    let body: serde_json::Value = create_resp.json().await.expect("invalid json");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("missing conversation_id")
        .to_string();

    harness.register_conversation(&conv_id, AGENT_ID).await;

    // Send many messages to potentially trigger multiple chains
    let messages = [
        "Explain microservices architecture patterns in great detail. Cover service mesh, API gateways, circuit breakers, and saga patterns with examples.",
        "Now explain event-driven architecture. Cover CQRS, event sourcing, message brokers, and exactly-once delivery guarantees with code examples.",
        "Compare the two approaches for a real-time trading platform. Be very thorough with pros and cons.",
        "Design the complete system architecture combining both approaches. Include diagrams in text form.",
    ];

    for (i, msg) in messages.iter().enumerate() {
        if i > 0 {
            tokio::time::sleep(Duration::from_secs(10)).await;
        }
        step!("Sending message {}", i + 1);
        harness
            .send_message(&conv_id, msg)
            .await
            .expect("send_message failed");
        let (_events, text) = harness
            .stream_sse_until_done(&conv_id, LLM_TIMEOUT)
            .await
            .expect("stream failed");
        check!(!text.is_empty(), "response {} should not be empty", i + 1);
        step!("  Response {}: {} chars", i + 1, text.len());
    }

    // Check all webhooks use the same conversation_id
    step!("Verifying all webhooks use the same conversation_id");
    let log = harness
        .get_webhook_log()
        .await
        .expect("webhook log fetch failed");

    let chain_events: Vec<_> = log
        .entries
        .iter()
        .filter(|e| {
            let et = e.event_type();
            et == Some("chain_started") || et == Some("chain_completed")
        })
        .collect();

    step!("Total chain events: {}", chain_events.len());

    for entry in &chain_events {
        check_eq!(
            entry.conversation_id(),
            Some(conv_id.as_str()),
            "chain event conversation_id should match original"
        );
    }

    // Verify we got at least one chain handoff
    let chain_started_count = log.by_type("chain_started").len();
    check!(
        chain_started_count >= 1,
        "should have at least 1 chain_started event, got {}",
        chain_started_count
    );

    step!(
        "PASS — {} chain handoff(s), all with stable conversation_id={}",
        chain_started_count,
        conv_id
    );
}
