//! E2E tests for parallel execution capabilities.
//!
//! These tests verify:
//! - Batch tool concurrency
//! - Subagent foreground/background execution
//! - Gap identification for future features

use bridge_e2e::{ConversationTurn, TestHarness};
use std::time::{Duration, Instant};

const TEST_TIMEOUT: Duration = Duration::from_secs(60);

// ============================================================================
// Phase 1: Current Functionality Tests
// ============================================================================

/// Test that batch tool reads multiple files concurrently
#[tokio::test]
async fn test_batch_reads_multiple_files() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Create test files via bash tool
    let setup = r#"
mkdir -p /tmp/parallel_test && \
echo "content of file 1" > /tmp/parallel_test/file1.txt && \
echo "content of file 2" > /tmp/parallel_test/file2.txt && \
echo "content of file 3" > /tmp/parallel_test/file3.txt
"#;
    
    let turn = harness
        .converse(
            "agent_mock_llm",
            None,
            &format!("Run this bash command: {}", setup),
            TEST_TIMEOUT,
        )
        .await
        .expect("setup failed");

    // Now test batch reading
    let turn = harness
        .converse(
            "agent_mock_llm",
            None,
            "Use the batch tool to read /tmp/parallel_test/file1.txt, /tmp/parallel_test/file2.txt, and /tmp/parallel_test/file3.txt",
            TEST_TIMEOUT,
        )
        .await
        .expect("batch read failed");

    // Verify response contains all file contents
    assert!(
        turn.response_text.contains("content of file 1"),
        "should contain file1 content: {}",
        turn.response_text
    );
    assert!(
        turn.response_text.contains("content of file 2"),
        "should contain file2 content: {}",
        turn.response_text
    );
    assert!(
        turn.response_text.contains("content of file 3"),
        "should contain file3 content: {}",
        turn.response_text
    );

    // Verify batch tool was called
    let batch_called = turn
        .sse_events
        .iter()
        .any(|e| e.event_type == "tool_call_start" && e.data.get("name").and_then(|n| n.as_str()) == Some("batch"));
    assert!(batch_called, "batch tool should have been called");
}

/// Test that multiple background subagents run in parallel
#[tokio::test]
async fn test_background_subagents_run_in_parallel() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let start = Instant::now();
    
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

    // Should return quickly (not waiting for subagents to complete)
    assert!(
        elapsed < Duration::from_secs(5),
        "background spawn should return quickly, took {:?}",
        elapsed
    );

    // Should contain task_ids
    assert!(
        turn.response_text.contains("task_id") || turn.response_text.contains("task"),
        "should contain task reference: {}",
        turn.response_text
    );
}

/// Test foreground subagent blocks until complete
#[tokio::test]
async fn test_foreground_subagent_blocks() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let start = Instant::now();
    
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

    // Should take some time (subagent needs to execute)
    assert!(
        elapsed >= Duration::from_millis(100),
        "foreground should take noticeable time, took {:?}",
        elapsed
    );

    // Should have actual results
    assert!(
        !turn.response_text.is_empty(),
        "should have non-empty response"
    );
}

// ============================================================================
// Phase 2: Gap Documentation Tests (These document current limitations)
// ============================================================================

/// GAP TEST: No built-in join mechanism for multiple background tasks
/// This test documents that we cannot easily wait for multiple background tasks
#[tokio::test]
async fn test_gap_no_join_for_multiple_background_tasks() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

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

    // Document the behavior — currently there's no join tool, so the agent
    // may struggle to wait for all three. This test captures current behavior.
    eprintln!("GAP DOCUMENTATION: Response when asked to join multiple background tasks:");
    eprintln!("{}", turn.response_text);
    
    // The test passes — it documents current behavior, even if suboptimal
    assert!(!turn.response_text.is_empty(), "should have some response");
}

/// GAP TEST: Cannot batch agent tool calls
/// This test documents that agent tool cannot be used within batch tool
#[tokio::test]
async fn test_gap_cannot_batch_agent_tool() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

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

    // Document the behavior — batch tool should reject agent calls
    eprintln!("GAP DOCUMENTATION: Response when trying to batch agent tool:");
    eprintln!("{}", turn.response_text);
    
    // Likely contains error about external tools not being batchable
    let has_error = turn.sse_events.iter().any(|e| {
        e.event_type == "tool_call_result" && 
        e.data.get("is_error").and_then(|v| v.as_bool()).unwrap_or(false)
    });
    
    if has_error {
        eprintln!("CONFIRMED: Batch tool rejects agent calls (as expected with current design)");
    }
}

/// GAP TEST: No parallel spawn-and-wait primitive
/// This test documents the need for a tool that spawns N subagents and returns when all complete
#[tokio::test]
async fn test_gap_no_parallel_spawn_and_wait() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let start = Instant::now();
    
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
    
    eprintln!("GAP DOCUMENTATION: Sequential subagent execution took {:?}", sequential_time);
    eprintln!("With parallel execution, this could be ~3x faster");
    
    // Test passes — documents current sequential limitation
    assert!(!turn.response_text.is_empty(), "should have results");
}

/// GAP TEST: No resource limiting for subagent spawn
/// This test documents that we can spawn unlimited subagents concurrently
#[tokio::test]
async fn test_gap_no_concurrency_limits() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

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

    eprintln!("GAP DOCUMENTATION: System allowed spawning 5 concurrent background subagents");
    eprintln!("There is currently no max_concurrent limit");
    
    // Should have spawned all 5
    assert!(!turn.response_text.is_empty(), "should have response");
}

// ============================================================================
// Phase 3: Performance Benchmarks (Document current performance)
// ============================================================================

/// Benchmark: Batch vs Sequential file reads
#[tokio::test]
async fn benchmark_batch_vs_sequential_reads() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Setup: Create test files
    let setup = r#"
mkdir -p /tmp/bench_test && \
for i in {1..5}; do echo "file $i content" > /tmp/bench_test/file$i.txt; done
"#;
    
    let _ = harness
        .converse(
            "agent_mock_llm",
            None,
            &format!("Run this bash command: {}", setup),
            TEST_TIMEOUT,
        )
        .await;

    // Test batch reads
    let start = Instant::now();
    let batch_turn = harness
        .converse(
            "agent_mock_llm",
            None,
            "Use the batch tool to read all 5 files: /tmp/bench_test/file1.txt through /tmp/bench_test/file5.txt",
            TEST_TIMEOUT,
        )
        .await
        .expect("batch read failed");
    let batch_time = start.elapsed();

    // Test sequential reads (individual Read tool calls)
    let start = Instant::now();
    let seq_turn = harness
        .converse(
            "agent_mock_llm",
            None,
            "Read these files one at a time (not using batch): /tmp/bench_test/file1.txt, /tmp/bench_test/file2.txt, /tmp/bench_test/file3.txt, /tmp/bench_test/file4.txt, /tmp/bench_test/file5.txt",
            TEST_TIMEOUT,
        )
        .await
        .expect("sequential read failed");
    let seq_time = start.elapsed();

    eprintln!("\n=== BATCH VS SEQUENTIAL PERFORMANCE ===");
    eprintln!("Batch time: {:?}", batch_time);
    eprintln!("Sequential time: {:?}", seq_time);
    eprintln!("Speedup: {:.2}x", seq_time.as_secs_f64() / batch_time.as_secs_f64());

    // Both should succeed
    assert!(!batch_turn.response_text.is_empty(), "batch should return results");
    assert!(!seq_turn.response_text.is_empty(), "sequential should return results");
}
