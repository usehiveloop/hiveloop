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
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/rag/ragclient"
)

// RagEngineConfig tunes StartRagEngineInTestMode. Zero-value fields use
// documented defaults.
type RagEngineConfig struct {
	// Embedder selects the embedding backend. Default "fake". Supported
	// values correspond to the Rust crate's `LLM_PROVIDER`:
	//   "fake"           — in-memory deterministic embedder (DEFAULT)
	//   "openai_compat"  — real OpenAI-compatible API (requires creds via ExtraEnv)
	Embedder string
	// Reranker selects the reranker backend. Default "fake". Supported
	// values correspond to the Rust crate's `RERANKER_KIND`:
	//   "fake"        — deterministic rank order (DEFAULT)
	//   "siliconflow" — real SiliconFlow API (requires creds via ExtraEnv)
	Reranker string
	// EmbeddingDim is forwarded as `LLM_EMBEDDING_DIM`. Default 2560 to
	// match the production Qwen3-Embedding-4B setup.
	EmbeddingDim uint32
	// MinIO overrides the docker-compose defaults when nonzero.
	MinIO MinIOConfig
	// MinIOBucket is the S3 bucket used for LanceDB storage. Default
	// `hiveloop-rag-test`.
	MinIOBucket string
	// LancePrefix is appended to the bucket URI so every RagEngineInstance
	// gets isolated storage. When empty we generate a UUID-based prefix.
	LancePrefix string
	// BranchOrBinary overrides the worktree to build from OR points at
	// a pre-built binary. See ragengine_binary.go::resolveBuildTarget.
	// Common pattern: set via env `RAG_ENGINE_BRANCH` in tests.
	BranchOrBinary string
	// ExtraEnv is appended verbatim to the child's environment. Values
	// here override anything the helper set.
	ExtraEnv map[string]string
	// BootTimeout caps how long StartRagEngineInTestMode waits for the
	// gRPC health check to report SERVING. Default 30s.
	BootTimeout time.Duration
	// ShutdownTimeout caps how long cleanup waits after SIGTERM before
	// escalating to SIGKILL. Default 10s.
	ShutdownTimeout time.Duration
}

// RagEngineInstance is a live, health-verified rag-engine-server process.
type RagEngineInstance struct {
	// Addr is the host:port the server is listening on.
	Addr string
	// SharedSecret is the bearer token the client must present.
	SharedSecret string
	// Client is a connected ragclient.Client with the shared secret and
	// a sensible per-RPC deadline already configured. Tests use this as
	// the single entrypoint to the engine.
	Client *ragclient.Client
	// BinaryPath is the absolute path of the binary that was exec'd —
	// exported so tests can stat it (e.g. the build-reuse test).
	BinaryPath string
	// MinIO is the resolved MinIO config the engine was pointed at.
	MinIO MinIOConfig
	// Bucket is the resolved bucket name.
	Bucket string
	// Prefix is the per-instance S3 prefix under Bucket. Cleanup deletes
	// everything under it.
	Prefix string

	cmd      *exec.Cmd
	shutOnce sync.Once
	done     chan struct{}
}

// Stop requests graceful shutdown (SIGTERM) with escalation to SIGKILL
// after ShutdownTimeout. Safe to call multiple times. Normally tests
// don't call this directly — it's invoked by the t.Cleanup handler.
func (r *RagEngineInstance) Stop() {
	r.shutOnce.Do(func() {
		if r.cmd != nil && r.cmd.Process != nil {
			_ = r.cmd.Process.Signal(syscall.SIGTERM)
			waited := make(chan error, 1)
			go func() { waited <- r.cmd.Wait() }()
			timeout := 10 * time.Second
			select {
			case <-waited:
			case <-time.After(timeout):
				_ = r.cmd.Process.Kill()
				<-waited
			}
		}
		close(r.done)
	})
}

// Done returns a channel that closes after the process has exited and
// the Stop handler has returned. Used by the graceful-shutdown test.
func (r *RagEngineInstance) Done() <-chan struct{} { return r.done }

// StartRagEngineInTestMode launches a real Rust rag-engine-server on an
// ephemeral port and returns a ready-to-use instance. It asserts MinIO
// is reachable (creating the bucket if needed), waits for health, and
// registers t.Cleanup handlers to shut the process down and delete the
// per-test S3 prefix.
//
// This function is the single chokepoint for ALL Go tests that need a
// real rag-engine. It must never mock or stub.
func StartRagEngineInTestMode(t *testing.T, cfg RagEngineConfig) *RagEngineInstance {
	t.Helper()

	cfg = applyDefaults(cfg)

	// Hard-rule-7 check: MinIO must be up. We do this BEFORE paying the
	// build cost so a developer who forgot `make test-services-up` sees
	// the guidance in seconds, not minutes.
	AssertMinIOUp(t, cfg.MinIO)

	// Ensure the bucket exists (idempotent). The per-test prefix
	// ensures state isolation without creating a new bucket per test.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := EnsureBucket(ctx, cfg.MinIO, cfg.MinIOBucket); err != nil {
		t.Fatalf("ensure bucket %q on %s: %v", cfg.MinIOBucket, cfg.MinIO.Endpoint, err)
	}

	bin, err := BuildRagEngineBinary(cfg.BranchOrBinary)
	if err != nil {
		t.Fatalf("build rag-engine: %v", err)
	}

	addr := pickFreePort(t)
	secret := mustRandomHex(t, 32)

	env := buildChildEnv(cfg, addr, secret)

	cmd := exec.Command(bin)
	cmd.Env = env
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	// Own process group: prevents ctrl-c from the Go test harness
	// racing against our SIGTERM.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start rag-engine at %s: %v", bin, err)
	}

	inst := &RagEngineInstance{
		Addr:         addr,
		SharedSecret: secret,
		BinaryPath:   bin,
		MinIO:        cfg.MinIO,
		Bucket:       cfg.MinIOBucket,
		Prefix:       cfg.LancePrefix,
		cmd:          cmd,
		done:         make(chan struct{}),
	}
	t.Cleanup(func() {
		inst.Stop()
		if inst.Prefix != "" {
			cctx, ccancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer ccancel()
			if err := DeleteS3Prefix(cctx, cfg.MinIO, cfg.MinIOBucket, inst.Prefix); err != nil {
				// Leaking state in MinIO is bad but non-fatal to the test.
				t.Logf("delete S3 prefix %s/%s: %v", cfg.MinIOBucket, inst.Prefix, err)
			}
		}
		if inst.Client != nil {
			_ = inst.Client.Close()
		}
	})

	if err := waitForServing(addr, cfg.BootTimeout); err != nil {
		inst.Stop()
		t.Fatalf("rag-engine never became SERVING on %s within %s: %v", addr, cfg.BootTimeout, err)
	}

	client, err := ragclient.New(context.Background(), ragclient.Config{
		Endpoint:     addr,
		SharedSecret: secret,
		DialTimeout:  5 * time.Second,
		MaxRetries:   0,
	})
	if err != nil {
		inst.Stop()
		t.Fatalf("dial rag-engine at %s: %v", addr, err)
	}
	inst.Client = client

	return inst
}

// applyDefaults fills in any zero-value RagEngineConfig fields.
func applyDefaults(cfg RagEngineConfig) RagEngineConfig {
	if cfg.Embedder == "" {
		cfg.Embedder = "fake"
	}
	if cfg.Reranker == "" {
		cfg.Reranker = "fake"
	}
	if cfg.EmbeddingDim == 0 {
		cfg.EmbeddingDim = 2560
	}
	if cfg.MinIO.Endpoint == "" {
		cfg.MinIO = DefaultMinIOConfig()
	}
	if cfg.MinIOBucket == "" {
		cfg.MinIOBucket = DefaultMinIOBucket
	}
	if cfg.LancePrefix == "" {
		cfg.LancePrefix = "tests/" + uuid.New().String() + "/"
	}
	if cfg.BootTimeout == 0 {
		cfg.BootTimeout = 30 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}
	// Let env override branch/binary when the caller didn't explicitly
	// request a specific target. This is the documented escape hatch
	// for running the E2E suite against a sibling worktree that has 2F
	// wired.
	if cfg.BranchOrBinary == "" {
		if override := os.Getenv("RAG_ENGINE_BRANCH"); override != "" {
			cfg.BranchOrBinary = override
		}
	}
	return cfg
}

// buildChildEnv assembles the env slice the Rust binary is exec'd with.
// We start from the parent's environment, strip anything that could
// bleed in real-provider credentials (so a developer with
// SILICONFLOW_API_KEY in their shell never accidentally gets billed by
// running `go test`), then layer our controlled variables on top.
//
// ExtraEnv always wins — tests that want real-provider runs can set the
// credentials there explicitly, and the plan documents this as opt-in.
func buildChildEnv(cfg RagEngineConfig, addr, secret string) []string {
	stripped := map[string]struct{}{}
	for _, k := range envBlocklist() {
		stripped[k] = struct{}{}
	}

	out := make([]string, 0, len(os.Environ())+20)
	for _, kv := range os.Environ() {
		if eq := indexByte(kv, '='); eq >= 0 {
			k := kv[:eq]
			if _, drop := stripped[k]; drop {
				continue
			}
		}
		out = append(out, kv)
	}

	controlled := map[string]string{
		"RAG_ENGINE_LISTEN_ADDR":   addr,
		"RAG_ENGINE_SHARED_SECRET": secret,
		"RAG_ENGINE_LOG_LEVEL":     "warn",

		// Embedder + reranker selection. Plan §2F documents both the
		// flag and the env-var path; we use env vars because they
		// compose better with the existing `figment` loader in the Rust
		// `Config::from_env` path.
		"LLM_PROVIDER":      cfg.Embedder,
		"LLM_EMBEDDING_DIM": fmt.Sprintf("%d", cfg.EmbeddingDim),
		"RERANKER_KIND":     cfg.Reranker,

		// LanceDB / MinIO wiring.
		"LANCE_S3_URI":            fmt.Sprintf("s3://%s/%s", cfg.MinIOBucket, cfg.LancePrefix),
		"LANCE_S3_ENDPOINT":       cfg.MinIO.Endpoint,
		"LANCE_S3_REGION":         cfg.MinIO.Region,
		"LANCE_ACCESS_KEY_ID":     cfg.MinIO.AccessKey,
		"LANCE_SECRET_ACCESS_KEY": cfg.MinIO.SecretKey,
		"LANCE_S3_ALLOW_HTTP":     "true",
	}
	for k, v := range controlled {
		out = append(out, k+"="+v)
	}
	for k, v := range cfg.ExtraEnv {
		out = append(out, k+"="+v)
	}
	return out
}

// envBlocklist lists env vars we never let bleed from the host into the
// child. They'd otherwise force the Rust embedder/reranker into a
// real-provider mode and potentially incur paid API calls.
func envBlocklist() []string {
	return []string{
		"LLM_PROVIDER",
		"LLM_EMBEDDING_DIM",
		"LLM_API_KEY",
		"LLM_API_URL",
		"LLM_MODEL",
		"LLM_ID",
		"LLM_MAX_INPUT_TOKENS",
		"RERANKER_KIND",
		"RERANKER_API_KEY",
		"RERANKER_BASE_URL",
		"RERANKER_MODEL",
		"SILICONFLOW_API_KEY",
		"SILICONFLOW_BASE_URL",
		"LANCE_S3_URI",
		"LANCE_S3_ENDPOINT",
		"LANCE_S3_REGION",
		"LANCE_ACCESS_KEY_ID",
		"LANCE_SECRET_ACCESS_KEY",
		"LANCE_S3_ALLOW_HTTP",
		"RAG_ENGINE_LISTEN_ADDR",
		"RAG_ENGINE_SHARED_SECRET",
		"RAG_ENGINE_LOG_LEVEL",
		"RAG_ENGINE_CONFIG",
		"RAG_ENGINE_METRICS_ADDR",
	}
}

// indexByte is a tiny helper to avoid importing strings just for IndexByte.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// pickFreePort asks the kernel for an ephemeral loopback port, closes
// the listener, and returns the address. There's a micro-race between
// close and the Rust server's bind — in practice this has been reliable
// on both dev laptops and CI.
func pickFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}
	return addr
}

// mustRandomHex returns `n` random bytes encoded as a 2n-char hex
// string. Used for per-test shared secrets.
func mustRandomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return hex.EncodeToString(b)
}

// waitForServing polls grpc.health.v1 until the server reports SERVING
// or the deadline passes. Health is intentionally unauthenticated so
// probes don't require the shared secret.
func waitForServing(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for SERVING after %s", timeout)
		}
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			hc := grpc_health_v1.NewHealthClient(conn)
			resp, hcErr := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
			cancel()
			_ = conn.Close()
			if hcErr == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// LanceURI returns the full `s3://bucket/prefix` URI this instance is
// configured to use. Exported for tests that want to assert on it.
func (r *RagEngineInstance) LanceURI() string {
	return fmt.Sprintf("s3://%s/%s", r.Bucket, r.Prefix)
}

// ResolveBinaryForBranch is a convenience wrapper tests may call to
// pre-validate that a branch override actually produces a buildable
// target before running full E2E flow.
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

