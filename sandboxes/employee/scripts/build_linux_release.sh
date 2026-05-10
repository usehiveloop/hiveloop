#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/usr/local/bin:/opt/homebrew/bin:$HOME/.cargo/bin:$PATH"

case "$(uname -m)" in
  arm64|aarch64)
    default_target="aarch64-unknown-linux-gnu"
    ;;
  *)
    default_target="x86_64-unknown-linux-gnu"
    ;;
esac

TARGET="${EMPLOYEE_BRIDGE_LINUX_TARGET:-$default_target}"
OUT="${EMPLOYEE_BRIDGE_LINUX_BINARY:-$ROOT/dist/employee-bridge-$TARGET}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

if ! command -v zig >/dev/null 2>&1; then
  echo "zig is required for local Linux cross-linking on non-Linux hosts" >&2
  exit 1
fi

if ! command -v rustup >/dev/null 2>&1; then
  echo "rustup is required so the Linux stdlib target is available to rustc" >&2
  exit 1
fi

export PATH="$HOME/.cargo/bin:$PATH"

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

rustup target add "$TARGET" --toolchain stable
mkdir -p "$(dirname "$OUT")"

target_env="${TARGET//-/_}"
target_env_upper="$(printf '%s' "$target_env" | tr '[:lower:]' '[:upper:]')"

export "CC_${target_env}=$TMP_DIR/zig-cc"
export "CXX_${target_env}=$TMP_DIR/zig-cxx"
export "AR_${target_env}=zig ar"
export "CARGO_TARGET_${target_env_upper}_LINKER=$TMP_DIR/zig-cc"

rustup run stable cargo build --release -p employee-bridge --target "$TARGET"
cp "$ROOT/target/$TARGET/release/employee-bridge" "$OUT"
chmod +x "$OUT"
file "$OUT"
