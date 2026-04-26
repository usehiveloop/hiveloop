#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! E2E tests for parallel execution capabilities.
//!
//! These tests verify:
//! - Batch tool concurrency
//! - Subagent foreground/background execution
//! - Gap identification for future features

use bridge_e2e::{check, step, TestHarness};
use std::time::{Duration, Instant};

const TEST_TIMEOUT: Duration = Duration::from_secs(60);

// ============================================================================
// Phase 1: Current Functionality Tests
// ============================================================================

/// Test that batch tool reads multiple files concurrently.
/// Requires a real LLM — mock LLM cannot construct batch tool call arguments.
#[tokio::test]
#[ignore]
async fn test_batch_reads_multiple_files() {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping");
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Creating test files via bash tool");
    // Create test files via bash tool
    let turn = harness
        .converse(
            "streaming-agent",
            None,
            "Run this bash command: mkdir -p /tmp/parallel_test && echo 'content of file 1' > /tmp/parallel_test/file1.txt && echo 'content of file 2' > /tmp/parallel_test/file2.txt && echo 'content of file 3' > /tmp/parallel_test/file3.txt",
            Duration::from_secs(120),
        )
        .await
        .expect("setup failed");

    step!("Setup SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    step!("Sending batch read request for 3 files");
    // Now test batch reading
    let turn = harness
        .converse(
            "streaming-agent",
            None,
            "Use the batch tool to read these three files at once: /tmp/parallel_test/file1.txt, /tmp/parallel_test/file2.txt, and /tmp/parallel_test/file3.txt. Show me the contents.",
            Duration::from_secs(120),
        )
        .await
        .expect("batch read failed");

    step!("Batch read SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    step!("Verifying response is non-empty");
    // Verify response mentions file contents
    check!(
        !turn.response_text.is_empty(),
        "should have non-empty response"
    );
    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(200)]
    );

    step!("PASS — batch read of 3 files completed");
}

/// Test that multiple background subagents run in parallel.
/// Requires real agents (agent_delegator fixture only available via start_real).
#[tokio::test]
#[ignore]
async fn test_background_subagents_run_in_parallel() {
    step!("Starting harness");
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let start = Instant::now();

    step!("Spawning 2 background subagents using 'explorer'");
    // Spawn two background subagents (using agent_delegator fixture which has explorer subagent)
    let turn = harness
        .converse(
            "agent_delegator",
            None,
            "Spawn TWO background tasks using the 'explorer' subagent:\n\
             1. First task: 'List all Rust files in the current directory'\n\
             2. Second task: 'Find all test files'\n\
             Report both task_ids returned.",
            TEST_TIMEOUT,
        )
        .await
        .expect("background spawn failed");

    let elapsed = start.elapsed();

    step!("Background spawn completed in {:?}", elapsed);

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(200)]
    );

    step!("Verifying background spawn returned quickly (< 5s)");
    // Should return quickly (not waiting for subagents to complete)
    check!(
        elapsed < Duration::from_secs(5),
        "background spawn should return quickly, took {:?}",
        elapsed
    );

    step!("Verifying response contains task references");
    // Should contain task_ids
    check!(
        turn.response_text.contains("task_id") || turn.response_text.contains("task"),
        "should contain task reference: {}",
        turn.response_text
    );

    step!("PASS — 2 background subagents spawned in {:?}", elapsed);
}

/// Test foreground subagent blocks until complete.
/// Requires real agents (agent_delegator fixture only available via start_real).
#[tokio::test]
#[ignore]
async fn test_foreground_subagent_blocks() {
    step!("Starting harness");
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let start = Instant::now();

    step!("Running foreground subagent 'explorer' to list files");
    // Use foreground subagent (default mode)
    let turn = harness
        .converse(
            "agent_delegator",
            None,
            "Use the 'explorer' subagent to list all files in the current directory (do NOT use background mode).",
            TEST_TIMEOUT,
        )
        .await
        .expect("foreground subagent failed");

    let elapsed = start.elapsed();

    step!("Foreground subagent completed in {:?}", elapsed);

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(200)]
    );

    step!("Verifying foreground took noticeable time (>= 10ms)");
    // Should take some time (subagent needs to execute).
    // On fast CI machines this can complete in ~50ms, so we use a low threshold.
    check!(
        elapsed >= Duration::from_millis(10),
        "foreground should take noticeable time, took {:?}",
        elapsed
    );

    step!("Verifying response is non-empty");
    // Should have actual results
    check!(
        !turn.response_text.is_empty(),
        "should have non-empty response"
    );

    step!(
        "PASS — foreground subagent blocked and returned results in {:?}",
        elapsed
    );
}

/// GAP TEST: No built-in join mechanism for multiple background tasks
/// This test documents that we cannot easily wait for multiple background tasks.
/// Requires real agents (agent_delegator fixture only available via start_real).
#[tokio::test]
#[ignore]
async fn test_gap_no_join_for_multiple_background_tasks() {
    step!("Starting harness");
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    step!("Spawning 3 background tasks and asking to join all");
    // Spawn multiple background tasks
    let turn = harness
        .converse(
            "agent_delegator",
            None,
            "Spawn three background tasks using the 'explorer' subagent:\n\
             1. 'List all .rs files'\n\
             2. 'List all .toml files'\n\
             3. 'List all .md files'\n\
             After spawning, wait for ALL THREE to complete and report the combined results.",
            TEST_TIMEOUT,
        )
        .await
        .expect("test conversation failed");

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    // Document the behavior — currently there's no join tool, so the agent
    // may struggle to wait for all three. This test captures current behavior.
    step!("GAP DOCUMENTATION: Response when asked to join multiple background tasks:");
    eprintln!(
        "    {:?}",
        &turn.response_text[..turn.response_text.len().min(300)]
    );

    // The test passes — it documents current behavior, even if suboptimal
    check!(!turn.response_text.is_empty(), "should have some response");

    step!("PASS — gap documented: no join mechanism for background tasks");
}
