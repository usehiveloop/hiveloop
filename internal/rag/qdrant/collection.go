package qdrant

import (
	"context"
	"strings"

	qc "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CollectionConfig struct {
	Name      string
	VectorDim uint32
	OnDisk    bool
}

func (c *Client) CollectionExists(ctx context.Context, name string) (bool, error) {
	exists, err := c.c.CollectionExists(ctx, name)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (c *Client) CreateCollection(ctx context.Context, cfg CollectionConfig) error {
	return c.c.CreateCollection(ctx, &qc.CreateCollection{
		CollectionName: cfg.Name,
		VectorsConfig: qc.NewVectorsConfig(&qc.VectorParams{
			Size:     uint64(cfg.VectorDim),
			Distance: qc.Distance_Cosine,
			OnDisk:   qc.PtrOf(cfg.OnDisk),
		}),
		OnDiskPayload: qc.PtrOf(cfg.OnDisk),
		HnswConfig: &qc.HnswConfigDiff{
			M:           qc.PtrOf(uint64(16)),
			EfConstruct: qc.PtrOf(uint64(200)),
		},
	})
}

func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	if err := c.c.DeleteCollection(ctx, name); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) CreatePayloadIndex(ctx context.Context, collection, fieldName string, fieldType qc.FieldType, params *qc.PayloadIndexParams) error {
	req := &qc.CreateFieldIndexCollection{
		CollectionName:   collection,
		FieldName:        fieldName,
		FieldType:        qc.PtrOf(fieldType),
		FieldIndexParams: params,
	}
	_, err := c.c.CreateFieldIndex(ctx, req)
	if err != nil && !isAlreadyExists(err) {
		return err
	}
	return nil
}

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
	keywordTenantParams := qc.NewPayloadIndexParamsKeyword(&qc.KeywordIndexParams{
		IsTenant: qc.PtrOf(true),
	})
	indices := []struct {
		name      string
		fieldType qc.FieldType
		params    *qc.PayloadIndexParams
	}{
		{"org_id", qc.FieldType_FieldTypeKeyword, keywordTenantParams},
		{"acl", qc.FieldType_FieldTypeKeyword, nil},
		{"doc_id", qc.FieldType_FieldTypeKeyword, nil},
		{"rag_source_id", qc.FieldType_FieldTypeKeyword, nil},
		{"is_public", qc.FieldType_FieldTypeBool, nil},
		{"doc_updated_at", qc.FieldType_FieldTypeInteger, nil},
	}
	for _, idx := range indices {
		if err := c.CreatePayloadIndex(ctx, cfg.Name, idx.name, idx.fieldType, idx.params); err != nil {
			return err
		}
	}
	return nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.AlreadyExists {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate")
}
