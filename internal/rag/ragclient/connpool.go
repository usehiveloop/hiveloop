package ragclient

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// dialEngine establishes a single long-lived *grpc.ClientConn. gRPC
// internally multiplexes all RPCs over one HTTP/2 connection, so one
// conn is sufficient for throughput — a pool would only add complexity
// without buying anything on a loopback / single-endpoint deployment.
//
// Transport security: plaintext via insecure.NewCredentials(). The
// rag-engine runs on the private internal network per Phase 2 plan
// decision. Swap to TLS (credentials.NewTLS) when exposed externally.
//
// Keepalive: Time=30s, Timeout=10s matches typical proxy idle timeouts
// (60s+) and catches dead peers within 40s.
func dialEngine(
	ctx context.Context,
	endpoint string,
	dialTimeout time.Duration,
	unaryInterceptor grpc.UnaryClientInterceptor,
) (*grpc.ClientConn, error) {
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: false,
		}),
	}

	// grpc.NewClient is the v1.80+ idiomatic constructor; it dials lazily.
	// We force an initial connect attempt bounded by dialCtx so callers
	// get a fast error when the endpoint is wrong at startup rather
	// than on the first RPC. The blocking happens via WaitForStateChange.
	//
	// grpc.NewClient only fails on malformed options — it does NOT
	// validate the target string. A bad endpoint surfaces later via
	// WaitForStateChange returning false after dialCtx expires.
	conn, err := grpc.NewClient(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("ragclient: build grpc client for %s: %w", endpoint, err)
	}
	// Kick the connection off the Idle state so WaitForStateChange sees
	// real state transitions instead of sitting idle.
	conn.Connect()
	// Wait until the conn is READY, or dial context expires. If the
	// conn terminally fails (SHUTDOWN) WaitForStateChange returns false
	// and we fall out through the same error path.
	for {
		state := conn.GetState()
		if state.String() == "READY" {
			return conn, nil
		}
		if !conn.WaitForStateChange(dialCtx, state) {
			_ = conn.Close()
			return nil, fmt.Errorf("ragclient: dial to %s did not become READY within %s (last state=%s)", endpoint, dialTimeout, state)
		}
	}
}
