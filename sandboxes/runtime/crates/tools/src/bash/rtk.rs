//! RTK (Rust Token Killer) integration for the Bash tool.
//!
//! Two responsibilities:
//!
//! 1. **Bootstrap**: at bridge startup, write our embedded Laravel/PHP filter
//!    set to rtk's user-global config directory so rtk can find it (macOS:
//!    `~/Library/Application Support/rtk/filters.toml`, Linux:
//!    `~/.config/rtk/filters.toml` or `$XDG_CONFIG_HOME/rtk/`). The path is
//!    resolved via the `dirs` crate — the same one rtk uses internally — so
//!    both sides stay in sync across platforms.
//!
//! 2. **Per-call rewrite**: for every Bash tool invocation, prepend `rtk `
//!    to command segments whose first word is in a hard-coded allowlist of
//!    rtk-filterable commands. This replaces the earlier design that
//!    delegated to `rtk rewrite`, whose hardcoded registry doesn't know
//!    about Laravel/PHP commands and would let our filter set sit idle.
//!    The allowlist + segment-aware rewriter lives in `rtk_router`.
//!
//! Escape hatch: `BRIDGE_DISABLE_RTK=1` disables both responsibilities —
//! useful for debugging or if an rtk update regresses.

use std::path::PathBuf;
use std::sync::OnceLock;

use anyhow::{Context, Result};
use tracing::{debug, info};

use super::rtk_router;

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

/// Resolve the global filters.toml path that rtk will read. Matches rtk's
/// own resolution — verified empirically against `RTK_TOML_DEBUG=1`.
pub fn global_filters_path() -> Result<PathBuf> {
    let config_dir = dirs::config_dir()
        .context("could not resolve user config directory (dirs::config_dir returned None)")?;
    Ok(config_dir.join("rtk").join("filters.toml"))
}

/// Write the embedded filter set to rtk's global config path. Idempotent —
/// skips the write when content is byte-identical.
///
/// Errors are surfaced to the caller but should be logged as warnings, not
/// fatal: bridge must not refuse to start if rtk bootstrap fails. rtk
/// integration degrades gracefully to a no-op when this fails.
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

/// Route a shell command through rtk by prepending `rtk ` to every segment
/// whose first bare word is in our allowlist.
///
/// Returns the original string when rtk is unavailable, disabled, or when
/// no segment matches the allowlist. Idempotent: already-rtk commands pass
/// through unchanged.
///
/// See `rtk_router` for the allowlist and segment-parsing logic.
pub fn rewrite(command: &str) -> String {
    if !is_rtk_available() {
        return command.to_string();
    }
    rtk_router::rewrite(command)
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
        assert!(
            EMBEDDED_FILTERS.contains("[filters.artisan-zz-generic]"),
            "embedded filters should include artisan-zz-generic fallback"
        );
    }

    #[test]
    fn rewrite_passes_through_when_rtk_disabled() {
        // SAFETY: set_var is marked unsafe in newer Rust; this test mutates
        // process-global env for the duration of one test case. Because
        // is_rtk_available() caches its result in OnceLock, this assertion
        // holds only if this is the first is_rtk_available() call in the
        // process — which is why the test explicitly checks the disabled
        // branch, not the enabled one.
        unsafe {
            std::env::set_var(DISABLE_ENV, "1");
        }
        assert_eq!(rewrite("git status"), "git status");
        assert_eq!(rewrite("php artisan migrate"), "php artisan migrate");
    }
}
