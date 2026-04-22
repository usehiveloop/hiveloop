package testhelpers

// Build-once machinery for the Rust rag-engine binary.
//
// Behavior (integration-first, per TESTING.md):
//   - We never mock the Rust engine. The only legitimate test path is to
//     build `services/rag-engine` from source and exec the resulting
//     binary against MinIO. This file owns the per-process build cache.
//   - `cargo build --release --bin rag-engine-server` runs at most once
//     per `go test` invocation per worktree path, guarded by sync.Once.
//   - If `cargo` is not on PATH we fail fast with a human-readable
//     message. No fallback to any embedded server.
//
// Multi-worktree: the Phase 2 plan permits running E2E tests against a
// binary from a sibling worktree (e.g. `rag/phase-2f-server-wiring`).
// The worktree root is resolved once at package load time by walking up
// from this source file; callers may override via
// `RagEngineConfig.BranchOrBinary` (an absolute path to a pre-built
// binary or an absolute path to a worktree root).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// ragEngineBuild caches one Rust build per worktree path. Keyed on the
// absolute path of the worktree root (== parent of services/rag-engine).
// A single `go test` invocation may target only one worktree in practice,
// but the map keeps us safe if a future test ever passes two roots.
type ragEngineBuild struct {
	once sync.Once
	path string
	err  error
}

var (
	ragEngineBuildsMu sync.Mutex
	ragEngineBuilds   = map[string]*ragEngineBuild{}

	repoRootOnce sync.Once
	repoRootVal  string
	repoRootErr  error
)

// DefaultRepoRoot returns the worktree root that contains this source
// file plus services/rag-engine. Cached. Returns an error when the
// walk-up falls off the filesystem without finding both markers.
func DefaultRepoRoot() (string, error) {
	repoRootOnce.Do(func() {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			repoRootErr = fmt.Errorf("runtime.Caller failed; cannot locate repo root")
			return
		}
		dir := filepath.Dir(thisFile)
		for i := 0; i < 10; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				if _, err := os.Stat(filepath.Join(dir, "services", "rag-engine", "Cargo.toml")); err == nil {
					repoRootVal = dir
					return
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		repoRootErr = fmt.Errorf(
			"could not locate repo root containing both go.mod and services/rag-engine/Cargo.toml")
	})
	return repoRootVal, repoRootErr
}

// resolveBuildTarget returns (worktreeRoot, absBinaryPath, error).
//
// Rules:
//   - If override is empty: use DefaultRepoRoot() and the standard target
//     path under it.
//   - If override is a path to an executable file: return it verbatim as
//     the binary; worktreeRoot is the grandparent of target/release.
//   - If override is a directory: treat it as a worktree root and build
//     there.
//   - If override has no slashes and doesn't exist on disk: treat it as a
//     branch name and look for ../hiveloop-<branch-suffix>. Supported as
//     a convenience — we resolve to a worktree path the caller can
//     cross-check with `git worktree list`. Anything we can't resolve is
//     returned as an error (NO silent fallback to the current worktree).
func resolveBuildTarget(override string) (string, string, error) {
	if override == "" {
		root, err := DefaultRepoRoot()
		if err != nil {
			return "", "", err
		}
		return root, filepath.Join(root, "services", "rag-engine", "target", "release", "rag-engine-server"), nil
	}

	abs := override
	if !filepath.IsAbs(abs) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("get cwd to resolve override %q: %w", override, err)
		}
		abs = filepath.Join(cwd, abs)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("override path %q not found: %w", override, err)
	}

	if info.Mode().IsRegular() {
		// Pre-built binary. We still need a worktree root for bookkeeping;
		// infer it by walking up past target/release.
		root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(abs))))
		return root, abs, nil
	}

	if info.IsDir() {
		cargo := filepath.Join(abs, "services", "rag-engine", "Cargo.toml")
		if _, err := os.Stat(cargo); err != nil {
			return "", "", fmt.Errorf("override dir %q has no services/rag-engine/Cargo.toml", abs)
		}
		return abs, filepath.Join(abs, "services", "rag-engine", "target", "release", "rag-engine-server"), nil
	}

	return "", "", fmt.Errorf("override %q is neither a regular file nor a directory", override)
}

// BuildRagEngineBinary builds (or reuses a prior build of) the
// rag-engine-server binary for the requested worktree. Returns the
// absolute path to the compiled binary.
//
// When `override` points to a pre-built binary file, no build happens;
// we just stat it and return the path.
func BuildRagEngineBinary(override string) (string, error) {
	root, binPath, err := resolveBuildTarget(override)
	if err != nil {
		return "", err
	}

	// If override pointed to a pre-existing binary, skip the build.
	if info, err := os.Stat(binPath); err == nil && info.Mode().IsRegular() && override != "" {
		if o2, err := os.Stat(override); err == nil && o2.Mode().IsRegular() {
			return binPath, nil
		}
	}

	ragEngineBuildsMu.Lock()
	entry, ok := ragEngineBuilds[root]
	if !ok {
		entry = &ragEngineBuild{}
		ragEngineBuilds[root] = entry
	}
	ragEngineBuildsMu.Unlock()

	entry.once.Do(func() {
		if _, err := exec.LookPath("cargo"); err != nil {
			entry.err = fmt.Errorf(
				"cargo is required to build services/rag-engine at %s "+
					"(install Rust toolchain from https://rustup.rs and retry)", root)
			return
		}
		cmd := exec.Command("cargo", "build", "--release", "--bin", "rag-engine-server")
		cmd.Dir = filepath.Join(root, "services", "rag-engine")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			entry.err = fmt.Errorf("cargo build --release at %s: %w", cmd.Dir, err)
			return
		}
		if _, err := os.Stat(binPath); err != nil {
			entry.err = fmt.Errorf("built binary missing at %s: %w", binPath, err)
			return
		}
		entry.path = binPath
	})

	return entry.path, entry.err
}
