//! Integration-style test for `Config::load`.
//!
//! Env loading via `figment` reads the PROCESS environment, so we
//! serialise these tests behind a mutex — they can't run in parallel
//! without polluting each other. `cargo test` defaults to threads,
//! hence the mutex.
//!
//! Business value: deploy-time config correctness. If env overrides
//! stop working, every operator who ships the container with a
//! different port or secret silently falls back to defaults and we
//! get a very subtle outage.

use std::sync::Mutex;

use rag_engine_server::{Config, ConfigError};

// Global mutex: env vars are process-global, so these tests are
// inherently serial. We don't use `#[serial_test]` to avoid pulling
// in a crate just for one test module.
static ENV_LOCK: Mutex<()> = Mutex::new(());

fn clear_env() {
    for key in [
        "RAG_ENGINE_LISTEN_ADDR",
        "RAG_ENGINE_SHARED_SECRET",
        "RAG_ENGINE_LOG_LEVEL",
        "RAG_ENGINE_CONFIG",
    ] {
        std::env::remove_var(key);
    }
}

#[test]
fn loads_values_from_env_overrides() {
    let _g = ENV_LOCK.lock().unwrap();
    clear_env();

    std::env::set_var("RAG_ENGINE_LISTEN_ADDR", "127.0.0.1:7443");
    std::env::set_var("RAG_ENGINE_SHARED_SECRET", "env-secret");
    std::env::set_var("RAG_ENGINE_LOG_LEVEL", "debug");

    let cfg = Config::load().expect("config should load");
    assert_eq!(cfg.listen_addr, "127.0.0.1:7443");
    assert_eq!(cfg.shared_secret, "env-secret");
    assert_eq!(cfg.log_level, "debug");

    clear_env();
}

#[test]
fn missing_shared_secret_returns_typed_error() {
    let _g = ENV_LOCK.lock().unwrap();
    clear_env();

    // Set everything EXCEPT the secret.
    std::env::set_var("RAG_ENGINE_LISTEN_ADDR", "0.0.0.0:50051");
    std::env::set_var("RAG_ENGINE_LOG_LEVEL", "info");

    let err = Config::load().expect_err("must refuse to boot without secret");
    assert!(
        matches!(err, ConfigError::MissingSharedSecret),
        "expected MissingSharedSecret, got {err:?}"
    );

    clear_env();
}

#[test]
fn empty_shared_secret_is_also_rejected() {
    let _g = ENV_LOCK.lock().unwrap();
    clear_env();

    std::env::set_var("RAG_ENGINE_SHARED_SECRET", "   "); // whitespace-only
    std::env::set_var("RAG_ENGINE_LOG_LEVEL", "info");

    let err = Config::load().expect_err("whitespace-only secret must be rejected");
    assert!(matches!(err, ConfigError::MissingSharedSecret));

    clear_env();
}

#[test]
fn defaults_applied_when_env_missing_non_secret_keys() {
    let _g = ENV_LOCK.lock().unwrap();
    clear_env();

    std::env::set_var("RAG_ENGINE_SHARED_SECRET", "only-secret-set");

    let cfg = Config::load().expect("should load using defaults for other keys");
    assert_eq!(cfg.listen_addr, "0.0.0.0:50051");
    assert_eq!(cfg.log_level, "info");
    assert_eq!(cfg.shared_secret, "only-secret-set");

    clear_env();
}
