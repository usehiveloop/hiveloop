package ragclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
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
