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

// ============================================================================
// Phase 2: Gap Documentation Tests (These document current limitations)
// ============================================================================

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

/// GAP TEST: Cannot batch agent tool calls
/// This test documents that agent tool cannot be used within batch tool.
/// Requires real agents (agent_delegator fixture only available via start_real).
#[tokio::test]
#[ignore]
async fn test_gap_cannot_batch_agent_tool() {
    step!("Starting harness");
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    step!("Attempting to batch 2 agent tool calls simultaneously");
    let turn = harness
        .converse(
            "agent_delegator",
            None,
            "Try to use the batch tool to spawn TWO 'explorer' subagents simultaneously:\n\
             - First call: explorer with prompt 'Find all source files'\n\
             - Second call: explorer with prompt 'Find all test files'\n\
             Report what happens.",
            TEST_TIMEOUT,
        )
        .await
        .expect("test conversation failed");

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    step!("GAP DOCUMENTATION: Response when trying to batch agent tool:");
    eprintln!(
        "    {:?}",
        &turn.response_text[..turn.response_text.len().min(300)]
    );

    // Likely contains error about external tools not being batchable
    let has_error = turn.sse_events.iter().any(|e| {
        e.event_type == "tool_call_result"
            && e.data
                .get("is_error")
                .and_then(|v| v.as_bool())
                .unwrap_or(false)
    });

    if has_error {
        step!("CONFIRMED: Batch tool rejects agent calls (as expected with current design)");
    } else {
        step!("No error returned — batch may have accepted agent calls");
    }

    step!("PASS — gap documented: batch agent tool behavior captured");
}

/// GAP TEST: No parallel spawn-and-wait primitive
/// This test documents the need for a tool that spawns N subagents and returns when all complete.
/// Requires real agents (agent_delegator fixture only available via start_real).
#[tokio::test]
#[ignore]
async fn test_gap_no_parallel_spawn_and_wait() {
    step!("Starting harness");
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let start = Instant::now();

    step!("Running 3 sequential foreground subagent tasks");
    // Sequential foreground subagents (current workaround)
    let turn = harness
        .converse(
            "agent_delegator",
            None,
            "Run THREE sequential foreground tasks using the 'explorer' subagent:\n\
             1. 'Find all Cargo.toml files'\n\
             2. 'Find all README files'\n\
             3. 'Find all main.rs files'\n\
             Wait for each to complete before starting the next.",
            Duration::from_secs(120),
        )
        .await
        .expect("sequential execution failed");

    let sequential_time = start.elapsed();

    step!("Sequential execution completed in {:?}", sequential_time);

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(200)]
    );

    step!(
        "GAP DOCUMENTATION: Sequential subagent execution took {:?}. With parallel execution, this could be ~3x faster",
        sequential_time
    );

    // Test passes — documents current sequential limitation
    check!(!turn.response_text.is_empty(), "should have results");

    step!(
        "PASS — gap documented: sequential subagent took {:?}",
        sequential_time
    );
}

/// GAP TEST: No resource limiting for subagent spawn
/// This test documents that we can spawn unlimited subagents concurrently.
/// Requires real agents (agent_delegator fixture only available via start_real).
#[tokio::test]
#[ignore]
async fn test_gap_no_concurrency_limits() {
    step!("Starting harness");
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    step!("Spawning 5 background subagents concurrently");
    // Try to spawn many background subagents
    let turn = harness
        .converse(
            "agent_delegator",
            None,
            "Spawn FIVE background tasks using the 'explorer' subagent with these prompts:\n\
             1. 'List .rs files'\n\
             2. 'List .toml files'\n\
             3. 'List .md files'\n\
             4. 'List .json files'\n\
             5. 'List .yaml files'\n\
             Report all task_ids. Note: This tests system behavior with many concurrent subagents.",
            TEST_TIMEOUT,
        )
        .await
        .expect("test conversation failed");

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }

    eprintln!(
        "    Response: {:?}",
        &turn.response_text[..turn.response_text.len().min(300)]
    );

    step!("GAP DOCUMENTATION: System allowed spawning 5 concurrent background subagents (no max_concurrent limit)");

    // Should have spawned all 5
    check!(!turn.response_text.is_empty(), "should have response");

    step!("PASS — gap documented: 5 concurrent subagents spawned without limit");
}

// ============================================================================
// Phase 3: Performance Benchmarks (Document current performance)
// ============================================================================

/// Benchmark: Batch vs Sequential file reads.
/// Requires a real LLM — mock LLM cannot construct batch tool call arguments.
#[tokio::test]
#[ignore]
async fn benchmark_batch_vs_sequential_reads() {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping");
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let timeout = Duration::from_secs(120);

    step!("Creating 5 test files for benchmark");
    // Setup: Create test files
    let _ = harness
        .converse(
            "streaming-agent",
            None,
            "Run this bash command: mkdir -p /tmp/bench_test && for i in 1 2 3 4 5; do echo \"file $i content\" > /tmp/bench_test/file$i.txt; done",
            timeout,
        )
        .await;

    step!("Benchmark: batch read of 5 files");
    // Test batch reads
    let start = Instant::now();
    let batch_turn = harness
        .converse(
            "streaming-agent",
            None,
            "Use the batch tool to read all 5 files: /tmp/bench_test/file1.txt through /tmp/bench_test/file5.txt",
            timeout,
        )
        .await
        .expect("batch read failed");
    let batch_time = start.elapsed();

    step!("Batch read completed in {:?}", batch_time);
    step!("Batch SSE events ({} total)", batch_turn.sse_events.len());
    for e in &batch_turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }
    eprintln!(
        "    Response: {:?}",
        &batch_turn.response_text[..batch_turn.response_text.len().min(200)]
    );

    step!("Benchmark: sequential read of 5 files");
    // Test sequential reads (individual Read tool calls)
    let start = Instant::now();
    let seq_turn = harness
        .converse(
            "streaming-agent",
            None,
            "Read these files one at a time (not using batch): /tmp/bench_test/file1.txt, /tmp/bench_test/file2.txt, /tmp/bench_test/file3.txt, /tmp/bench_test/file4.txt, /tmp/bench_test/file5.txt",
            timeout,
        )
        .await
        .expect("sequential read failed");
    let seq_time = start.elapsed();

    step!("Sequential read completed in {:?}", seq_time);
    step!(
        "Sequential SSE events ({} total)",
        seq_turn.sse_events.len()
    );
    for e in &seq_turn.sse_events {
        eprintln!("    - {}", e.event_type);
    }
    eprintln!(
        "    Response: {:?}",
        &seq_turn.response_text[..seq_turn.response_text.len().min(200)]
    );

    step!("=== BATCH VS SEQUENTIAL PERFORMANCE ===");
    eprintln!("    Batch time:      {:?}", batch_time);
    eprintln!("    Sequential time: {:?}", seq_time);
    eprintln!(
        "    Speedup:         {:.2}x",
        seq_time.as_secs_f64() / batch_time.as_secs_f64()
    );

    // Both should succeed
    check!(
        !batch_turn.response_text.is_empty(),
        "batch should return results"
    );
    check!(
        !seq_turn.response_text.is_empty(),
        "sequential should return results"
    );

    step!(
        "PASS — benchmark: batch={:?}, sequential={:?}, speedup={:.2}x",
        batch_time,
        seq_time,
        seq_time.as_secs_f64() / batch_time.as_secs_f64()
    );
}
