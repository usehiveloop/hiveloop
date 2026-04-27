package qdrant

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net/http"
)

type Point struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type upsertEnvelope struct {
	Result struct {
		OperationID uint64 `json:"operation_id"`
		Status      string `json:"status"`
	} `json:"result"`
}

func (c *Client) Upsert(ctx context.Context, collection string, points []Point, wait bool) error {
	if len(points) == 0 {
		return nil
	}
	body := map[string]any{"points": points}
	q := "?wait=false"
	if wait {
		q = "?wait=true"
	}
	var out upsertEnvelope
	return c.do(ctx, http.MethodPut, "/collections/"+collection+"/points"+q, body, &out)
}

type Filter struct {
	Must    []FilterClause `json:"must,omitempty"`
	Should  []FilterClause `json:"should,omitempty"`
	MustNot []FilterClause `json:"must_not,omitempty"`
}

type FilterClause struct {
	Key   string      `json:"key,omitempty"`
	Match *MatchValue `json:"match,omitempty"`
}

type MatchValue struct {
	Value any   `json:"value,omitempty"`
	Any   []any `json:"any,omitempty"`
}

func (c *Client) SetPayload(ctx context.Context, collection string, ids []string, payload map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	body := map[string]any{
		"points":  ids,
		"payload": payload,
	}
	return c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/payload?wait=true", body, nil)
}

func (c *Client) DeleteByFilter(ctx context.Context, collection string, filter Filter) error {
	body := map[string]any{"filter": filter}
	return c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", body, nil)
}

func (c *Client) DeleteByIDs(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	body := map[string]any{"points": ids}
	return c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", body, nil)
}

type countEnvelope struct {
	Result struct {
		Count uint64 `json:"count"`
	} `json:"result"`
}

func (c *Client) Count(ctx context.Context, collection string, filter *Filter) (uint64, error) {
	body := map[string]any{"exact": true}
	if filter != nil {
		body["filter"] = filter
	}
	var out countEnvelope
	if err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/count", body, &out); err != nil {
		return 0, err
	}
	return out.Result.Count, nil
}

// Stable UUIDv5 from (org_id, source_id, doc_id). Re-ingesting the same doc
// under the same source upserts in place; a doc shared by two sources gets
// two points.
func PointID(orgID, sourceID, docID string) string {
	h := sha1.Sum([]byte(orgID + "::" + sourceID + "::" + docID))
	h[6] = (h[6] & 0x0f) | 0x50
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(h[0:4]),
		binary.BigEndian.Uint16(h[4:6]),
		binary.BigEndian.Uint16(h[6:8]),
		binary.BigEndian.Uint16(h[8:10]),
		h[10:16])
}
