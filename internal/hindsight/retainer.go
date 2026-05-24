package hindsight

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/streaming"
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

	var conv model.EmployeeConversation
	if err := r.db.Preload("Employee").
		Where("id = ?", convID).First(&conv).Error; err != nil {
		return
	}

	agent := conv.Employee

	if agent.OrgID == nil {
		return
	}

	bankID := OrgBankID(*agent.OrgID)
	if err := r.ensureOrgBankConfigured(ctx, &agent); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "hindsight retainer: failed to ensure org bank",
			"employee_id", agent.ID, "org_id", agent.OrgID, "error", err)
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
	tags = append(tags, "memory_type:company_context")
	observationScopes := [][]string{{"company:" + agent.OrgID.String()}}

	_, err = r.client.Retain(ctx, bankID, &RetainRequest{
		Items: []RetainItem{{
			Content:           transcript,
			Context:           agentContext,
			DocumentID:        "conv-" + convID.String(),
			Tags:              tags,
			Timestamp:         conv.CreatedAt.Format(time.RFC3339),
			Metadata:          map[string]string{"conversation_id": convID.String(), "employee_id": agent.ID.String()},
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
