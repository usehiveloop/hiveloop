//! Build script for the `rag-engine-proto` crate.
//!
//! Compiles `proto/rag_engine.proto` (at the Hiveloop monorepo root) into
//! both gRPC server and client stubs via `tonic-build`. This is the ONLY
//! place in the workspace that invokes the proto compiler — every other
//! crate consumes the generated code via `rag_engine_proto`.

use std::env;
use std::path::PathBuf;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Locate the proto file relative to this crate's manifest directory.
    // Layout:
    //   <repo-root>/proto/rag_engine.proto
    //   <repo-root>/services/rag-engine/crates/rag-engine-proto/build.rs
    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR")?);
    let proto_root = manifest_dir
        .join("..")
        .join("..")
        .join("..")
        .join("..")
        .join("proto");
    let proto_file = proto_root.join("rag_engine.proto");

    println!("cargo:rerun-if-changed={}", proto_file.display());

    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        .compile_protos(&[proto_file], &[proto_root])?;

    Ok(())
}
