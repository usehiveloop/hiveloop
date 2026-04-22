#!/usr/bin/env bash
# Download the LanceDB Go SDK native artifacts into .lancedb-native/.
#
# The github.com/lancedb/lancedb-go package is a CGO wrapper around a Rust
# static library. The library is NOT distributed via `go get`; each
# developer (and CI) needs to run this script once. It is idempotent.
#
# Currently pins to v0.1.2 — the only tagged release with prebuilt
# binaries. See internal/rag/doc/SPIKE_RESULT.md for the known gaps in
# that release and the migration options (build from main, sidecar, etc.).
set -euo pipefail

VERSION="${LANCEDB_GO_VERSION:-v0.1.2}"
DEST_ROOT="${DEST_ROOT:-$(cd "$(dirname "$0")/.." && pwd)/.lancedb-native}"

if [[ -f "$DEST_ROOT/include/lancedb.h" ]]; then
  echo "lancedb-go native artifacts already present at $DEST_ROOT — skipping download."
  exit 0
fi

echo "Downloading LanceDB Go native artifacts ($VERSION) into $DEST_ROOT..."
mkdir -p "$DEST_ROOT"
cd "$DEST_ROOT"

SCRIPT_URL="https://raw.githubusercontent.com/lancedb/lancedb-go/$VERSION/scripts/download-artifacts.sh"
curl -sSL "$SCRIPT_URL" -o download-artifacts.sh
chmod +x download-artifacts.sh
./download-artifacts.sh "$VERSION"

echo "Done. Native libraries installed at $DEST_ROOT/lib/."
