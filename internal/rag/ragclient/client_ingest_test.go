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
