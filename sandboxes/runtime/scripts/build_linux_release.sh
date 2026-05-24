#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/usr/local/bin:/opt/homebrew/bin:$HOME/.cargo/bin:$PATH"

host_os="$(uname -s)"
case "$(uname -m)" in
  arm64|aarch64)
    default_target="aarch64-unknown-linux-gnu"
    ;;
  *)
    default_target="x86_64-unknown-linux-gnu"
    ;;
esac

TARGET="${HIVY_SANDBOXES_RUNTIME_LINUX_TARGET:-$default_target}"
OUT="${HIVY_SANDBOXES_RUNTIME_LINUX_BINARY:-$ROOT/dist/hivy-sandboxes-runtime-$TARGET}"
RUST_TOOLCHAIN="${HIVY_SANDBOXES_RUNTIME_RUST_TOOLCHAIN:-stable}"
NOFILE_LIMIT="${HIVY_SANDBOXES_RUNTIME_NOFILE_LIMIT:-8192}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

if ! command -v cargo >/dev/null 2>&1; then
  echo "cargo is required to build the runtime" >&2
  exit 1
fi

if current_nofile="$(ulimit -n 2>/dev/null)"; then
  if [[ "$current_nofile" != "unlimited" && "$current_nofile" -lt "$NOFILE_LIMIT" ]]; then
    ulimit -n "$NOFILE_LIMIT" 2>/dev/null || true
  fi
fi

if [[ "$host_os" == "Linux" && "$TARGET" == "$default_target" ]]; then
  mkdir -p "$(dirname "$OUT")"
  cargo build --release --locked -p hivy-sandboxes-runtime
  cp "$ROOT/target/release/hivy-sandboxes-runtime" "$OUT"
  chmod +x "$OUT"
  file "$OUT"
  exit 0
fi

if ! command -v zig >/dev/null 2>&1; then
  echo "zig is required for Linux cross-linking from this host/target combination" >&2
  echo "requested target: $TARGET" >&2
  exit 1
fi

if ! command -v rustup >/dev/null 2>&1; then
  echo "rustup is required so the Linux stdlib target is available to rustc" >&2
  exit 1
fi

CARGO_BIN="$(rustup which --toolchain "$RUST_TOOLCHAIN" cargo)"
RUSTC_BIN="$(rustup which --toolchain "$RUST_TOOLCHAIN" rustc)"

case "$TARGET" in
  x86_64-unknown-linux-gnu)
    zig_target="x86_64-linux-gnu"
    ;;
  aarch64-unknown-linux-gnu)
    zig_target="aarch64-linux-gnu"
    ;;
  *)
    echo "unsupported target for this helper: $TARGET" >&2
    exit 1
    ;;
esac

cat >"$TMP_DIR/zig-cc" <<EOF
#!/usr/bin/env bash
set -euo pipefail
args=()
for arg in "\$@"; do
  case "\$arg" in
    --target=$TARGET)
      ;;
    *)
      args+=("\$arg")
      ;;
  esac
done
exec zig cc -target $zig_target "\${args[@]}"
EOF

cat >"$TMP_DIR/zig-cxx" <<EOF
#!/usr/bin/env bash
set -euo pipefail
args=()
for arg in "\$@"; do
  case "\$arg" in
    --target=$TARGET)
      ;;
    *)
      args+=("\$arg")
      ;;
  esac
done
exec zig c++ -target $zig_target "\${args[@]}"
EOF

chmod +x "$TMP_DIR/zig-cc" "$TMP_DIR/zig-cxx"

rustup target add "$TARGET" --toolchain "$RUST_TOOLCHAIN"
if ! "$RUSTC_BIN" --print target-libdir --target "$TARGET" >/dev/null 2>&1; then
  echo "Rust target $TARGET is still unavailable for toolchain $RUST_TOOLCHAIN after rustup target add" >&2
  echo "try manually: rustup target add $TARGET --toolchain $RUST_TOOLCHAIN" >&2
  exit 1
fi
mkdir -p "$(dirname "$OUT")"

target_env="${TARGET//-/_}"
target_env_upper="$(printf '%s' "$target_env" | tr '[:lower:]' '[:upper:]')"

export "CC_${target_env}=$TMP_DIR/zig-cc"
export "CXX_${target_env}=$TMP_DIR/zig-cxx"
export "AR_${target_env}=zig ar"
export "CARGO_TARGET_${target_env_upper}_LINKER=$TMP_DIR/zig-cc"
export RUSTUP_TOOLCHAIN="$RUST_TOOLCHAIN"
export RUSTC="$RUSTC_BIN"

"$CARGO_BIN" build --release --locked -p hivy-sandboxes-runtime --target "$TARGET"
cp "$ROOT/target/$TARGET/release/hivy-sandboxes-runtime" "$OUT"
chmod +x "$OUT"
file "$OUT"
