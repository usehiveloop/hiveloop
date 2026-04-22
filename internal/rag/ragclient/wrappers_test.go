package ragclient

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

// TestClient_AllRPCs_ReachServer round-trips every unary RPC on the
// Client against the Tranche-2A stub. Each should surface UNIMPLEMENTED
// from the server, which proves: auth passed, deadline applied, the
// generated gRPC stub was dispatched, and our error path returns
// correctly. Coverage target for all wrapper funcs.
func TestClient_AllRPCs_ReachServer(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cases := []struct {
		name string
		call func() error
	}{
		{"CreateDataset", func() error {
			_, err := client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{
				DatasetName: "d", VectorDim: 1, EmbeddingPrecision: "float32", IdempotencyKey: "k",
			})
			return err
		}},
		{"DropDataset", func() error {
			_, err := client.DropDataset(ctx, &ragpb.DropDatasetRequest{DatasetName: "d", Confirm: true})
			return err
		}},
		{"IngestBatch", func() error {
			_, err := client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
				DatasetName:       "d",
				OrgId:             "o",
				Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
				IdempotencyKey:    "k",
				DeclaredVectorDim: 1,
			})
			return err
		}},
		{"UpdateACL", func() error {
			_, err := client.UpdateACL(ctx, &ragpb.UpdateACLRequest{
				DatasetName:    "d",
				OrgId:          "o",
				IdempotencyKey: "k",
			})
			return err
		}},
		{"Search", func() error {
			_, err := client.Search(ctx, &ragpb.SearchRequest{
				DatasetName: "d", OrgId: "o", QueryText: "q", Limit: 1,
			})
			return err
		}},
		{"DeleteByDocID", func() error {
			_, err := client.DeleteByDocID(ctx, &ragpb.DeleteByDocIDRequest{
				DatasetName: "d", OrgId: "o", DocIds: []string{"x"}, IdempotencyKey: "k",
			})
			return err
		}},
		{"DeleteByOrg", func() error {
			// Use a very tight deadline override so we don't actually
			// wait 30 minutes on the default for this RPC.
			tightCtx, tc := context.WithTimeout(context.Background(), 2*time.Second)
			defer tc()
			_, err := client.DeleteByOrg(tightCtx, &ragpb.DeleteByOrgRequest{
				OrgId: "o", DatasetNames: []string{"d"}, Confirm: true, IdempotencyKey: "k",
			})
			return err
		}},
		{"Prune", func() error {
			_, err := client.Prune(ctx, &ragpb.PruneRequest{
				DatasetName: "d", OrgId: "o", KeepDocIds: []string{"x"}, IdempotencyKey: "k",
			})
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if status.Code(err) != codes.Unimplemented {
				t.Fatalf("%s: err code = %v, want Unimplemented (err=%v)", tc.name, status.Code(err), err)
			}
		})
	}
}

// TestClient_CallerDeadlineWinsWhenShorter covers the applyDeadline
// branch where an existing context deadline is tighter than the
// per-RPC default.
func TestClient_CallerDeadlineWinsWhenShorter(t *testing.T) {
	addr, _ := startRagEngine(t, testSecret)
	client := mustNewClient(t, addr, testSecret, 0)

	// 100ms deadline; DeleteByOrg default is 30min — caller MUST win.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = client.DeleteByOrg(ctx, &ragpb.DeleteByOrgRequest{
		OrgId: "o", DatasetNames: []string{"d"}, Confirm: true, IdempotencyKey: "k",
	})
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("elapsed = %v, want ≤ ~100ms-plus-overhead (caller deadline must win)", elapsed)
	}
}
