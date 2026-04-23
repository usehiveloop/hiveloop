package ragclient

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/sony/gobreaker/v2"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

// Config drives Client construction. All fields are validated by New.
type Config struct {
	// Endpoint is the gRPC target, e.g. "rag-engine.internal:50051".
	Endpoint string
	// SharedSecret is the bearer token the Rust server's auth
	// interceptor requires. Required.
	SharedSecret string
	// DialTimeout caps how long New waits for the initial connection
	// to reach READY state. Zero → 5s default.
	DialTimeout time.Duration
	// CallTimeout is a per-RPC fallback deadline. When > 0, it's
	// applied to any RPC that doesn't already have a context deadline.
	// Zero → use per-RPC defaults from perRPCDeadline (recommended).
	CallTimeout time.Duration
	// MaxRetries is the number of retry attempts on retryable errors
	// (UNAVAILABLE, and DEADLINE_EXCEEDED for idempotent RPCs). The
	// total attempts are MaxRetries+1. Zero disables retry.
	MaxRetries int
}

// validate returns nil if cfg is usable.
func (c Config) validate() error {
	if c.Endpoint == "" {
		return errors.New("ragclient: Config.Endpoint is required")
	}
	if c.SharedSecret == "" {
		return errors.New("ragclient: Config.SharedSecret is required")
	}
	if c.MaxRetries < 0 {
		return errors.New("ragclient: Config.MaxRetries must be >= 0")
	}
	return nil
}

// Client is the typed wrapper around the generated ragpb gRPC client.
// One Client owns one *grpc.ClientConn and one circuit breaker; it's
// safe for concurrent use.
type Client struct {
	cfg     Config
	conn    *grpc.ClientConn
	rag     ragpb.RagEngineClient
	health  grpc_health_v1.HealthClient
	breaker *gobreaker.CircuitBreaker[any]

	rngMu sync.Mutex
	rng   *rand.Rand
}

// New dials the engine and returns a ready-to-use Client. Returns an
// error if dial times out or the config is invalid.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	conn, err := dialEngine(ctx, cfg.Endpoint, cfg.DialTimeout, unaryAuthInterceptor(cfg.SharedSecret))
	if err != nil {
		return nil, err
	}
	return &Client{
		cfg:     cfg,
		conn:    conn,
		rag:     ragpb.NewRagEngineClient(conn),
		health:  grpc_health_v1.NewHealthClient(conn),
		breaker: newBreaker("ragclient:" + cfg.Endpoint),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// Close releases the underlying gRPC connection. Safe to call more than
// once (subsequent calls are no-ops).
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// perRPCDeadline is the default deadline for each RPC. Caller context
// deadlines, when shorter, always win. Values are tuned to the Phase 2
// plan §5 table.
func perRPCDeadline(rpc string) time.Duration {
	switch rpc {
	case "Search":
		return 10 * time.Second
	case "IngestBatch":
		// 10s SLO + 5s overhead for chunk+embed+write.
		return 15 * time.Second
	case "UpdateACL", "DeleteByDocID", "CreateDataset", "DropDataset":
		return 30 * time.Second
	case "Prune":
		return 5 * time.Minute
	case "DeleteByOrg":
		return 30 * time.Minute
	case "Health":
		return 2 * time.Second
	default:
		return 10 * time.Second
	}
}

// applyDeadline returns a ctx that honors whichever is SHORTER between
// the caller's existing deadline and the configured default. A cancel
// is always returned; callers must defer it.
func (c *Client) applyDeadline(ctx context.Context, rpc string) (context.Context, context.CancelFunc) {
	def := perRPCDeadline(rpc)
	if c.cfg.CallTimeout > 0 {
		def = c.cfg.CallTimeout
	}
	now := time.Now()
	defaultDeadline := now.Add(def)
	if existing, ok := ctx.Deadline(); ok && existing.Before(defaultDeadline) {
		// Caller already specified something tighter — honor it.
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, defaultDeadline)
}

// invoke is the single chokepoint every RPC wrapper funnels through.
// It layers breaker → retry → deadline around the raw gRPC call.
func (c *Client) invoke(
	ctx context.Context,
	rpc string,
	policy retryPolicy,
	call rpcCall,
) error {
	ctx, cancel := c.applyDeadline(ctx, rpc)
	defer cancel()

	_, err := c.breaker.Execute(func() (any, error) {
		return nil, runWithRetry(ctx, policy, c.safeRand(), call)
	})
	return mapBreakerError(err)
}

// safeRand returns a *rand.Rand guarded by the client mutex so the
// jitter path is race-free under concurrent RPCs.
func (c *Client) safeRand() *rand.Rand {
	c.rngMu.Lock()
	defer c.rngMu.Unlock()
	// Return a new derived source each time so the caller can use it
	// without holding the mutex for the duration of the RPC.
	return rand.New(rand.NewSource(c.rng.Int63()))
}

// --- RPC wrappers (one per service method) ----------------------------

// CreateDataset creates a new dataset. Idempotent — server dedupes via
// idempotency_key and returns ALREADY_EXISTS-compatible success when
// the dataset exists and the schema matches.
