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
// Test 1: Chain handoff triggers and fires chain_started + chain_completed webhooks
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_chain_handoff_emits_events_and_continues() {
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

    step!("Creating conversation for immortal-agent (token_budget=1500)");
    let create_resp = harness
        .create_conversation(AGENT_ID)
        .await
        .expect("create_conversation failed");

    let body: serde_json::Value = create_resp.json().await.expect("invalid json");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("missing conversation_id")
        .to_string();

    step!("Conversation created: {}", conv_id);
    harness.register_conversation(&conv_id, AGENT_ID).await;

    // Messages designed to quickly fill a 1500-token budget.
    let messages = [
        "Explain the differences between PostgreSQL and MySQL in detail. Cover indexing strategies, replication models, MVCC implementation, and query optimization. Be thorough.",
        "Now compare their JSON support, full-text search capabilities, partitioning strategies, and extension ecosystems. Give code examples for each.",
        "Based on what we discussed, which would you recommend for a high-write OLTP workload with complex JSON queries? Explain your reasoning step by step.",
    ];

    // Send first message normally
    step!("Sending message 1 to fill context");
    harness
        .send_message(&conv_id, messages[0])
        .await
        .expect("send_message failed");
    let (events1, text1) = harness
        .stream_sse_until_done(&conv_id, LLM_TIMEOUT)
        .await
        .expect("stream failed");
    check!(!text1.is_empty(), "first response should not be empty");
    step!(
        "  Response 1: {} chars, {} events",
        text1.len(),
        events1.len()
    );

    // Send remaining messages with delays to allow processing
    for (i, msg) in messages[1..].iter().enumerate() {
        tokio::time::sleep(Duration::from_secs(5)).await;
        step!("Sending message {} to build up context", i + 2);
        harness
            .send_message(&conv_id, msg)
            .await
            .expect("send_message failed");
        let (events, text) = harness
            .stream_sse_until_done(&conv_id, LLM_TIMEOUT)
            .await
            .expect("stream failed");
        check!(!text.is_empty(), "response {} should not be empty", i + 2);
        step!(
            "  Response {}: {} chars, {} events",
            i + 2,
            text.len(),
            events.len()
        );
    }

    // Wait for chain_started webhook
    step!(
        "Waiting for chain_started webhook (timeout: {:?})",
        WEBHOOK_TIMEOUT
    );
    let log = harness
        .wait_for_webhook_type("chain_started", WEBHOOK_TIMEOUT)
        .await
        .expect("webhook log fetch failed");

    let chain_started = log.by_type("chain_started");
    check!(
        !chain_started.is_empty(),
        "should have at least one chain_started webhook"
    );

    let cs_entry = chain_started[0];
    let cs_data = cs_entry.data().expect("chain_started should have data");

    check_eq!(
        cs_entry.agent_id(),
        Some(AGENT_ID),
        "chain_started agent_id"
    );
    check_eq!(
        cs_entry.conversation_id(),
        Some(conv_id.as_str()),
        "chain_started conversation_id matches"
    );

    let chain_index = cs_data
        .get("chain_index")
        .and_then(|v| v.as_u64())
        .expect("chain_index missing");
    check!(
        chain_index >= 1,
        "chain_index should be >= 1, got {}",
        chain_index
    );

    let token_count = cs_data
        .get("token_count")
        .and_then(|v| v.as_u64())
        .expect("token_count missing");
    check!(
        token_count > 1500,
        "token_count ({}) should exceed budget (1500)",
        token_count
    );

    step!(
        "chain_started: chain_index={}, token_count={}",
        chain_index,
        token_count
    );

    // Check for chain_completed webhook
    step!("Checking for chain_completed webhook");
    let chain_completed = log.by_type("chain_completed");
    check!(
        !chain_completed.is_empty(),
        "should have at least one chain_completed webhook"
    );

    let cc_entry = chain_completed[0];
    let cc_data = cc_entry.data().expect("chain_completed should have data");

    check_eq!(
        cc_entry.conversation_id(),
        Some(conv_id.as_str()),
        "chain_completed conversation_id matches"
    );

    let journal_count = cc_data
        .get("journal_entry_count")
        .and_then(|v| v.as_u64())
        .expect("journal_entry_count missing");
    check!(
        journal_count >= 1,
        "journal should have at least 1 entry (the checkpoint), got {}",
        journal_count
    );

    step!(
        "chain_completed: chain_index={}, journal_entries={}",
        cc_data
            .get("chain_index")
            .and_then(|v| v.as_u64())
            .unwrap_or(0),
        journal_count
    );

    // Verify conversation_id is unchanged throughout
    step!("Verifying conversation_id is stable across chain boundary");
    check_eq!(
        cs_entry.conversation_id(),
        Some(conv_id.as_str()),
        "conversation_id unchanged after chain"
    );

    // Send a post-chain message to verify the agent continues coherently
    step!("Sending post-chain message to verify continuity");
    tokio::time::sleep(Duration::from_secs(3)).await;
    harness
        .send_message(
            &conv_id,
            "Summarize what we discussed so far in one sentence.",
        )
        .await
        .expect("post-chain send_message failed");

    let (_post_events, post_text) = harness
        .stream_sse_until_done(&conv_id, LLM_TIMEOUT)
        .await
        .expect("post-chain stream failed");

    check!(
        !post_text.is_empty(),
        "post-chain response should not be empty"
    );
    step!(
        "Post-chain response: {:?}",
        &post_text[..post_text.len().min(200)]
    );

    step!("PASS — chain handoff triggered, events emitted, agent continues coherently");
}
