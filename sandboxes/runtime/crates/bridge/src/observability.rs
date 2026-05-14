//! Sentry initialization. Errors-only — no perf traces.
//!
//! Reads `SENTRY_DSN` from env. When unset, returns `None` and the rest
//! of the binary runs uninstrumented. When set, initializes the SDK with:
//!
//! - the `panic` integration (panics → Sentry events)
//! - `attach_stacktrace = true` (stack traces on `capture_error`)
//! - the `tracing` integration (`tracing::error!` becomes a Sentry event;
//!   `info!`/`warn!` become breadcrumbs leading up to it)
//! - `traces_sample_rate = 0.0` (no perf transactions; this is errors-only)
//!
//! Environment knobs:
//! - `SENTRY_DSN`              — DSN; if absent, Sentry is disabled.
//! - `SENTRY_ENVIRONMENT`      — defaults to `production`.
//! - `SENTRY_RELEASE`          — defaults to `bridge@<cargo pkg version>`.
//! - `BRIDGE_INSTANCE_ID`      — attached as a `server_name` for grouping.

use std::borrow::Cow;
use tracing::info;

/// Initialize Sentry if `SENTRY_DSN` is set. Returns the guard which MUST
/// be held for the lifetime of the process — dropping it flushes and
/// shuts down the transport.
pub fn init_sentry() -> Option<sentry::ClientInitGuard> {
    let dsn = std::env::var("SENTRY_DSN").ok().filter(|s| !s.is_empty())?;

    let environment = std::env::var("SENTRY_ENVIRONMENT")
        .ok()
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| "production".to_string());

    let release = std::env::var("SENTRY_RELEASE")
        .ok()
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| format!("bridge@{}", env!("CARGO_PKG_VERSION")));

    let server_name = std::env::var("BRIDGE_INSTANCE_ID")
        .ok()
        .filter(|s| !s.is_empty());

    let debug = std::env::var("SENTRY_DEBUG")
        .ok()
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false);

    let options = sentry::ClientOptions {
        release: Some(Cow::Owned(release.clone())),
        environment: Some(Cow::Owned(environment.clone())),
        server_name: server_name.clone().map(Cow::Owned),
        attach_stacktrace: true,
        send_default_pii: false,
        // Errors-only. If perf tracing is wanted later, raise this.
        traces_sample_rate: 0.0,
        max_breadcrumbs: 100,
        debug,
        ..Default::default()
    };

    let guard = sentry::init((dsn, options));

    // Stamp every event with always-true tags so the dashboard can filter
    // by harness/version/instance without per-call configure_scope.
    sentry::configure_scope(|scope| {
        scope.set_tag("bridge.version", env!("CARGO_PKG_VERSION"));
        if let Some(id) = &server_name {
            scope.set_tag("bridge.instance_id", id);
        }
        if std::env::var("IS_SANDBOX").as_deref() == Ok("1") {
            scope.set_tag("bridge.sandbox", "1");
        }
    });

    info!(
        environment = %environment,
        release = %release,
        instance_id = ?server_name,
        "sentry initialized"
    );

    // SENTRY_BOOT_PING=1 sends a one-shot info-level event right after init
    // so operators can verify the transport actually reaches the configured
    // DSN without waiting for a real error to fire.
    if std::env::var("SENTRY_BOOT_PING").as_deref() == Ok("1") {
        sentry::capture_message("bridge boot ping", sentry::Level::Error);
    }

    Some(guard)
}
