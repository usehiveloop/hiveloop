//! Graceful shutdown signal handling.
//!
//! Tranche 2H adds SIGTERM + SIGINT handling (previously in-line in
//! `main.rs`) and a `ShutdownConfig` carrying the drain deadline. The
//! future returned by [`shutdown_signal`] is designed to be passed
//! directly to `tonic::transport::Server::serve_with_shutdown`, which
//! handles the in-flight RPC drain for us. Our job is to give tonic a
//! clean trigger and surface the deadline for higher-level force-abort
//! logic.
//!
//! # Signal semantics
//!
//! * `SIGINT`  — ctrl-c from an attached terminal (dev/test).
//! * `SIGTERM` — delivered by `docker stop` / `kubectl delete pod` /
//!   systemd. This is the signal ops infrastructure actually uses.
//!
//! On any non-Unix platform we only wait on `ctrl_c` (Windows has no
//! SIGTERM; its equivalent is `CTRL_SHUTDOWN_EVENT`, which is out of
//! scope for this phase — the binary runs in Linux containers in prod).
//!
//! # Why the deadline lives here, not in tonic
//!
//! tonic's graceful shutdown will wait indefinitely for the final
//! in-flight RPC to return. In practice a wedged LanceDB write could
//! hold a connection open forever. Phase 2H bounds that with
//! `drain_deadline` (default 30 s). The deadline is *not* enforced
//! inside this module — it's enforced in `main.rs::run` by racing the
//! server join against a timer. Keeping the deadline as a configured
//! value near the signal handler keeps all "how we exit" code
//! colocated.

use std::time::Duration;

use tracing::info;

/// Tunable lifecycle knobs. Populated from env in `main.rs`.
#[derive(Debug, Clone, Copy)]
pub struct ShutdownConfig {
    /// How long we let tonic drain in-flight RPCs after the first
    /// shutdown signal arrives. If any RPC is still running after this
    /// budget, `main::run` force-aborts the server join handle and
    /// exits with a non-zero code.
    pub drain_deadline: Duration,
}

impl Default for ShutdownConfig {
    fn default() -> Self {
        Self {
            drain_deadline: Duration::from_secs(30),
        }
    }
}

impl ShutdownConfig {
    /// Load the drain deadline from `RAG_ENGINE_DRAIN_DEADLINE_SECS`,
    /// falling back to the default (30 s). Invalid or zero values fall
    /// back to the default — a zero-second drain is always a
    /// misconfiguration (tonic needs at least a tick to close listeners)
    /// and silently ignoring it is the operationally correct call.
    pub fn from_env() -> Self {
        let deadline = std::env::var("RAG_ENGINE_DRAIN_DEADLINE_SECS")
            .ok()
            .and_then(|s| s.parse::<u64>().ok())
            .filter(|&n| n > 0)
            .map(Duration::from_secs)
            .unwrap_or_else(|| Self::default().drain_deadline);
        Self {
            drain_deadline: deadline,
        }
    }
}

/// Resolve as soon as the process receives SIGINT (`ctrl-c`) OR SIGTERM.
/// Logs which signal fired. Returns `()` on either — the caller can't
/// distinguish the two, which is deliberate: the drain behavior is
/// identical.
///
/// Passed directly to `Server::serve_with_shutdown(addr, shutdown_signal())`.
pub async fn shutdown_signal() {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{signal, SignalKind};
        // Install both signal handlers upfront. If either install
        // fails we fall back to ctrl-c only rather than panicking —
        // a pod that can't catch SIGTERM still needs to respond to
        // ctrl-c during `kubectl exec`.
        let mut sigterm = match signal(SignalKind::terminate()) {
            Ok(s) => s,
            Err(err) => {
                tracing::warn!(?err, "failed to install SIGTERM handler; ctrl-c only");
                let _ = tokio::signal::ctrl_c().await;
                info!(signal = "SIGINT", "shutdown signal received, draining");
                return;
            }
        };
        tokio::select! {
            _ = tokio::signal::ctrl_c() => {
                info!(signal = "SIGINT", "shutdown signal received, draining");
            }
            _ = sigterm.recv() => {
                info!(signal = "SIGTERM", "shutdown signal received, draining");
            }
        }
    }
    #[cfg(not(unix))]
    {
        let _ = tokio::signal::ctrl_c().await;
        info!(signal = "SIGINT", "shutdown signal received, draining");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_deadline_is_thirty_seconds() {
        assert_eq!(
            ShutdownConfig::default().drain_deadline,
            Duration::from_secs(30)
        );
    }

    #[test]
    fn from_env_rejects_zero_and_falls_back_to_default() {
        // Rust has no scoped env lock; these tests can't safely run in
        // parallel with real env mutation. We validate the pure logic by
        // setting a known value and unsetting afterwards.
        std::env::set_var("RAG_ENGINE_DRAIN_DEADLINE_SECS", "0");
        let cfg = ShutdownConfig::from_env();
        assert_eq!(cfg.drain_deadline, Duration::from_secs(30));
        std::env::remove_var("RAG_ENGINE_DRAIN_DEADLINE_SECS");
    }

    #[test]
    fn from_env_parses_positive_seconds() {
        std::env::set_var("RAG_ENGINE_DRAIN_DEADLINE_SECS", "5");
        let cfg = ShutdownConfig::from_env();
        assert_eq!(cfg.drain_deadline, Duration::from_secs(5));
        std::env::remove_var("RAG_ENGINE_DRAIN_DEADLINE_SECS");
    }
}
