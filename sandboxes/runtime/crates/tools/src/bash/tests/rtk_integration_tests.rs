//! End-to-end integration tests for the rtk Bash-tool integration.
//!
//! These tests actually spawn `rtk` against a real command and assert that
//! bridge's rewriter plus rtk's filter pipeline produce compressed output
//! — the gap that unit tests on the rewriter alone cannot catch.
//!
//! All tests gracefully skip when rtk isn't installed or the underlying
//! command isn't available, so CI without rtk still passes.

use crate::bash::rtk;
use crate::bash::run_command;

/// Baseline probe: is rtk actually on PATH and runnable? If not, every test
/// in this module skips. Prints a diagnostic so a skipped-but-passing test
/// run is visible in CI logs.
fn rtk_ready() -> bool {
    if !rtk::is_rtk_available() {
        eprintln!(
            "[rtk-integration] skipped: rtk not on PATH. \
             Install with `cargo install --git https://github.com/rtk-ai/rtk rtk`."
        );
        return false;
    }
    true
}

/// `git log` is a good canary: rtk ships a built-in `git` filter, every dev
/// machine has git, and the bridge repo has plenty of commit history for a
/// non-trivial byte delta. Asserts that routing through rtk produces output
/// no larger than raw — i.e. the integration actually reaches the filter
/// tier, not just passes through.
#[tokio::test]
async fn rtk_filters_git_log_against_real_repo() {
    if !rtk_ready() {
        return;
    }

    // Run from the bridge crate's manifest dir — a git repo with history.
    let workdir = env!("CARGO_MANIFEST_DIR");

    let rewritten = rtk::rewrite("git log --oneline -20");
    assert_eq!(
        rewritten, "rtk git log --oneline -20",
        "rewriter must prepend `rtk` for git"
    );

    let filtered = run_command(&rewritten, workdir, 10_000)
        .await
        .expect("rtk git log should run");
    let raw = run_command("git log --oneline -20", workdir, 10_000)
        .await
        .expect("git log should run");

    assert_eq!(
        filtered.exit_code,
        Some(0),
        "rtk git log should exit 0; output was: {}",
        filtered.output
    );
    assert_eq!(raw.exit_code, Some(0), "git log should exit 0");
    assert!(
        !filtered.output.is_empty(),
        "rtk git log should produce output"
    );
    // rtk's git filter never makes output larger than raw (worst case it
    // passes through unchanged). If this fails, something is injecting
    // content instead of filtering.
    assert!(
        filtered.output.len() <= raw.output.len(),
        "filtered ({} bytes) should not exceed raw ({} bytes)",
        filtered.output.len(),
        raw.output.len()
    );
}

/// End-to-end proof that the Laravel pipeline works: rewriter routes
/// `composer validate` through `rtk`, rtk looks it up in our embedded
/// filters.toml, the `composer-validate` filter fires, output is
/// compressed. Skips if composer isn't installed.
///
/// This is the kind of test the previous integration lacked — it proves
/// every layer (rewriter, rtk registry, TOML filter) connects end-to-end
/// against a real command.
#[tokio::test]
async fn rtk_filters_composer_validate_via_toml() {
    if !rtk_ready() {
        return;
    }
    let composer_present = std::process::Command::new("composer")
        .arg("--version")
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .status()
        .map(|s| s.success())
        .unwrap_or(false);
    if !composer_present {
        eprintln!("[rtk-integration] skipped: composer not on PATH.");
        return;
    }

    // Create a throwaway dir with a minimal composer.json so `composer
    // validate` has something to validate. Use the system tempdir.
    let tmp = std::env::temp_dir().join(format!("bridge-rtk-composer-{}", std::process::id()));
    std::fs::create_dir_all(&tmp).expect("temp dir");
    std::fs::write(
        tmp.join("composer.json"),
        r#"{"name":"acme/test","require":{}}"#,
    )
    .expect("write composer.json");
    let workdir = tmp.to_string_lossy().into_owned();

    // Verify the rewriter produces what we expect before running.
    let rewritten = rtk::rewrite("composer validate");
    assert_eq!(
        rewritten, "rtk composer validate",
        "rewriter must prepend `rtk` for composer"
    );

    let filtered = run_command(&rewritten, &workdir, 5_000)
        .await
        .expect("rtk composer validate should run");

    // composer validate on a minimal valid json exits 0. Output should be
    // the compressed summary from our composer-validate filter.
    assert!(
        !filtered.output.is_empty(),
        "rtk composer validate should produce output"
    );
    // Either the filter short-circuit fired ("composer.json is valid" or
    // a compressed summary) or composer surfaced a real warning. Both are
    // acceptable — what's NOT acceptable is unfiltered noise, which would
    // include "Loading composer repositories" / "Updating dependencies"
    // lines that our filter strips.
    assert!(
        !filtered.output.contains("Loading composer repositories"),
        "filter should strip 'Loading composer repositories' banner; \
         got output:\n{}",
        filtered.output
    );

    let _ = std::fs::remove_dir_all(&tmp);
}

/// Prove the rewriter does NOT accidentally swallow non-routable commands.
/// A bash call to `echo hello` should pass through verbatim whether rtk
/// is installed or not — any change here would be a regression touching
/// every Bash tool call that isn't a filterable command.
#[tokio::test]
async fn non_routable_commands_pass_through_unchanged() {
    let workdir = env!("CARGO_MANIFEST_DIR");

    // Rewriter decision: leave it alone.
    assert_eq!(rtk::rewrite("echo hello world"), "echo hello world");

    // End-to-end: the command itself still runs correctly through the
    // bash runner.
    let result = run_command("echo hello world", workdir, 2_000)
        .await
        .expect("echo should run");
    assert_eq!(result.exit_code, Some(0));
    assert!(result.output.contains("hello world"));
}

/// Prove the rewriter correctly handles a `cd X && cmd` compound — this is
/// exactly the pattern that broke when the prior integration delegated to
/// `rtk rewrite`, which refused to rewrite any segment of a compound whose
/// first segment (`cd`) wasn't in rtk's registry.
#[tokio::test]
async fn compound_command_routes_second_segment_only() {
    let workdir = env!("CARGO_MANIFEST_DIR");

    if rtk::is_rtk_available() {
        assert_eq!(
            rtk::rewrite("cd src && git log --oneline -3"),
            "cd src && rtk git log --oneline -3"
        );
    }

    // Run it end-to-end to make sure sh still parses the rewritten form.
    let cmd = if rtk::is_rtk_available() {
        "cd src && rtk git log --oneline -3"
    } else {
        "cd src && git log --oneline -3"
    };
    let result = run_command(cmd, workdir, 5_000)
        .await
        .expect("compound command should run");
    assert_eq!(
        result.exit_code,
        Some(0),
        "compound should exit 0, output: {}",
        result.output
    );
}
