//! Gates that the proto crate emits BOTH the server trait and the
//! client stub.
//!
//! Business value: downstream tranches depend on both — the server
//! binary mounts `RagEngineServer<T: RagEngine>`, integration tests
//! and the Go client (via the same `.proto`) use the client stub. A
//! regression in `build.rs` that accidentally dropped
//! `build_client(true)` or `build_server(true)` would compile this
//! crate but break every test harness that uses it — we'd rather it
//! fail right here.

// Server side: the trait + generated server struct must both exist.
#[allow(dead_code, unused_imports)]
mod server_surface {
    use rag_engine_proto::rag_engine_server::{RagEngine, RagEngineServer};

    // If codegen drops `build_server(true)`, `rag_engine_server` goes
    // missing and this fails to compile.
    fn _assert_server_trait_is_object_safe_enough_for_tonic<T: RagEngine>() {
        // No body — we only needed the bound to resolve.
    }

    fn _assert_server_struct_exists<T: RagEngine>(_svc: T) -> RagEngineServer<T> {
        RagEngineServer::new(_svc)
    }
}

// Client side: the generated client struct + ::new must exist.
#[allow(dead_code, unused_imports)]
mod client_surface {
    use rag_engine_proto::rag_engine_client::RagEngineClient;
    use tonic::transport::Channel;

    fn _assert_client_type_exists(ch: Channel) -> RagEngineClient<Channel> {
        RagEngineClient::new(ch)
    }
}

// One runtime test so `cargo test` reports a passing test rather than
// just "compile clean, no tests ran".
#[test]
fn proto_surface_compiles_with_server_and_client_stubs() {
    // The two modules above fail at compile time if the surface is
    // missing. Reaching this assertion at runtime means we're good.
}
