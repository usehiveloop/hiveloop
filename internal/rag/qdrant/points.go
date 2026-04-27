package qdrant

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	qc "github.com/qdrant/go-client/qdrant"
)

type Point struct {
	ID      string
	Vector  []float32
	Payload map[string]any
}

func (c *Client) Upsert(ctx context.Context, collection string, points []Point, wait bool) error {
	if len(points) == 0 {
		return nil
	}
	pts := make([]*qc.PointStruct, len(points))
	for i, p := range points {
		pts[i] = &qc.PointStruct{
			Id:      qc.NewID(p.ID),
			Vectors: qc.NewVectors(p.Vector...),
			Payload: toValueMap(p.Payload),
		}
	}
	_, err := c.c.Upsert(ctx, &qc.UpsertPoints{
		CollectionName: collection,
		Points:         pts,
		Wait:           qc.PtrOf(wait),
	})
	return err
}

func (c *Client) SetPayload(ctx context.Context, collection string, ids []string, payload map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	pids := make([]*qc.PointId, len(ids))
	for i, s := range ids {
		pids[i] = qc.NewID(s)
	}
	_, err := c.c.SetPayload(ctx, &qc.SetPayloadPoints{
		CollectionName: collection,
		PointsSelector: qc.NewPointsSelector(pids...),
		Payload:        toValueMap(payload),
		Wait:           qc.PtrOf(true),
	})
	return err
}

func (c *Client) DeleteByIDs(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pids := make([]*qc.PointId, len(ids))
	for i, s := range ids {
		pids[i] = qc.NewID(s)
	}
	_, err := c.c.Delete(ctx, &qc.DeletePoints{
		CollectionName: collection,
		Points:         qc.NewPointsSelector(pids...),
		Wait:           qc.PtrOf(true),
	})
	return err
}

func (c *Client) DeleteByFilter(ctx context.Context, collection string, filter *qc.Filter) error {
	_, err := c.c.Delete(ctx, &qc.DeletePoints{
		CollectionName: collection,
		Points:         qc.NewPointsSelectorFilter(filter),
		Wait:           qc.PtrOf(true),
	})
	return err
}

func (c *Client) Count(ctx context.Context, collection string, filter *qc.Filter) (uint64, error) {
	n, err := c.c.Count(ctx, &qc.CountPoints{
		CollectionName: collection,
		Filter:         filter,
		Exact:          qc.PtrOf(true),
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Stable UUID-shaped string from (org_id, source_id, doc_id). Re-ingesting the
// same doc under the same source upserts in place; a doc shared by two sources
// gets two points.
func PointID(orgID, sourceID, docID string) string {
	h := sha256.Sum256([]byte(orgID + "::" + sourceID + "::" + docID))
	h[6] = (h[6] & 0x0f) | 0x80
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(h[0:4]),
		binary.BigEndian.Uint16(h[4:6]),
		binary.BigEndian.Uint16(h[6:8]),
		binary.BigEndian.Uint16(h[8:10]),
		h[10:16])
}
