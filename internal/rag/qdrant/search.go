package qdrant

import (
	"context"

	qc "github.com/qdrant/go-client/qdrant"
)

type Filter = qc.Filter

type SearchRequest struct {
	Collection  string
	Vector      []float32
	Filter      *qc.Filter
	Limit       uint32
	HNSWEf      uint32
	WithPayload bool
}

type Hit struct {
	ID      string
	Score   float64
	Payload map[string]any
}

func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Hit, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 10
	}
	q := &qc.QueryPoints{
		CollectionName: req.Collection,
		Query:          qc.NewQueryDense(req.Vector),
		Filter:         req.Filter,
		Limit:          qc.PtrOf(uint64(limit)),
		WithPayload:    qc.NewWithPayload(req.WithPayload),
	}
	if req.HNSWEf > 0 {
		q.Params = &qc.SearchParams{HnswEf: qc.PtrOf(uint64(req.HNSWEf))}
	}
	pts, err := c.c.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]Hit, len(pts))
	for i, p := range pts {
		out[i] = Hit{
			ID:      pointIDString(p.GetId()),
			Score:   float64(p.GetScore()),
			Payload: fromValueMap(p.GetPayload()),
		}
	}
	return out, nil
}

// Express: org_id == X AND (is_public OR acl any-of [...]); bypassACL drops
// the (is_public OR acl) clause, keeping only the org partition.
func BuildACLFilter(orgID string, aclAnyOf []string, bypassACL bool) *qc.Filter {
	must := []*qc.Condition{qc.NewMatchKeyword("org_id", orgID)}
	if bypassACL {
		return &qc.Filter{Must: must}
	}
	should := []*qc.Condition{qc.NewMatchBool("is_public", true)}
	if len(aclAnyOf) > 0 {
		should = append(should, qc.NewMatchKeywords("acl", aclAnyOf...))
	}
	must = append(must, &qc.Condition{
		ConditionOneOf: &qc.Condition_Filter{Filter: &qc.Filter{Should: should}},
	})
	return &qc.Filter{Must: must}
}

// org_id == X AND rag_source_id == Y.
func BuildSourceFilter(orgID, sourceID string) *qc.Filter {
	return &qc.Filter{Must: []*qc.Condition{
		qc.NewMatchKeyword("org_id", orgID),
		qc.NewMatchKeyword("rag_source_id", sourceID),
	}}
}

func pointIDString(id *qc.PointId) string {
	if id == nil {
		return ""
	}
	if u := id.GetUuid(); u != "" {
		return u
	}
	if n := id.GetNum(); n != 0 {
		return ""
	}
	return ""
}
