package ragclient

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

const testSecret = "phase-2i-shared-secret-donotreuse"

// mustNewClient is a small helper: connects with the shared secret,
// registers Close on cleanup, and fails the test on construction error.
func mustNewClient(t *testing.T, addr, secret string, maxRetries int) *Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := New(ctx, Config{
		Endpoint:     addr,
		SharedSecret: secret,
		DialTimeout:  5 * time.Second,
		MaxRetries:   maxRetries,
	})
	if err != nil {
		t.Fatalf("New client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestClient_Health_ReachesRunningServer proves the happy-path wiring:
// we can build a client against the real Rust binary, pass the shared
// secret, and receive SERVING.
func TestClient_Health_ReachesRunningServer(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING", resp.GetStatus())
	}
}

// TestClient_Health_ReturnsErrorWhenServerDown checks that tearing the
// server down produces a real error surface (UNAVAILABLE or
// ErrCircuitOpen) after retries.
func TestClient_Health_ReturnsErrorWhenServerDown(t *testing.T) {
	addr, shutdown := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 1)

	shutdown()
	time.Sleep(200 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := client.Health(ctx)
	if err == nil {
		t.Fatal("expected error when server is down, got nil")
	}
	if !errors.Is(err, ErrCircuitOpen) && status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v (code=%v), want UNAVAILABLE or ErrCircuitOpen", err, status.Code(err))
	}
}

// TestClient_Auth_RejectsWrongSecret verifies the Rust interceptor
// rejects a bad token as UNAUTHENTICATED.
func TestClient_Auth_RejectsWrongSecret(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, "wrong-secret-value-xxxxxxxxxxxxxx", 0)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{
		DatasetName: "x", VectorDim: 1, EmbeddingPrecision: "float32", IdempotencyKey: "k",
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("err code = %v, want Unauthenticated (err=%v)", status.Code(err), err)
	}
}

// TestClient_Auth_AcceptsCorrectSecret: any response other than
// UNAUTHENTICATED proves the auth interceptor passed the request through
// to the handler. (Pre-2F this always returned UNIMPLEMENTED from the
// 2A stub; post-2F the handler actually runs and may return OK or
// another code depending on handler logic.)
func TestClient_Auth_AcceptsCorrectSecret(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{
		DatasetName: "x", VectorDim: 1, EmbeddingPrecision: "float32", IdempotencyKey: "k",
	})
	if status.Code(err) == codes.Unauthenticated {
		t.Fatalf("auth unexpectedly rejected correct secret: err=%v", err)
	}
}

// TestClient_IngestBatch_RetryOnUnavailable_Succeeds demonstrates the
// retry loop against a real server lifecycle: stop the server so the
// first RPC attempt fails with UNAVAILABLE, respawn mid-backoff, then
// assert the final attempt lands on a live server (currently
// UNIMPLEMENTED — that proves the RPC handler was reached after
// retries).
func TestClient_IngestBatch_RetryOnUnavailable_Succeeds(t *testing.T) {
	addr, shutdown := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 5)

	// Kill the first-generation server.
	shutdown()
	time.Sleep(150 * time.Millisecond)

	// Schedule a respawn on the same port that lands inside the
	// client's exponential backoff window (100/200/400ms + jitter).
	respawnDone := make(chan struct{})
	go func() {
		defer close(respawnDone)
		time.Sleep(300 * time.Millisecond)
		stop := startFixedAddrServer(t, addr, testSecret)
		_ = stop // cleanup registered inside helper
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       "ds",
		OrgId:             "org",
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "stable-key",
		DeclaredVectorDim: 1,
	})
	<-respawnDone

	// Retry succeeded iff the final attempt reached the respawned server.
	// Anything that's NOT `Unavailable` / `DeadlineExceeded` proves that —
	// including NotFound (no such dataset), InvalidArgument (dim mismatch),
	// or OK. We only fail on transport-level codes, which would mean the
	// retry loop never landed on a live server.
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		t.Fatalf("retry never reached the respawned server: err=%v", err)
	}
}

// TestClient_IngestBatch_NoRetryOnInvalidArgument: since the
// rag-engine stub currently returns UNIMPLEMENTED, we drive the retry
// engine directly with a synthetic INVALID_ARGUMENT to confirm it
// stops after exactly one attempt — mirroring what will happen when
// the real server validates `declared_vector_dim`.
func TestClient_IngestBatch_NoRetryOnInvalidArgument(t *testing.T) {
	calls := 0
	err := runWithRetry(context.Background(), idempotentPolicy(5), newTestRand(), func(_ context.Context) error {
		calls++
		return status.Error(codes.InvalidArgument, "declared_vector_dim mismatch")
	})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on INVALID_ARGUMENT)", calls)
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

// TestClient_IngestBatch_RequiresIdempotencyKey asserts the client-side
// guardrail: callers must supply a stable key.
func TestClient_IngestBatch_RequiresIdempotencyKey(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.IngestBatch(ctx, &ragpb.IngestBatchRequest{DatasetName: "d"})
	if err == nil || !strings.Contains(err.Error(), "idempotency_key") {
		t.Fatalf("want idempotency_key error, got %v", err)
	}
}

// TestClient_Search_TimeoutHonored uses an already-expired context to
// prove the deadline propagates client-side. Documented limitation:
// the current Rust stub cannot be coerced into delaying its response,
// so a server-side slow path is exercised elsewhere once real
// handlers ship.
func TestClient_Search_TimeoutHonored(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 0)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	_, err := client.Search(ctx, &ragpb.SearchRequest{DatasetName: "d", OrgId: "o", QueryText: "q", Limit: 1})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if status.Code(err) != codes.DeadlineExceeded && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v (code=%v), want DeadlineExceeded", err, status.Code(err))
	}
}

// TestClient_CircuitBreaker_OpensAfterConsecutiveFailures points the
// client at a port where nothing is listening after a brief healthy
// window (so New() succeeds), then fires enough RPCs to trip the
// breaker and asserts the next call short-circuits with ErrCircuitOpen.
func TestClient_CircuitBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	addr, shutdown := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 0) // 0 retries → 1 attempt per call

	shutdown()
	time.Sleep(200 * time.Millisecond)

	for i := 0; i < int(breakerConsecutiveFailThreshold); i++ {
		cctx, cc := context.WithTimeout(context.Background(), 800*time.Millisecond)
		_, _ = client.Health(cctx)
		cc()
	}

	cctx, cc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cc()
	_, err := client.Health(cctx)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
}

// TestAuthInterceptor_SetsBearerHeader verifies the outgoing metadata
// carries `authorization: Bearer <secret>`. We dial a tiny in-test
// gRPC server with an interceptor that captures metadata. This is not
// a mock of the rag-engine — it's an inspection of what the auth
// interceptor emits.
func TestAuthInterceptor_SetsBearerHeader(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	got := make(chan string, 1)
	srv := grpc.NewServer(grpc.UnaryInterceptor(func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		_ grpc.UnaryHandler,
	) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		vals := md.Get(authMetadataKey)
		if len(vals) == 0 {
			got <- ""
		} else {
			got <- vals[0]
		}
		return nil, status.Error(codes.Unimplemented, "echo-done")
	}))
	grpc_health_v1.RegisterHealthServer(srv, stubHealth{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := New(ctx, Config{
		Endpoint:     addr,
		SharedSecret: "echo-secret",
		DialTimeout:  3 * time.Second,
		MaxRetries:   0,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	_, _ = client.Health(ctx)
	select {
	case v := <-got:
		if v != "Bearer echo-secret" {
			t.Fatalf("auth header = %q, want %q", v, "Bearer echo-secret")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no metadata captured")
	}
}

// TestAuthInterceptor_PreservesExistingHeader — if the caller injected
// their own authorization metadata (e.g. for a negative-auth test),
// the interceptor must not clobber it.
func TestAuthInterceptor_PreservesExistingHeader(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	got := make(chan string, 1)
	srv := grpc.NewServer(grpc.UnaryInterceptor(func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		_ grpc.UnaryHandler,
	) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		got <- md.Get(authMetadataKey)[0]
		return nil, status.Error(codes.Unimplemented, "echo-done")
	}))
	grpc_health_v1.RegisterHealthServer(srv, stubHealth{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	ctx := metadata.AppendToOutgoingContext(context.Background(), authMetadataKey, "Bearer caller-override")
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	client, err := New(ctx, Config{
		Endpoint:     addr,
		SharedSecret: "new-server-secret",
		DialTimeout:  3 * time.Second,
		MaxRetries:   0,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	_, _ = client.Health(ctx)
	select {
	case v := <-got:
		if v != "Bearer caller-override" {
			t.Fatalf("auth header = %q, want caller-supplied value to win", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no metadata captured")
	}
}

// TestClient_Close_IsSafeToCallTwice exercises the double-close branch
// in Close().
func TestClient_Close_IsSafeToCallTwice(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := New(ctx, Config{Endpoint: addr, SharedSecret: testSecret, DialTimeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close must be no-op, got %v", err)
	}
}

// TestNew_FailsFastOnBadEndpoint ensures the dial error path is
// surfaced to the caller within DialTimeout.
func TestNew_FailsFastOnBadEndpoint(t *testing.T) {
	// A reserved-for-documentation address that will refuse to connect.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := New(ctx, Config{
		Endpoint:     "127.0.0.1:1", // guaranteed no listener
		SharedSecret: "s",
		DialTimeout:  500 * time.Millisecond,
		MaxRetries:   0,
	})
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
}

// TestNew_ReturnsErrorOnInvalidConfig walks the validation branches.
func TestNew_ReturnsErrorOnInvalidConfig(t *testing.T) {
	_, err := New(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

// TestNew_FailsOnMalformedEndpoint covers the grpc.NewClient error
// branch. grpc.NewClient rejects only a handful of inputs at
// construction time (malformed URL escapes among them), so we use
// "%%%" — a reliable trigger that makes the DNS resolver URL parser
// reject the target.
func TestNew_FailsOnMalformedEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := New(ctx, Config{
		Endpoint:     "%%%",
		SharedSecret: "s",
		DialTimeout:  500 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected construction error for malformed endpoint")
	}
}

// TestNew_AppliesDefaultDialTimeoutWhenZero exercises the DialTimeout
// defaulting branch in dialEngine. We pass DialTimeout=0 against a
// valid local stub server, so the default is applied and dial succeeds.
func TestNew_AppliesDefaultDialTimeoutWhenZero(t *testing.T) {
	addr := startLocalOkServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := New(ctx, Config{
		Endpoint:     addr,
		SharedSecret: "s",
		DialTimeout:  0, // triggers the default-apply branch
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
}

// --- helpers -----------------------------------------------------------

// startFixedAddrServer spawns a rag-engine instance bound to the given
// addr (port must be free) and waits for SERVING. Cleanup registered
// on t. Returns a manual shutdown for tests that need to stop early.
func startFixedAddrServer(t *testing.T, addr, secret string) func() {
	t.Helper()
	bin, err := buildRagEngineBinary()
	if err != nil {
		t.Fatalf("build rag-engine: %v", err)
	}
	cmd := exec.Command(bin)
	baseEnv := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		upper := strings.ToUpper(kv)
		switch {
		case strings.HasPrefix(upper, "LLM_"),
			strings.HasPrefix(upper, "RERANKER_"),
			strings.HasPrefix(upper, "LANCE_"),
			strings.HasPrefix(upper, "SILICONFLOW_"),
			strings.HasPrefix(upper, "RAG_ENGINE_"):
			continue
		}
		baseEnv = append(baseEnv, kv)
	}
	cmd.Env = append(baseEnv,
		"RAG_ENGINE_LISTEN_ADDR="+addr,
		"RAG_ENGINE_SHARED_SECRET="+secret,
		"RAG_ENGINE_LOG_LEVEL=warn",
		"LLM_PROVIDER=fake",
		"LLM_EMBEDDING_DIM=2560",
		"RERANKER_KIND=fake",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fixed-addr rag-engine: %v", err)
	}
	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
			waited := make(chan struct{})
			go func() { _ = cmd.Wait(); close(waited) }()
			select {
			case <-waited:
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
				<-waited
			}
		})
	}
	t.Cleanup(shutdown)
	if err := waitForServing(t, addr, 10*time.Second); err != nil {
		shutdown()
		t.Fatalf("fixed-addr rag-engine never SERVING on %s: %v", addr, err)
	}
	return shutdown
}

// stubHealth is a minimal health server that always reports SERVING.
type stubHealth struct {
	grpc_health_v1.UnimplementedHealthServer
}

func (stubHealth) Check(context.Context, *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}
