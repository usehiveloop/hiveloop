package testhelpers

// Public helper that spins up a real Rust `rag-engine-server` in test
// mode for Go integration tests.
//
// Contract:
//   - Binary is built at most once per `go test` invocation per worktree
//     path (see ragengine_binary.go). Subsequent tests reuse the
//     artifact at `services/rag-engine/target/release/rag-engine-server`.
//   - Each call picks a free TCP port on 127.0.0.1, generates a random
//     32-byte-hex shared secret, and launches the binary.
//   - Fake embedder + fake reranker are the default so tests are fast
//     and make zero paid API calls. Real providers can be selected via
//     Embedder/Reranker fields but the helper never injects API keys.
//   - A real MinIO bucket (default `hiveloop-rag-test`) must be up. When
//     the bucket is missing we create it; when MinIO itself is down we
//     fail loudly with the exact remediation string documented in
//     TESTING.md Hard Rule #7.
//   - Cleanup on `t.Cleanup`: SIGTERM with 10s grace → SIGKILL, then
//     release the per-test S3 prefix so tests don't leak state.
//
// NOTE on unimplemented handlers: this helper is independent of which
// RPCs the binary actually serves. When a test hits a handler that
// still returns UNIMPLEMENTED, the helper itself keeps working — the
// binary boots, health flips to SERVING, a client connects — and the
// calling E2E test fails loudly. That is the correct behaviour per
// TESTING.md (no skips).

import (
	"os"
	"time"
)

func ResolveBinaryForBranch(override string) (string, error) {
	_, path, err := resolveBuildTarget(override)
	if err != nil {
		return "", err
	}
	return path, nil
}

// binaryStat is a tiny typed wrapper around os.Stat used by the
// build-reuse test.
type binaryStat struct {
	Path    string
	ModTime time.Time
	Size    int64
}

// StatBinary returns the stat metadata of the current rag-engine
// binary, if any. Used by TestStartRagEngineInTestMode_ReusesBinary.
func StatBinary(override string) (*binaryStat, error) {
	_, path, err := resolveBuildTarget(override)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &binaryStat{Path: path, ModTime: info.ModTime(), Size: info.Size()}, nil
}
