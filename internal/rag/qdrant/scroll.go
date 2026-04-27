package qdrant

import (
	"context"

	qc "github.com/qdrant/go-client/qdrant"
)

type ScrollRequest struct {
	Collection  string
	Filter      *qc.Filter
	Limit       uint32
	Offset      *qc.PointId
	WithPayload bool
}

type ScrolledPoint struct {
	ID      string
	Payload map[string]any
}

type ScrollPage struct {
	Points     []ScrolledPoint
	NextOffset *qc.PointId
}

func (c *Client) Scroll(ctx context.Context, req ScrollRequest) (*ScrollPage, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 256
	}
	resp, err := c.c.GetPointsClient().Scroll(ctx, &qc.ScrollPoints{
		CollectionName: req.Collection,
		Filter:         req.Filter,
		Limit:          qc.PtrOf(limit),
		Offset:         req.Offset,
		WithPayload:    qc.NewWithPayload(req.WithPayload),
		WithVectors:    qc.NewWithVectors(false),
	})
	if err != nil {
		return nil, err
	}
	pts := resp.GetResult()
	out := make([]ScrolledPoint, len(pts))
	for i, p := range pts {
		out[i] = ScrolledPoint{
			ID:      pointIDString(p.GetId()),
			Payload: fromValueMap(p.GetPayload()),
		}
	}
	return &ScrollPage{Points: out, NextOffset: resp.GetNextPageOffset()}, nil
}
