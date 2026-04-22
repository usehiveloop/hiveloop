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

    // Include path resolution:
    //   * Our repo's proto/ directory (so `rag_engine.proto` is found).
    //   * Google's well-known types (Timestamp et al). `protoc` bundles
    //     these, but the include path it uses differs per distribution
    //     (`/usr/include` on Debian when `libprotobuf-dev` is installed,
    //     `<protoc-install>/include` on Homebrew, etc.). If the env var
    //     `PROTOC_INCLUDE` is set we honour it; otherwise we fall back
    //     to a short list of well-known locations and rely on `protoc`'s
    //     own built-in descriptor for the rest.
    let mut includes: Vec<std::path::PathBuf> = vec![proto_root.clone()];
    if let Ok(extra) = env::var("PROTOC_INCLUDE") {
        for p in extra.split(':') {
            if !p.is_empty() {
                includes.push(std::path::PathBuf::from(p));
            }
        }
    }
    for candidate in [
        "/usr/include",
        "/usr/local/include",
        "/opt/homebrew/include",
    ] {
        let p = std::path::PathBuf::from(candidate);
        if p.join("google/protobuf/timestamp.proto").exists() {
            includes.push(p);
        }
    }

    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        .compile_protos(&[proto_file], &includes)?;

    Ok(())
}
