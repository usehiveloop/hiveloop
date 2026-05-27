package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
	"github.com/usehivy/hivy/internal/slackapp"
)

func (h *SlackChannelHandler) autoCreateSlackRAGSource(ctx context.Context, orgID uuid.UUID) {
	if h.enq == nil {
		return
	}
	connID, err := h.activeSlackConnectionID(ctx, orgID)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag: %w", err))
		return
	}

	src := &ragmodel.RAGSource{
		OrgIDValue: orgID,
		KindValue:  ragmodel.RAGSourceKindIntegration,
		Name:       "Slack",
		Status:     ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:    true,
		AccessType: ragmodel.AccessTypeSync,
		RefreshFreqSeconds: intPtr(3600),
	}
	src.ConnectionID = &connID

	if err := h.db.Create(src).Error; err != nil {
		if isDuplicateKeyError(err) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag source for org %s: %w", orgID, err))
		return
	}

	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: src.ID})
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag: build ingest task for %s: %w", src.ID, err))
		return
	}
	if _, err := h.enq.Enqueue(task, asynq.Unique(60*time.Second)); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag: enqueue ingest for %s: %w", src.ID, err))
	}
}

func (h *SlackChannelHandler) activeSlackConnectionID(ctx context.Context, orgID uuid.UUID) (uuid.UUID, error) {
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, slackapp.Provider).
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return uuid.Nil, fmt.Errorf("no active Slack connection: %w", err)
	}
	return conn.ID, nil
}
