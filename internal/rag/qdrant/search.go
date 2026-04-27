package qdrant

import (
	"context"
	"net/http"
)

type SearchRequest struct {
	Collection  string
	Vector      []float32
	Filter      map[string]any
	Limit       uint32
	HNSWEf      uint32
	WithPayload bool
}

type Hit struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

type queryEnvelope struct {
	Result struct {
		Points []Hit `json:"points"`
	} `json:"result"`
}

func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Hit, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 10
	}
	ef := req.HNSWEf
	if ef == 0 {
		ef = 64
	}
	body := map[string]any{
		"query":        req.Vector,
		"limit":        limit,
		"with_payload": req.WithPayload,
		"with_vector":  false,
		"params":       map[string]any{"hnsw_ef": ef, "exact": false},
	}
	if req.Filter != nil {
		body["filter"] = req.Filter
	}
	var out queryEnvelope
	if err := c.do(ctx, http.MethodPost,
		"/collections/"+req.Collection+"/points/query", body, &out); err != nil {
		return nil, err
	}
	return out.Result.Points, nil
}

// BuildACLFilter expresses: org_id == X AND (is_public OR acl matches any of [...]).
// bypassACL drops the (is_public OR acl) clause, keeping only org partition.
func BuildACLFilter(orgID string, aclAnyOf []string, bypassACL bool) map[string]any {
	must := []map[string]any{
		{"key": "org_id", "match": map[string]any{"value": orgID}},
	}
	if bypassACL {
		return map[string]any{"must": must}
	}
	should := []map[string]any{
		{"key": "is_public", "match": map[string]any{"value": true}},
	}
	if len(aclAnyOf) > 0 {
		anyAcl := make([]any, len(aclAnyOf))
		for i, v := range aclAnyOf {
			anyAcl[i] = v
		}
		should = append(should, map[string]any{
			"key":   "acl",
			"match": map[string]any{"any": anyAcl},
		})
	}
	must = append(must, map[string]any{"should": should})
	return map[string]any{"must": must}
}
