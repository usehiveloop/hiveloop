package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	qdrantgo "github.com/qdrant/go-client/qdrant"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

func (d *Deps) HandlePrune(ctx context.Context, t *asynq.Task) error {
	deps := d.withDefaults()
	payload, err := UnmarshalPrune(t.Payload())
	if err != nil {
		return err
	}
	src, err := loadSource(ctx, deps.DB, payload.RAGSourceID)
	if err != nil {
		return err
	}
	conn, err := buildConnector(src, deps)
	if err != nil {
		return err
	}
	slim, ok := conn.(interfaces.SlimConnector)
	if !ok {
		return fmt.Errorf("prune %s: connector kind %q does not implement SlimConnector",
			src.ID, src.KindValue)
	}

	keep, err := drainSlim(ctx, slim, src)
	if err != nil {
		return fmt.Errorf("prune %s: list slim docs: %w", src.ID, err)
	}
	keepSet := make(map[string]struct{}, len(keep))
	for _, id := range keep {
		keepSet[id] = struct{}{}
	}

	stalePointIDs, err := scrollStalePoints(ctx, deps, src, keepSet)
	if err != nil {
		return fmt.Errorf("prune %s: scroll stale points: %w", src.ID, err)
	}
	if len(stalePointIDs) > 0 {
		if err := deps.Qdrant.DeleteByIDs(ctx, deps.Collection, stalePointIDs); err != nil {
			return fmt.Errorf("prune %s: qdrant delete: %w", src.ID, err)
		}
	}
	return touchLastPruned(ctx, deps.DB, src.ID)
}

func drainSlim(
	ctx context.Context,
	slim interfaces.SlimConnector,
	src *ragmodel.RAGSource,
) ([]string, error) {
	stream, err := slim.ListAllSlim(ctx, src)
	if err != nil {
		return nil, err
	}
	var keep []string
	for item := range stream {
		if item.Failure != nil {
			return nil, fmt.Errorf("slim listing failure: %s", item.Failure.FailureMessage)
		}
		if item.Slim == nil {
			continue
		}
		keep = append(keep, item.Slim.DocID)
	}
	return keep, nil
}

func scrollStalePoints(
	ctx context.Context,
	deps *Deps,
	src *ragmodel.RAGSource,
	keepSet map[string]struct{},
) ([]string, error) {
	filter := qdrant.BuildSourceFilter(src.OrgIDValue.String(), src.ID.String())
	var stale []string
	var offset = (*qdrantgo.PointId)(nil)
	for {
		page, err := deps.Qdrant.Scroll(ctx, qdrant.ScrollRequest{
			Collection:  deps.Collection,
			Filter:      filter,
			Limit:       512,
			Offset:      offset,
			WithPayload: true,
		})
		if err != nil {
			return nil, err
		}
		for _, p := range page.Points {
			docID, _ := p.Payload["doc_id"].(string)
			if _, kept := keepSet[docID]; kept {
				continue
			}
			if p.ID != "" {
				stale = append(stale, p.ID)
			}
		}
		if page.NextOffset == nil {
			break
		}
		offset = page.NextOffset
	}
	return stale, nil
}

func touchLastPruned(ctx context.Context, db *gorm.DB, sourceID uuid.UUID) error {
	now := time.Now()
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGSource{}).
		Where("id = ?", sourceID).
		Updates(map[string]any{
			"last_pruned": now,
			"updated_at":  now,
		}).Error; err != nil {
		return fmt.Errorf("prune: advance last_pruned: %w", err)
	}
	return nil
}
