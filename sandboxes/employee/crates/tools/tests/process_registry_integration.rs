use std::collections::HashMap;
use std::time::{Duration, Instant};
use tools::ProcessRegistry;

// SCENARIO: Agent starts a long npm install in background.
// Registers the process and can poll its status for output.
#[tokio::test]
async fn agent_spawns_background_process_and_polls_status() {
    let registry = ProcessRegistry::new();
    let pid = registry.spawn("sleep 2 && echo done", HashMap::new(), 5);
    assert!(
        pid.starts_with("bash-"),
        "process ID should start with bash-"
    );

    let status = registry.status(&pid).unwrap();
    assert!(
        status.running,
        "long-running process should still be running"
    );
    assert_eq!(
        status.exit_code, None,
        "running process should not have an exit code yet"
    );
}

// SCENARIO: Agent's background build completes.
// Status shows exit_code 0 and captures the output.
#[tokio::test]
async fn completed_background_process_shows_exit_code_and_output() {
    let registry = ProcessRegistry::new();
    let pid = registry.spawn("echo 'build complete: 42 tests passed'", HashMap::new(), 5);

    let (exit_code, output) = wait_until_finished(&registry, &pid).await;
    assert_eq!(exit_code, Some(0));
    assert!(
        output.contains("build complete"),
        "output should contain the message"
    );
}

// SCENARIO: Agent tries to check a process that never existed.
// Returns None - the process may have expired or was never created.
#[tokio::test]
async fn checking_nonexistent_process_returns_none() {
    let registry = ProcessRegistry::new();
    let result = registry.status("bash-nonexistent-12345");
    assert!(result.is_none(), "nonexistent process should return None");
}

// SCENARIO: Agent spawns multiple background tasks simultaneously.
// Each gets a unique process ID, all can be polled independently.
#[tokio::test]
async fn multiple_background_processes_get_unique_ids() {
    let registry = ProcessRegistry::new();
    let pid1 = registry.spawn("sleep 1", HashMap::new(), 5);
    tokio::time::sleep(std::time::Duration::from_millis(2)).await;
    let pid2 = registry.spawn("echo task2", HashMap::new(), 5);
    tokio::time::sleep(std::time::Duration::from_millis(2)).await;
    let pid3 = registry.spawn("echo task3", HashMap::new(), 5);

    assert_ne!(pid1, pid2, "each process must have unique ID");
    assert_ne!(pid2, pid3, "each process must have unique ID");
    assert_ne!(pid1, pid3, "each process must have unique ID");

    // All three should be findable
    assert!(registry.status(&pid1).is_some());
    assert!(registry.status(&pid2).is_some());
    assert!(registry.status(&pid3).is_some());
}

// SCENARIO: Agent spawns a command that fails immediately.
// Status shows the error output and non-zero exit code.
#[tokio::test]
async fn failed_command_shows_error_in_status() {
    let registry = ProcessRegistry::new();
    let pid = registry.spawn("nonexistent_command_xyz 2>&1", HashMap::new(), 5);

    let (exit_code, output) = wait_until_finished(&registry, &pid).await;
    assert!(
        matches!(exit_code, Some(code) if code != 0),
        "failed command should have non-zero exit code, got {:?}",
        exit_code
    );
    assert!(
        output.contains("nonexistent_command_xyz") || output.contains("not found"),
        "failed command should capture shell error output, got {:?}",
        output
    );
}

async fn wait_until_finished(registry: &ProcessRegistry, pid: &str) -> (Option<i32>, String) {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let status = registry.status(pid).expect("process should exist");
        if !status.running {
            return (status.exit_code, status.output);
        }
        assert!(
            Instant::now() < deadline,
            "process {pid} did not finish before deadline"
        );
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
}
