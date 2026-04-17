# Ubuntu 24.04 test bed for the LSP smoke flow.
# Matches the production Dev-Box sandbox.
FROM --platform=linux/amd64 ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive \
    PATH=/root/.cargo/bin:/root/.local/bin:/root/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update && apt-get install -y --no-install-recommends \
        curl wget git unzip jq ca-certificates build-essential \
        pkg-config libssl-dev \
        nodejs npm \
        golang-go \
        python3 python3-pip python3-venv \
        ruby-full \
 && rm -rf /var/lib/apt/lists/*

# Rust toolchain (matches workspace stable channel)
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
 | sh -s -- -y --default-toolchain stable --profile minimal

WORKDIR /work
