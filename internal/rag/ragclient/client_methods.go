package ragclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

func (c *Client) CreateDataset(ctx context.Context, in *ragpb.CreateDatasetRequest) (*ragpb.CreateDatasetResponse, error) {
	var out *ragpb.CreateDatasetResponse
	err := c.invoke(ctx, "CreateDataset", idempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.CreateDataset(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DropDataset destroys a dataset (requires confirm=true server-side).
// Idempotent.
func (c *Client) DropDataset(ctx context.Context, in *ragpb.DropDatasetRequest) (*ragpb.DropDatasetResponse, error) {
	var out *ragpb.DropDatasetResponse
	err := c.invoke(ctx, "DropDataset", idempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.DropDataset(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// IngestBatch submits 500–1000 raw documents for chunk+embed+write.
// Idempotent IFF idempotency_key is stable per logical batch.
func (c *Client) IngestBatch(ctx context.Context, in *ragpb.IngestBatchRequest) (*ragpb.IngestBatchResponse, error) {
	if in != nil && in.IdempotencyKey == "" {
		return nil, fmt.Errorf("ragclient: IngestBatch requires a stable idempotency_key")
	}
	var out *ragpb.IngestBatchResponse
	err := c.invoke(ctx, "IngestBatch", idempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.IngestBatch(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateACL replaces ACLs for the listed doc_ids in bulk. Idempotent.
func (c *Client) UpdateACL(ctx context.Context, in *ragpb.UpdateACLRequest) (*ragpb.UpdateACLResponse, error) {
	var out *ragpb.UpdateACLResponse
	err := c.invoke(ctx, "UpdateACL", idempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.UpdateACL(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Search runs hybrid retrieval. NOT retried on DEADLINE_EXCEEDED because
// a timed-out request may have partially executed server-side; a
// duplicate search wastes embedding credits and bandwidth. Only
// UNAVAILABLE (nothing reached the service) is retried.
func (c *Client) Search(ctx context.Context, in *ragpb.SearchRequest) (*ragpb.SearchResponse, error) {
	var out *ragpb.SearchResponse
	err := c.invoke(ctx, "Search", nonIdempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.Search(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteByDocID deletes chunks for the listed doc_ids. Idempotent.
func (c *Client) DeleteByDocID(ctx context.Context, in *ragpb.DeleteByDocIDRequest) (*ragpb.DeleteByDocIDResponse, error) {
	var out *ragpb.DeleteByDocIDResponse
	err := c.invoke(ctx, "DeleteByDocID", idempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.DeleteByDocID(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteByOrg wipes all chunks for an org across the listed datasets.
// Server-side gated by confirm=true. Idempotent. Default deadline 30min.
func (c *Client) DeleteByOrg(ctx context.Context, in *ragpb.DeleteByOrgRequest) (*ragpb.DeleteByOrgResponse, error) {
	var out *ragpb.DeleteByOrgResponse
	err := c.invoke(ctx, "DeleteByOrg", idempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.DeleteByOrg(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Prune deletes doc_ids NOT in keep_doc_ids. Server refuses empty
// keep_doc_ids. Idempotent.
func (c *Client) Prune(ctx context.Context, in *ragpb.PruneRequest) (*ragpb.PruneResponse, error) {
	var out *ragpb.PruneResponse
	err := c.invoke(ctx, "Prune", idempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.rag.Prune(ctx, in)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Health probes the standard grpc.health.v1.Health service for
// "hiveloop.rag.v1.RagEngine". It uses a non-idempotent policy since
// probes are short and cheap to redo — retry only on UNAVAILABLE.
func (c *Client) Health(ctx context.Context) (*grpc_health_v1.HealthCheckResponse, error) {
	const grpcServiceName = "hiveloop.rag.v1.RagEngine"
	var out *grpc_health_v1.HealthCheckResponse
	err := c.invoke(ctx, "Health", nonIdempotentPolicy(c.cfg.MaxRetries), func(ctx context.Context) error {
		resp, err := c.health.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: grpcServiceName})
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
