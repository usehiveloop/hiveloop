# syntax=docker/dockerfile:1.7

# Multi-stage build:
#   stage 1: compile bridge inside a rust:bookworm image
#   stage 2: runtime image with Node + claude-agent-acp + the bridge binary

# ─── stage 1: build ─────────────────────────────────────────────
FROM rust:1.90-bookworm AS builder

WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY crates ./crates
COPY e2e ./e2e

# Cache the dep build between iterations.
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/usr/local/cargo/git \
    --mount=type=cache,target=/src/target \
    cargo build --release -p bridge \
    && cp /src/target/release/bridge /bridge

# ─── stage 2: runtime ───────────────────────────────────────────
FROM node:22-bookworm-slim

ENV NPM_CONFIG_UPDATE_NOTIFIER=false \
    NPM_CONFIG_FUND=false \
    DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl tini \
    && rm -rf /var/lib/apt/lists/*

# Install both ACP agents globally. Each ships its own native binary as
# a platform-specific optional npm dep; bridge dispatches to one based
# on the agent definition's `harness` field.
RUN npm install -g \
        @agentclientprotocol/claude-agent-acp@0.31.4 \
        opencode-ai@1.14.32 \
    && npm cache clean --force

# Sandbox-style isolation: every $HOME write stays under /work.
ENV HOME=/work \
    CLAUDE_CONFIG_DIR=/work/.claude \
    OPENCODE_CONFIG_DIR=/work/.opencode \
    TMPDIR=/work/tmp \
    NO_BROWSER=1 \
    BRIDGE_WORKING_DIR=/workspace \
    BRIDGE_STORAGE_PATH=/work/bridge.db \
    BRIDGE_LISTEN_ADDR=0.0.0.0:8080 \
    BRIDGE_CONTROL_PLANE_API_KEY=test-control-plane-key \
    BRIDGE_LOG_LEVEL=info,claude_acp=info,opencode_acp=info,harness=debug,runtime=debug

RUN mkdir -p /work/.claude /work/.opencode /work/tmp /workspace

WORKDIR /work

COPY --from=builder /bridge /usr/local/bin/bridge
RUN chmod +x /usr/local/bin/bridge

EXPOSE 8080
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/usr/local/bin/bridge"]
