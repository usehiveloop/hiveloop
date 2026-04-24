//! RTK (Rust Token Killer) integration for the Bash tool.
//!
//! Detects a locally-installed `rtk` binary and routes Bash tool invocations
//! through `rtk rewrite` so tools like `git`, `cargo`, `composer`, `php artisan`,
//! etc. emit compressed output. Also writes our embedded Laravel/PHP filter set
//! to rtk's user-global config directory at bridge startup, so filtering is
//! available in every project without per-project `rtk trust` calls.
//!
//! Global filter path (resolved via the `dirs` crate, matching rtk's own
//! lookup):
//!   - macOS:   `~/Library/Application Support/rtk/filters.toml`
//!   - Linux:   `~/.config/rtk/filters.toml` (or `$XDG_CONFIG_HOME/rtk/`)
//!
//! Escape hatch: setting `BRIDGE_DISABLE_RTK=1` disables both the bootstrap and
//! the per-call rewrite — useful for debugging or if an rtk update regresses.

use std::path::PathBuf;
use std::sync::OnceLock;
use std::time::Duration;

use anyhow::{Context, Result};
use tokio::io::AsyncReadExt;
use tracing::{debug, info, warn};

/// Embedded filter set, compiled into the binary. Source of truth lives at
/// `crates/tools/assets/laravel-filters.toml` — edit there, rebuild, redeploy.
const EMBEDDED_FILTERS: &str = include_str!("../../assets/laravel-filters.toml");

/// Header stamped onto the file rtk reads, so users looking at the global
/// filters file on disk know where it comes from and don't edit it in place.
const MANAGED_HEADER: &str = concat!(
    "# MANAGED BY portal.bridge — DO NOT EDIT HERE.\n",
    "# This file is written by the bridge on every startup from an embedded asset.\n",
    "# Source of truth: crates/tools/assets/laravel-filters.toml in the bridge repo.\n",
    "# Local edits here will be overwritten on the next bridge launch.\n",
    "\n",
);

/// Envvar that disables rtk integration entirely.
const DISABLE_ENV: &str = "BRIDGE_DISABLE_RTK";

/// Cached result of `which rtk` — populated on first call.
static RTK_AVAILABLE: OnceLock<bool> = OnceLock::new();

/// True if rtk is both installed (binary on PATH, `rtk --version` exits 0)
/// and not disabled via `BRIDGE_DISABLE_RTK=1`.
pub fn is_rtk_available() -> bool {
    if std::env::var(DISABLE_ENV).is_ok_and(|v| v == "1") {
        return false;
    }
    *RTK_AVAILABLE.get_or_init(|| {
        std::process::Command::new("rtk")
            .arg("--version")
            .stdin(std::process::Stdio::null())
            .stdout(std::process::Stdio::null())
            .stderr(std::process::Stdio::null())
            .status()
            .map(|s| s.success())
            .unwrap_or(false)
    })
}

/// Resolve the global filters.toml path that rtk will read.
///
/// rtk uses `dirs::config_dir()` internally (`~/Library/Application Support/`
/// on macOS, `~/.config/` on Linux). We use the same crate and version so the
/// resolution matches rtk's exactly — verified empirically against
/// `RTK_TOML_DEBUG=1`.
pub fn global_filters_path() -> Result<PathBuf> {
    let config_dir = dirs::config_dir()
        .context("could not resolve user config directory (dirs::config_dir returned None)")?;
    Ok(config_dir.join("rtk").join("filters.toml"))
}

/// Write the embedded filter set to rtk's global config path, replacing any
/// existing content. Idempotent — safe to call on every bridge startup.
///
/// Returns the path written. Errors are surfaced to the caller but should be
/// treated as warnings (bridge must not refuse to start if rtk bootstrap fails;
/// rtk integration degrades gracefully to no-op).
pub fn ensure_filters_installed() -> Result<PathBuf> {
    if std::env::var(DISABLE_ENV).is_ok_and(|v| v == "1") {
        return Err(anyhow::anyhow!(
            "rtk integration disabled via {DISABLE_ENV}=1"
        ));
    }

    let path = global_filters_path()?;
    let dir = path.parent().context("global filters path has no parent")?;
    std::fs::create_dir_all(dir)
        .with_context(|| format!("failed to create rtk config dir {}", dir.display()))?;

    let mut contents = String::with_capacity(MANAGED_HEADER.len() + EMBEDDED_FILTERS.len());
    contents.push_str(MANAGED_HEADER);
    contents.push_str(EMBEDDED_FILTERS);

    // Skip writing when current disk content is byte-identical — avoids
    // mtime churn that would invalidate any caller's filesystem cache.
    if let Ok(existing) = std::fs::read_to_string(&path) {
        if existing == contents {
            debug!(path = %path.display(), "rtk filters already up to date");
            return Ok(path);
        }
    }

    std::fs::write(&path, &contents)
        .with_context(|| format!("failed to write rtk filters to {}", path.display()))?;

    info!(
        path = %path.display(),
        bytes = contents.len(),
        "rtk filters installed"
    );
    Ok(path)
}

/// Rewrite a shell command through `rtk rewrite` if rtk is available.
///
/// Exit-code contract (verified against `src/hooks/rewrite_cmd.rs` in the rtk
/// repo — the protocol is about permission verdicts, not a simple pass/fail):
///   - exit 0 → Allow; stdout is the (rewritten or unchanged) command. Use it.
///   - exit 1 → Passthrough; no rewrite rule matched. Use original.
///   - exit 2 → Deny; rtk's permission rules block this command. We are NOT
///     a permission-enforcing layer here (that's upstream) — we just return
///     the original and let the existing tool pipeline decide.
///   - exit 3 → Ask/Default; rtk rewrote the command but wants the caller to
///     prompt. In an agent harness we can't prompt, so we use stdout (the
///     rewrite is still valuable — the "ask" is advisory).
///   - other  → treat as not-available; use original (fail safe).
///
/// rtk handles compound commands (`&&`, `||`, `;`, `|`), env prefixes
/// (`FOO=bar sudo cmd`), and heredoc / arithmetic expansion short-circuits
/// internally — we just hand the whole string over.
///
/// Budget: ~5-10ms per call (rtk startup is <10ms, rewrite is pure regex).
/// A spawn failure (rtk removed between startup check and now) is treated as
/// "not available" and the original command is returned.
pub async fn rewrite(command: &str) -> String {
    if !is_rtk_available() {
        return command.to_string();
    }

    match try_rewrite(command).await {
        Ok(rewritten) => rewritten,
        Err(e) => {
            warn!(error = %e, "rtk rewrite failed, passing through original command");
            command.to_string()
        }
    }
}

async fn try_rewrite(command: &str) -> Result<String> {
    let mut child = tokio::process::Command::new("rtk")
        .arg("rewrite")
        .arg(command)
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::piped())
        .spawn()
        .context("failed to spawn rtk rewrite")?;

    let mut stdout = child.stdout.take().context("no stdout on rtk rewrite")?;
    let mut buf = Vec::new();

    // 2-second hard cap. rewrite should complete in well under 50ms; if
    // something wedges, we must not stall the bash tool.
    let read = async {
        stdout.read_to_end(&mut buf).await?;
        child.wait().await
    };
    let status = tokio::time::timeout(Duration::from_secs(2), read)
        .await
        .context("rtk rewrite timed out")??;

    let out = String::from_utf8(buf).context("rtk rewrite emitted non-UTF8 output")?;
    let trimmed = out.trim_end_matches(['\n', '\r']).to_string();

    // Use rtk's rewrite iff (a) it signalled Allow (0) or Ask (3), AND
    // (b) stdout is non-empty. All other exit codes (1 passthrough, 2 deny,
    // anything unexpected) → keep the original command. Deny specifically is
    // upstream's problem to enforce — we don't want rtk silently turning a
    // bash call into a no-op.
    let use_stdout = matches!(status.code(), Some(0) | Some(3)) && !trimmed.is_empty();
    if use_stdout {
        Ok(trimmed)
    } else {
        Ok(command.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn global_filters_path_is_platform_appropriate() {
        let p = global_filters_path().expect("should resolve a path");
        let s = p.to_string_lossy();
        assert!(
            s.ends_with("rtk/filters.toml") || s.ends_with("rtk\\filters.toml"),
            "path should end with rtk/filters.toml, got: {s}"
        );

        // Platform-specific prefix check: surfaces a regression if the
        // `dirs` crate ever changes its macOS behavior and desyncs from rtk.
        #[cfg(target_os = "macos")]
        assert!(
            s.contains("Library/Application Support"),
            "on macOS path should be under Library/Application Support, got: {s}"
        );
        #[cfg(target_os = "linux")]
        assert!(
            s.contains(".config") || s.contains("/config"),
            "on Linux path should be under .config or XDG_CONFIG_HOME, got: {s}"
        );
    }

    #[test]
    fn embedded_filter_set_is_nonempty_and_has_schema() {
        assert!(
            !EMBEDDED_FILTERS.is_empty(),
            "embedded filters should not be empty"
        );
        assert!(
            EMBEDDED_FILTERS.contains("schema_version = 1"),
            "embedded filters should declare schema_version = 1"
        );
        // Spot-check: our consolidated TOML should include the Laravel
        // fallback filter. If this breaks, the asset file drifted.
        assert!(
            EMBEDDED_FILTERS.contains("[filters.artisan-zz-generic]"),
            "embedded filters should include artisan-zz-generic fallback"
        );
    }

    #[tokio::test]
    async fn rewrite_passes_through_when_rtk_disabled() {
        // Force the "disabled" branch: even if rtk is installed on the host,
        // setting BRIDGE_DISABLE_RTK=1 must short-circuit to identity.
        // SAFETY: only this test inspects the envvar; OnceLock caching of
        // RTK_AVAILABLE means we must not rely on unsetting it to later
        // re-enable — tests in this module stay disabled-only.
        // (Using `unsafe` because set_var is marked unsafe in newer Rust.)
        unsafe {
            std::env::set_var(DISABLE_ENV, "1");
        }
        let out = rewrite("git status").await;
        assert_eq!(out, "git status");
    }
}
