package ragclient

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// authMetadataKey is the standard HTTP-style metadata header the Rust
// server's SharedSecretAuth interceptor reads. Value format:
// "Bearer <shared-secret>". See services/rag-engine/.../auth.rs.
const authMetadataKey = "authorization"

// unaryAuthInterceptor returns a grpc.UnaryClientInterceptor that
// attaches `authorization: Bearer <secret>` to every outbound RPC.
// The interceptor never rewrites an existing authorization header —
// tests that pass explicit metadata (wrong-secret tests) still win.
func unaryAuthInterceptor(secret string) grpc.UnaryClientInterceptor {
	bearer := "Bearer " + secret
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// If caller already set an authorization header in the outgoing
		// metadata, leave it alone — this keeps negative-auth tests
		// honest and lets callers override for impersonation flows.
		if md, ok := metadata.FromOutgoingContext(ctx); ok {
			if vals := md.Get(authMetadataKey); len(vals) > 0 {
				return invoker(ctx, method, req, reply, cc, opts...)
			}
		}
		ctx = metadata.AppendToOutgoingContext(ctx, authMetadataKey, bearer)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
