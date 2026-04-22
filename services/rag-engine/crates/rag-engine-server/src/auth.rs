//! Shared-secret authentication for the `rag-engine` gRPC surface.
//!
//! Phase 2 locks in a shared-secret bearer token between Hiveloop's Go
//! backend and the Rust engine. This module implements the server side
//! of that check as a tonic `Interceptor`:
//!
//!   * `authorization: Bearer <shared-secret>` metadata is required on
//!     every non-health RPC.
//!   * Missing / malformed → `Status::unauthenticated`.
//!   * Present but wrong → `Status::unauthenticated`.
//!   * The secret comparison runs in constant time via `subtle` so
//!     attackers cannot derive the secret from response-time
//!     variation.
//!
//! The health service (`grpc.health.v1.Health`) is wrapped by a separate
//! unauthenticated tower layer in `main.rs` / `service.rs`; this
//! interceptor does not see those requests.

use std::sync::Arc;
use subtle::ConstantTimeEq;
use tonic::{metadata::MetadataValue, service::Interceptor, Request, Status};

/// Metadata key carrying `Bearer <secret>`. We reuse the standard
/// `authorization` header so the surface is familiar.
pub const AUTH_METADATA_KEY: &str = "authorization";

const BEARER_PREFIX: &str = "Bearer ";

/// Clone-cheap wrapper holding the shared secret. Stored as `Arc<str>`
/// so every cloned interceptor points at the same allocation.
#[derive(Clone)]
pub struct SharedSecretAuth {
    secret: Arc<str>,
}

impl SharedSecretAuth {
    /// Construct a new interceptor. Panics if `secret` is empty; a server
    /// should have refused to boot in that case via `ConfigError`.
    pub fn new(secret: impl Into<Arc<str>>) -> Self {
        let secret = secret.into();
        assert!(
            !secret.is_empty(),
            "shared secret must be non-empty; Config::load should have rejected this"
        );
        Self { secret }
    }

    /// Core check: returns `Ok(())` if `provided` matches the server's
    /// secret in constant time, `Err(Status::unauthenticated)` otherwise.
    /// Pulled out for direct unit testing without a tonic `Request`.
    //
    // `clippy::result_large_err` fires because `tonic::Status` is ~176 B.
    // It's the type tonic's interceptor trait forces on us, so boxing
    // would only add an allocation on every unauth request. Allowed.
    #[allow(clippy::result_large_err)]
    fn check(&self, provided: &str) -> Result<(), Status> {
        let expected = self.secret.as_bytes();
        let given = provided.as_bytes();

        // Constant-time equality. We must call the comparison on
        // equal-length buffers to actually get a timing-independent
        // result, so we first check the lengths and only THEN do a CT
        // compare against a same-sized padding when lengths differ.
        // `subtle::ConstantTimeEq::ct_eq` on equal-length slices is CT.
        if expected.len() != given.len() {
            // Still run a CT compare against a zero buffer of the
            // expected length to keep the rejection path time-class
            // similar to the "same length but wrong bytes" path.
            let sink = vec![0u8; expected.len()];
            let _ = sink.ct_eq(&sink); // side-effect: keeps CPU busy
            return Err(Status::unauthenticated("invalid credentials"));
        }

        if expected.ct_eq(given).unwrap_u8() == 1 {
            Ok(())
        } else {
            Err(Status::unauthenticated("invalid credentials"))
        }
    }
}

impl Interceptor for SharedSecretAuth {
    fn call(&mut self, request: Request<()>) -> Result<Request<()>, Status> {
        let header: &MetadataValue<_> = request
            .metadata()
            .get(AUTH_METADATA_KEY)
            .ok_or_else(|| Status::unauthenticated("missing authorization metadata"))?;

        let header_str = header
            .to_str()
            .map_err(|_| Status::unauthenticated("malformed authorization metadata"))?;

        let token = header_str
            .strip_prefix(BEARER_PREFIX)
            .ok_or_else(|| Status::unauthenticated("authorization must be 'Bearer <token>'"))?;

        self.check(token)?;
        Ok(request)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Business behavior: matching secret passes.
    #[test]
    fn check_accepts_exact_match() {
        let auth = SharedSecretAuth::new("s3cret-value");
        assert!(auth.check("s3cret-value").is_ok());
    }

    // Business behavior: wrong secret of the same length is rejected.
    // This is the branch that actually runs the constant-time compare.
    #[test]
    fn check_rejects_same_length_wrong_bytes() {
        let auth = SharedSecretAuth::new("s3cret-value");
        let err = auth.check("w4ong-value!").unwrap_err();
        assert_eq!(err.code(), tonic::Code::Unauthenticated);
    }

    // Business behavior: different-length secret rejected through the
    // length-mismatch branch.
    #[test]
    fn check_rejects_different_length() {
        let auth = SharedSecretAuth::new("s3cret-value");
        let err = auth.check("short").unwrap_err();
        assert_eq!(err.code(), tonic::Code::Unauthenticated);
    }
}
