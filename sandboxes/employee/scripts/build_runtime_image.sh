#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${EMPLOYEE_BRIDGE_RUNTIME_IMAGE:-employee-bridge:runtime}"
BINARY="${EMPLOYEE_BRIDGE_RELEASE_BINARY:-$ROOT/target/release/employee-bridge}"
PLATFORM="${EMPLOYEE_BRIDGE_RUNTIME_PLATFORM:-}"
TMP_CONTEXT="$(mktemp -d)"
trap 'rm -rf "$TMP_CONTEXT"' EXIT

if [[ ! -x "$BINARY" ]]; then
  echo "release binary not found or not executable: $BINARY" >&2
  echo "build it first with: cargo build --release -p employee-bridge" >&2
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
cp "$BINARY" "$TMP_CONTEXT/employee-bridge"

build_args=()
if [[ -n "$PLATFORM" ]]; then
  build_args+=(--platform "$PLATFORM")
fi

docker build \
  "${build_args[@]}" \
  -f "$TMP_CONTEXT/Dockerfile.runtime" \
  -t "$IMAGE" \
  "$TMP_CONTEXT"

echo "built runtime image: $IMAGE"
