#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/usr/local/bin:/opt/homebrew/bin:$HOME/.cargo/bin:$PATH"
DOCKER_BIN="${DOCKER_BIN:-$(command -v docker)}"
IMAGE="${HIVY_SANDBOXES_RUNTIME_IMAGE:-hivy-sandboxes-runtime:runtime}"
BINARY="${HIVY_SANDBOXES_RUNTIME_BINARY:-$ROOT/target/release/hivy-sandboxes-runtime}"
PLATFORM="${HIVY_SANDBOXES_RUNTIME_PLATFORM:-}"
PROFILE="${HIVY_SANDBOXES_RUNTIME_PROFILE:-employee}"
TMP_CONTEXT="$(mktemp -d)"
trap 'rm -rf "$TMP_CONTEXT"' EXIT

if [[ ! -x "$BINARY" ]]; then
  echo "release binary not found or not executable: $BINARY" >&2
  echo "build it first with: cargo build --release -p hivy-sandboxes-runtime" >&2
  exit 1
fi

kind="$(file "$BINARY")"
echo "$kind"
if [[ "$kind" != *"ELF"* ]] || [[ "$kind" != *"Linux"* ]]; then
  echo "refusing to package non-Linux binary into Debian image" >&2
  echo "expected an ELF Linux release binary at: $BINARY" >&2
  exit 1
fi

if [[ -z "$PLATFORM" ]]; then
  case "$kind" in
    *"x86-64"*)
      PLATFORM="linux/amd64"
      ;;
    *"ARM aarch64"*)
      PLATFORM="linux/arm64"
      ;;
  esac
fi

cp "$ROOT/Dockerfile.runtime" "$TMP_CONTEXT/Dockerfile.runtime"
cp "$BINARY" "$TMP_CONTEXT/hivy-sandboxes-runtime"
mkdir -p "$TMP_CONTEXT/docker"
cp -R "$ROOT/docker/runtime" "$TMP_CONTEXT/docker/runtime"

build_args=()
if [[ -n "$PLATFORM" ]]; then
  build_args+=(--platform "$PLATFORM")
fi

"$DOCKER_BIN" build \
  "${build_args[@]}" \
  --build-arg "RUNTIME_IMAGE_PROFILE=$PROFILE" \
  -f "$TMP_CONTEXT/Dockerfile.runtime" \
  -t "$IMAGE" \
  "$TMP_CONTEXT"

echo "built runtime image: $IMAGE (profile=$PROFILE)"
