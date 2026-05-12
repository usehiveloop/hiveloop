package hindsight

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/streaming"
)

const (
	retainerGroup     = "hindsight-retainer"
	retainerBatchSize = 50
	retainerBlockTime = 3 * time.Second
	// Brief delay to let the DB flusher persist events before we query Postgres.
	flusherSettleDelay   = 500 * time.Millisecond
	pendingCheckInterval = 30 * time.Second
)

// Retainer is a Redis Stream consumer that automatically retains conversation
// transcripts to Hindsight after every agent response. It mirrors the
// streaming.Flusher pattern exactly — a second consumer group on the same streams.
type Retainer struct {
	bus      *streaming.EventBus
	db       *gorm.DB
	client   *Client
	consumer string
}

// NewRetainer creates a new Retainer.
func NewRetainer(bus *streaming.EventBus, db *gorm.DB, client *Client) *Retainer {
	consumer, _ := os.Hostname()
	if consumer == "" {
		consumer = uuid.New().String()[:8]
	}
	return &Retainer{
		bus:      bus,
		db:       db,
		client:   client,
		consumer: "hs-" + consumer,
	}
}

// OrgBankID returns the Hindsight bank ID for an org.
func OrgBankID(orgID uuid.UUID) string {
	return "org-" + orgID.String()
}

// Run starts the retainer loop. Blocks until ctx is cancelled.
func (r *Retainer) Run(ctx context.Context) {
	logging.FromContext(ctx).InfoContext(ctx, "hindsight retainer started", "consumer", r.consumer)
	defer logging.FromContext(ctx).InfoContext(ctx, "hindsight retainer stopped", "consumer", r.consumer)

	r.processPending(ctx)

	ticker := time.NewTicker(retainerBlockTime)
	defer ticker.Stop()

	pendingTicker := time.NewTicker(pendingCheckInterval)
	defer pendingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processAll(ctx)
		case <-pendingTicker.C:
			r.processPending(ctx)
		}
	}
}

// processAll reads from all active conversation streams.
func (r *Retainer) processAll(ctx context.Context) {
	convIDs, err := r.bus.ActiveConversations(ctx)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "hindsight retainer: failed to get active conversations", "error", err)
		return
	}

	for _, convID := range convIDs {
		if ctx.Err() != nil {
			return
		}
		r.processStream(ctx, convID)
	}
}

// processStream reads new events from a single conversation stream and triggers
// retain on response_completed events.
func (r *Retainer) processStream(ctx context.Context, convID string) {
	streamKey := r.bus.Prefix() + convID

	_ = r.bus.Redis().XGroupCreateMkStream(ctx, streamKey, retainerGroup, "0").Err()

	streams, err := r.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    retainerGroup,
		Consumer: r.consumer,
		Streams:  []string{streamKey, ">"},
		Count:    retainerBatchSize,
		Block:    100 * time.Millisecond,
	}).Result()
	if err != nil && err != redis.Nil {
		if ctx.Err() == nil {
			logging.FromContext(ctx).ErrorContext(ctx, "hindsight retainer: XREADGROUP error", "conversation_id", convID, "error", err)
		}
		return
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return
	}

	msgs := streams[0].Messages
	entryIDs := make([]string, 0, len(msgs))
	shouldRetain := false

	for _, msg := range msgs {
		entryIDs = append(entryIDs, msg.ID)
		eventType, _ := msg.Values["event_type"].(string)
		if eventType == "response_completed" {
			shouldRetain = true
		}
	}

	if len(entryIDs) > 0 {
		r.bus.Redis().XAck(ctx, streamKey, retainerGroup, entryIDs...)
	}

	if !shouldRetain {
		return
	}

	time.Sleep(flusherSettleDelay)

	convUUID, err := uuid.Parse(convID)
	if err != nil {
		return
	}

	r.retainConversation(ctx, convUUID)
}

// retainConversation builds the full transcript and retains it to Hindsight.
func (r *Retainer) retainConversation(ctx context.Context, convID uuid.UUID) {

	var conv model.AgentConversation
	if err := r.db.Preload("Agent").
		Where("id = ?", convID).First(&conv).Error; err != nil {
		return
	}

	agent := conv.Agent

	if agent.OrgID == nil {
		return
	}

	bankID := OrgBankID(*agent.OrgID)
	if err := r.ensureOrgBankConfigured(ctx, &agent); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "hindsight retainer: failed to ensure org bank",
			"agent_id", agent.ID, "org_id", agent.OrgID, "error", err)
		return
	}

	transcript, err := r.buildTranscript(convID)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "hindsight retainer: failed to build transcript",
			"conversation_id", convID, "error", err)
		return
	}
	if transcript == "" {
		return
	}

	agentContext := agent.Name
	if agent.Description != nil && *agent.Description != "" {
		agentContext += " (" + *agent.Description + ")"
	}
	agentContext += " agent conversation"

	tags := baseMemoryTags(&agent, "manual")
	if memoryTeamTag(&agent) != "" {
		tags = append(tags, "memory_type:team_context")
	} else {
		tags = append(tags, "memory_type:company_context")
	}
	observationScopes := [][]string{{"company:" + agent.OrgID.String()}}
	if teamTag := memoryTeamTag(&agent); teamTag != "" {
		observationScopes = append(observationScopes, []string{"company:" + agent.OrgID.String(), teamTag})
	}

	_, err = r.client.Retain(ctx, bankID, &RetainRequest{
		Items: []RetainItem{{
			Content:           transcript,
			Context:           agentContext,
			DocumentID:        "conv-" + convID.String(),
			Tags:              tags,
			Timestamp:         conv.CreatedAt.Format(time.RFC3339),
			Metadata:          map[string]string{"conversation_id": convID.String(), "agent_id": agent.ID.String()},
			ObservationScopes: observationScopes,
		}},
		Async: true,
	})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "hindsight retainer: retain failed",
			"conversation_id", convID,
			"bank_id", bankID,
			"error", err)
		return
	}
}

// buildTranscript reconstructs the conversation from persisted events.
func (r *Retainer) buildTranscript(convID uuid.UUID) (string, error) {
	var events []model.ConversationEvent
	if err := r.db.Where("conversation_id = ? AND event_type IN ?",
		convID, []string{"message_received", "response_completed"}).
		Find(&events).Error; err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "", nil
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].SequenceNumber < events[j].SequenceNumber
	})

	var buf strings.Builder
	for _, e := range events {
		var data map[string]any
		if len(e.Data) > 0 {
			_ = json.Unmarshal(e.Data, &data)
		}
		if data == nil {
			continue
		}

		switch e.EventType {
		case "message_received":
			content, _ := data["content"].(string)
			if content != "" {
				buf.WriteString("User: ")
				buf.WriteString(content)
				buf.WriteString("\n\n")
			}
		case "response_completed":
			content, _ := data["full_response"].(string)
			if content != "" {
				buf.WriteString("Assistant: ")
				buf.WriteString(content)
				buf.WriteString("\n\n")
			}
		}
	}

	return strings.TrimSpace(buf.String()), nil
}

// ensureOrgBankConfigured creates and configures the org-scoped Hindsight bank
// if it doesn't exist yet, or re-applies config if it has changed.
// Per-agent observation scoping is set on each RetainItem in retainConversation.
func (r *Retainer) ensureOrgBankConfigured(ctx context.Context, agent *model.Agent) error {
	bankID := OrgBankID(*agent.OrgID)
	memCfg := DefaultMemoryConfig()

	configHash := fmt.Sprintf("%x", memCfg.Hash()+"|org-"+agent.OrgID.String())

	var bank model.HindsightBank
	err := r.db.Where("bank_id = ?", bankID).First(&bank).Error

	if err == gorm.ErrRecordNotFound {
		if err := r.client.ConfigureBank(ctx, bankID, memCfg.ToBankConfigUpdate()); err != nil {
			return fmt.Errorf("configuring org bank: %w", err)
		}

		_ = r.client.CreateMentalModel(ctx, bankID, &CreateMentalModelRequest{
			Name:        "Organization Memory",
			SourceQuery: "Summarize everything known across all agents in this organization.",
			Trigger:     &MentalModelTrigger{RefreshAfterConsolidation: true},
		})

		bank = model.HindsightBank{
			BankID:     bankID,
			ConfigHash: configHash,
		}
		if err := r.db.Create(&bank).Error; err != nil {
			if !isDuplicateKey(err) {
				return fmt.Errorf("recording org bank: %w", err)
			}
		}
		logging.FromContext(ctx).InfoContext(ctx, "hindsight retainer: org bank created",
			"bank_id", bankID, "org_id", agent.OrgID)
		return nil
	}

	if err != nil {
		return fmt.Errorf("checking org bank: %w", err)
	}

	if bank.ConfigHash != configHash {
		if err := r.client.ConfigureBank(ctx, bankID, memCfg.ToBankConfigUpdate()); err != nil {
			return fmt.Errorf("updating org bank config: %w", err)
		}
		r.db.Model(&bank).Update("config_hash", configHash)
	}

	return nil
}

// processPending re-processes unacknowledged entries (crash recovery).
func (r *Retainer) processPending(ctx context.Context) {
	convIDs, err := r.bus.ActiveConversations(ctx)
	if err != nil {
		return
	}

	for _, convID := range convIDs {
		if ctx.Err() != nil {
			return
		}
		streamKey := r.bus.Prefix() + convID

		_ = r.bus.Redis().XGroupCreateMkStream(ctx, streamKey, retainerGroup, "0").Err()

		streams, err := r.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    retainerGroup,
			Consumer: r.consumer,
			Streams:  []string{streamKey, "0"},
			Count:    retainerBatchSize,
		}).Result()
		if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}

		r.processStream(ctx, convID)
	}
}

// isDuplicateKey checks if an error is a Postgres unique constraint violation.
func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key")
}
