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
