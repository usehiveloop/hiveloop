package ragclient

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

// okRagServer is a test gRPC server that implements every RagEngine RPC
// by returning an empty-but-successful response. We use this to cover
// the "nil err → return out, nil" branch in every client wrapper,
// which the real Tranche-2A binary cannot produce (it returns
// UNIMPLEMENTED for every RPC).
//
// This is NOT a mock of the rag-engine — it's a reference implementation
// used to verify the Go wrapper layer's own behavior. The real binary
// is still exercised by every other integration test in this package.
type okRagServer struct {
	ragpb.UnimplementedRagEngineServer
}

func (okRagServer) CreateDataset(context.Context, *ragpb.CreateDatasetRequest) (*ragpb.CreateDatasetResponse, error) {
	return &ragpb.CreateDatasetResponse{Created: true, SchemaOk: true}, nil
}
func (okRagServer) DropDataset(context.Context, *ragpb.DropDatasetRequest) (*ragpb.DropDatasetResponse, error) {
	return &ragpb.DropDatasetResponse{Dropped: true}, nil
}
func (okRagServer) IngestBatch(context.Context, *ragpb.IngestBatchRequest) (*ragpb.IngestBatchResponse, error) {
	return &ragpb.IngestBatchResponse{}, nil
}
func (okRagServer) UpdateACL(context.Context, *ragpb.UpdateACLRequest) (*ragpb.UpdateACLResponse, error) {
	return &ragpb.UpdateACLResponse{DocsUpdated: 1}, nil
}
func (okRagServer) Search(context.Context, *ragpb.SearchRequest) (*ragpb.SearchResponse, error) {
	return &ragpb.SearchResponse{}, nil
}
func (okRagServer) DeleteByDocID(context.Context, *ragpb.DeleteByDocIDRequest) (*ragpb.DeleteByDocIDResponse, error) {
	return &ragpb.DeleteByDocIDResponse{DocsDeleted: 1}, nil
}
func (okRagServer) DeleteByOrg(context.Context, *ragpb.DeleteByOrgRequest) (*ragpb.DeleteByOrgResponse, error) {
	return &ragpb.DeleteByOrgResponse{ChunksDeleted: 1}, nil
}
func (okRagServer) Prune(context.Context, *ragpb.PruneRequest) (*ragpb.PruneResponse, error) {
	return &ragpb.PruneResponse{DocsPruned: 1}, nil
}

// startLocalOkServer boots an in-test gRPC server backed by okRagServer
// plus the stub health implementation, returning the listen address.
func startLocalOkServer(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	ragpb.RegisterRagEngineServer(srv, okRagServer{})
	grpc_health_v1.RegisterHealthServer(srv, stubHealth{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

// TestClient_AllRPCs_HappyPath exercises the nil-error → return-out
// branch in every wrapper. The real rag-engine binary can't produce
// those paths on Tranche 2A because every handler returns UNIMPLEMENTED.
func TestClient_AllRPCs_HappyPath(t *testing.T) {
	addr := startLocalOkServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := New(ctx, Config{Endpoint: addr, SharedSecret: "s", DialTimeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if resp, err := client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{DatasetName: "d", VectorDim: 1, EmbeddingPrecision: "float32", IdempotencyKey: "k"}); err != nil || !resp.Created {
		t.Fatalf("CreateDataset: resp=%v err=%v", resp, err)
	}
	if resp, err := client.DropDataset(ctx, &ragpb.DropDatasetRequest{DatasetName: "d", Confirm: true}); err != nil || !resp.Dropped {
		t.Fatalf("DropDataset: resp=%v err=%v", resp, err)
	}
	if _, err := client.IngestBatch(ctx, &ragpb.IngestBatchRequest{DatasetName: "d", OrgId: "o", IdempotencyKey: "k", DeclaredVectorDim: 1}); err != nil {
		t.Fatalf("IngestBatch: err=%v", err)
	}
	if resp, err := client.UpdateACL(ctx, &ragpb.UpdateACLRequest{DatasetName: "d", OrgId: "o", IdempotencyKey: "k"}); err != nil || resp.DocsUpdated != 1 {
		t.Fatalf("UpdateACL: resp=%v err=%v", resp, err)
	}
	if _, err := client.Search(ctx, &ragpb.SearchRequest{DatasetName: "d", OrgId: "o", QueryText: "q", Limit: 1}); err != nil {
		t.Fatalf("Search: err=%v", err)
	}
	if resp, err := client.DeleteByDocID(ctx, &ragpb.DeleteByDocIDRequest{DatasetName: "d", OrgId: "o", DocIds: []string{"x"}, IdempotencyKey: "k"}); err != nil || resp.DocsDeleted != 1 {
		t.Fatalf("DeleteByDocID: resp=%v err=%v", resp, err)
	}
	if resp, err := client.DeleteByOrg(ctx, &ragpb.DeleteByOrgRequest{OrgId: "o", DatasetNames: []string{"d"}, Confirm: true, IdempotencyKey: "k"}); err != nil || resp.ChunksDeleted != 1 {
		t.Fatalf("DeleteByOrg: resp=%v err=%v", resp, err)
	}
	if resp, err := client.Prune(ctx, &ragpb.PruneRequest{DatasetName: "d", OrgId: "o", KeepDocIds: []string{"x"}, IdempotencyKey: "k"}); err != nil || resp.DocsPruned != 1 {
		t.Fatalf("Prune: resp=%v err=%v", resp, err)
	}
}

// TestClient_CallTimeoutOverride exercises the applyDeadline branch
// where Config.CallTimeout is explicitly set (overriding the per-RPC
// default). A very short CallTimeout forces DEADLINE_EXCEEDED against
// a deliberately unreachable peer.
func TestClient_CallTimeoutOverride(t *testing.T) {
	// Use the local OK server to get a valid connection, then call it
	// with a 1ms CallTimeout which will expire mid-RPC.
	addr := startLocalOkServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := New(ctx, Config{
		Endpoint:     addr,
		SharedSecret: "s",
		DialTimeout:  3 * time.Second,
		CallTimeout:  1 * time.Nanosecond, // effectively zero; RPC must time out
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Search(ctx, &ragpb.SearchRequest{DatasetName: "d", OrgId: "o", QueryText: "q", Limit: 1})
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
}
