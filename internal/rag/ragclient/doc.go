// Package ragclient is Hiveloop's Go-side wrapper around the Rust
// rag-engine gRPC service defined in proto/rag_engine.proto.
//
// The client owns a single long-lived *grpc.ClientConn (gRPC multiplexes
// requests over one HTTP/2 connection), a shared-secret bearer-token
// auth interceptor, per-RPC default deadlines, retry-with-backoff for
// idempotent RPCs, and a circuit breaker (sony/gobreaker).
//
// Wire transport is plaintext (insecure.NewCredentials()) because the
// engine lives on the private network per Phase 2 plan. Swap to TLS
// when the service is exposed externally.
//
// # Example
//
//	cfg := ragclient.Config{
//	    Endpoint:     "rag-engine.internal:50051",
//	    SharedSecret: os.Getenv("RAG_ENGINE_SHARED_SECRET"),
//	    DialTimeout:  5 * time.Second,
//	    CallTimeout:  0, // use per-RPC defaults
//	    MaxRetries:   3,
//	}
//	client, err := ragclient.New(ctx, cfg)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	resp, err := client.Health(ctx)
//	if err != nil {
//	    return err
//	}
//	_ = resp
package ragclient
