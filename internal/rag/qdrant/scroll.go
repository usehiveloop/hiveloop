package qdrant

import (
	"context"
	"net/http"
)

type ScrollRequest struct {
	Collection  string
	Filter      map[string]any
	Limit       uint32
	Offset      any
	WithPayload bool
}

type ScrollPage struct {
	Points     []Hit
	NextOffset any
}

type scrollEnvelope struct {
	Result struct {
		Points     []Hit `json:"points"`
		NextOffset any   `json:"next_page_offset"`
	} `json:"result"`
}

func (c *Client) Scroll(ctx context.Context, req ScrollRequest) (*ScrollPage, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 256
	}
	body := map[string]any{
		"limit":        limit,
		"with_payload": req.WithPayload,
		"with_vector":  false,
	}
	if req.Filter != nil {
		body["filter"] = req.Filter
	}
	if req.Offset != nil {
		body["offset"] = req.Offset
	}
	var out scrollEnvelope
	if err := c.do(ctx, http.MethodPost,
		"/collections/"+req.Collection+"/points/scroll", body, &out); err != nil {
		return nil, err
	}
	return &ScrollPage{
		Points:     out.Result.Points,
		NextOffset: out.Result.NextOffset,
	}, nil
}
