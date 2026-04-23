package ragclient

import (
	"context"
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
)

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
