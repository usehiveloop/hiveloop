package qdrant

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type CollectionConfig struct {
	Name      string
	VectorDim uint32
	OnDisk    bool
}

type collectionInfoEnvelope struct {
	Result struct {
		Status string `json:"status"`
	} `json:"result"`
}

func (c *Client) CollectionExists(ctx context.Context, name string) (bool, error) {
	var out collectionInfoEnvelope
	err := c.do(ctx, http.MethodGet, "/collections/"+name, nil, &out)
	if err == nil {
		return true, nil
	}
	// Qdrant returns 404 for missing collection.
	if isNotFound(err) {
		return false, nil
	}
	return false, err
}

func (c *Client) CreateCollection(ctx context.Context, cfg CollectionConfig) error {
	body := map[string]any{
		"vectors": map[string]any{
			"size":     cfg.VectorDim,
			"distance": "Cosine",
			"on_disk":  cfg.OnDisk,
		},
		"hnsw_config": map[string]any{
			"on_disk":       cfg.OnDisk,
			"m":             16,
			"ef_construct":  200,
		},
		"on_disk_payload": cfg.OnDisk,
	}
	return c.do(ctx, http.MethodPut, "/collections/"+cfg.Name, body, nil)
}

func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/collections/"+name, nil, nil)
	if err != nil && !isNotFound(err) {
		return err
	}
	return nil
}

type FieldSchema struct {
	Type     string `json:"type"`
	IsTenant bool   `json:"is_tenant,omitempty"`
}

func (c *Client) CreatePayloadIndex(ctx context.Context, collection, fieldName string, schema any) error {
	body := map[string]any{
		"field_name":   fieldName,
		"field_schema": schema,
	}
	return c.do(ctx, http.MethodPut, "/collections/"+collection+"/index?wait=true", body, nil)
}

// EnsureCollection creates the collection + standard payload indices if absent.
// Idempotent — returns nil for both first-time and repeat calls.
func (c *Client) EnsureCollection(ctx context.Context, cfg CollectionConfig) error {
	exists, err := c.CollectionExists(ctx, cfg.Name)
	if err != nil {
		return err
	}
	if !exists {
		if err := c.CreateCollection(ctx, cfg); err != nil {
			return err
		}
	}
	indices := []struct {
		name   string
		schema any
	}{
		{"org_id", FieldSchema{Type: "keyword", IsTenant: true}},
		{"acl", "keyword"},
		{"doc_id", "keyword"},
		{"rag_source_id", "keyword"},
		{"is_public", FieldSchema{Type: "bool"}},
		{"doc_updated_at", FieldSchema{Type: "integer"}},
	}
	for _, idx := range indices {
		if err := c.CreatePayloadIndex(ctx, cfg.Name, idx.name, idx.schema); err != nil {
			// Qdrant returns 200 for duplicate index creation; ignore "already exists" if it leaks through.
			if !isAlreadyExists(err) {
				return err
			}
		}
	}
	return nil
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "-> 404:")
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate")
}

var _ = errors.New
