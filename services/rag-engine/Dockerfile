# syntax=docker/dockerfile:1.7

# Multi-stage Rust build for the rag-engine gRPC service.
# Uses cargo-chef to cache the dependency layer so iterative CI builds
# don't re-compile every crate for a source-only change.
#
# Build context: the Hiveloop monorepo root. The context must include
# the `proto/` directory (the `.proto` file is the build input to
# `rag-engine-proto`'s build.rs) and the `services/rag-engine/` tree.

ARG RUST_VERSION=1.82

# --- stage 1: chef -----------------------------------------------------
FROM rust:${RUST_VERSION}-slim AS chef
WORKDIR /build
RUN apt-get update && apt-get install -y --no-install-recommends \
        pkg-config \
        protobuf-compiler \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN cargo install cargo-chef --locked --version 0.1.68

# --- stage 2: planner --------------------------------------------------
FROM chef AS planner
COPY services/rag-engine /build/services/rag-engine
COPY proto /build/proto
WORKDIR /build/services/rag-engine
RUN cargo chef prepare --recipe-path recipe.json

# --- stage 3: builder --------------------------------------------------
FROM chef AS builder
# Re-copy the chef layout so cargo-chef can cook deps only.
WORKDIR /build/services/rag-engine
COPY --from=planner /build/services/rag-engine/recipe.json recipe.json
RUN cargo chef cook --release --recipe-path recipe.json

# Now copy the actual source tree and build the binary.
COPY services/rag-engine /build/services/rag-engine
COPY proto /build/proto
RUN cargo build --release --bin rag-engine-server

# --- stage 4: runtime --------------------------------------------------
FROM debian:stable-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Non-root user for defense in depth.
RUN useradd --system --create-home --shell /usr/sbin/nologin rag && \
    mkdir -p /var/lib/rag-engine && chown rag:rag /var/lib/rag-engine

COPY --from=builder /build/services/rag-engine/target/release/rag-engine-server \
     /usr/local/bin/rag-engine-server

USER rag
EXPOSE 50051

ENTRYPOINT ["/usr/local/bin/rag-engine-server"]
