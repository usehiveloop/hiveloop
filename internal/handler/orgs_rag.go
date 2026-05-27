package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

func (h *OrgHandler) autoCreateWebsiteRAGSource(ctx context.Context, org *model.Org) {
	if org.Website == "" {
		return
	}
	if h.enq == nil {
		return
	}

	src := &ragmodel.RAGSource{
		OrgIDValue: org.ID,
		KindValue:  ragmodel.RAGSourceKindWebsite,
		Name:       org.Website,
		Status:     ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:    true,
		AccessType: ragmodel.AccessTypePublic,
		ConfigValue: model.JSON{
			"url":       org.Website,
			"max_pages": float64(100),
		},
		RefreshFreqSeconds: intPtr(86400),
	}

	if err := h.db.Create(src).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("auto-create website rag source for org %s: %w", org.ID, err))
		return
	}

	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: src.ID})
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("auto-create website rag: build ingest task for %s: %w", src.ID, err))
		return
	}
	if _, err := h.enq.Enqueue(task, asynq.Unique(60*time.Second)); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("auto-create website rag: enqueue ingest for %s: %w", src.ID, err))
	}
}
