package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

const slackKnowledgeHistoryDays = 90

func (h *AgentProfileHandler) ensureSlackKnowledgeSource(ctx context.Context, agent model.Agent, profile *model.AgentProfile) {
	if profile == nil {
		return
	}
	source, err := h.upsertSlackKnowledgeSource(ctx, agent, profile)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("slack profile: ensure knowledge source failed: %w", err))
		return
	}
	if h.enq == nil {
		return
	}
	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: source.ID})
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("slack profile: build ingest task failed: %w", err))
		return
	}
	if _, err := h.enq.EnqueueContext(ctx, task, asynq.Unique(60*time.Second)); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		logging.Capture(ctx, fmt.Errorf("slack profile: enqueue ingest failed: %w", err))
	}
}

func (h *AgentProfileHandler) upsertSlackKnowledgeSource(ctx context.Context, agent model.Agent, profile *model.AgentProfile) (*ragmodel.RAGSource, error) {
	sourceID, _ := slackKnowledgeSourceID(profile.Config)
	var source ragmodel.RAGSource
	if sourceID != uuid.Nil {
		err := h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND kind = ?", sourceID, profile.OrgID, ragmodel.RAGSourceKindSlackBotProfile).
			First(&source).Error
		if err == nil {
			return &source, nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND kind = ? AND config->>'agent_profile_id' = ?", profile.OrgID, ragmodel.RAGSourceKindSlackBotProfile, profile.ID.String()).
		First(&source).Error
	if err == nil {
		if profile.Config == nil {
			profile.Config = model.JSON{}
		}
		profile.Config["knowledge_source_id"] = source.ID.String()
		_ = h.db.WithContext(ctx).Model(profile).Update("config", profile.Config).Error
		return &source, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	cfg := model.JSON{
		"agent_profile_id":                profile.ID.String(),
		"agent_id":                        agent.ID.String(),
		"history_days":                    slackKnowledgeHistoryDays,
		"include_public_channels":         true,
		"include_joined_private_channels": true,
	}
	refresh := 10 * 60
	indexingStart := time.Now().UTC().AddDate(0, 0, -slackKnowledgeHistoryDays)
	source = ragmodel.RAGSource{
		ID:                 uuid.New(),
		OrgIDValue:         profile.OrgID,
		KindValue:          ragmodel.RAGSourceKindSlackBotProfile,
		Name:               "Slack history for " + agent.Name,
		Status:             ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:            true,
		ConfigValue:        cfg,
		AccessType:         ragmodel.AccessTypePublic,
		IndexingStart:      &indexingStart,
		RefreshFreqSeconds: &refresh,
	}
	if err := h.db.WithContext(ctx).Create(&source).Error; err != nil {
		return nil, err
	}
	if profile.Config == nil {
		profile.Config = model.JSON{}
	}
	profile.Config["knowledge_source_id"] = source.ID.String()
	if err := h.db.WithContext(ctx).Model(profile).Update("config", profile.Config).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func slackKnowledgeSourceID(config model.JSON) (uuid.UUID, bool) {
	if config == nil {
		return uuid.Nil, false
	}
	value, ok := config["knowledge_source_id"].(string)
	if !ok || value == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(value)
	return id, err == nil
}
