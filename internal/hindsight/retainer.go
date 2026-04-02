package hindsight

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/streaming"
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

// BankID returns the Hindsight bank ID for an identity.
func (r *Retainer) BankID(identityID uuid.UUID) string {
	return "identity-" + identityID.String()
}

// MCPURL returns the Hindsight MCP server URL for an identity's bank.
func (r *Retainer) MCPURL(identityID uuid.UUID) string {
	return r.client.baseURL + "/mcp/" + r.BankID(identityID) + "/"
}

// Run starts the retainer loop. Blocks until ctx is cancelled.
func (r *Retainer) Run(ctx context.Context) {
	slog.Info("hindsight retainer started", "consumer", r.consumer)
	defer slog.Info("hindsight retainer stopped", "consumer", r.consumer)

	// Process pending (unacknowledged) entries from a previous crash first
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
		slog.Error("hindsight retainer: failed to get active conversations", "error", err)
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

	// Ensure consumer group exists
	r.bus.Redis().XGroupCreateMkStream(ctx, streamKey, retainerGroup, "0").Err()

	// Read new messages
	streams, err := r.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    retainerGroup,
		Consumer: r.consumer,
		Streams:  []string{streamKey, ">"},
		Count:    retainerBatchSize,
		Block:    100 * time.Millisecond,
	}).Result()
	if err != nil && err != redis.Nil {
		if ctx.Err() == nil {
			slog.Error("hindsight retainer: XREADGROUP error", "conversation_id", convID, "error", err)
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

	// ACK all entries regardless — we only care about response_completed as a trigger
	if len(entryIDs) > 0 {
		r.bus.Redis().XAck(ctx, streamKey, retainerGroup, entryIDs...)
	}

	if !shouldRetain {
		return
	}

	// Wait briefly for the DB flusher to persist events
	time.Sleep(flusherSettleDelay)

	convUUID, err := uuid.Parse(convID)
	if err != nil {
		return
	}

	r.retainConversation(ctx, convUUID)
}

// retainConversation builds the full transcript and retains it to Hindsight.
func (r *Retainer) retainConversation(ctx context.Context, convID uuid.UUID) {
	// Load conversation with agent and identity
	var conv model.AgentConversation
	if err := r.db.Preload("Agent").Preload("Agent.Identity").
		Where("id = ?", convID).First(&conv).Error; err != nil {
		slog.Debug("hindsight retainer: conversation not found", "conversation_id", convID)
		return
	}

	agent := conv.Agent
	identity := agent.Identity

	// Check if memory is enabled for this identity
	memCfg := ParseMemoryConfig(identity.MemoryConfig)
	if !memCfg.IsEnabled() {
		return
	}

	// Skip agents without a team (memory is opt-in via team assignment)
	if agent.Team == "" {
		return
	}

	// Build transcript from persisted events
	transcript, err := r.buildTranscript(convID)
	if err != nil {
		slog.Error("hindsight retainer: failed to build transcript",
			"conversation_id", convID, "error", err)
		return
	}
	if transcript == "" {
		return
	}

	// Ensure bank is created and configured
	if err := r.ensureBankConfigured(ctx, &identity); err != nil {
		slog.Error("hindsight retainer: failed to ensure bank",
			"identity_id", identity.ID, "error", err)
		return
	}

	// Build context string for Hindsight
	agentContext := agent.Name
	if agent.Description != nil && *agent.Description != "" {
		agentContext += " (" + *agent.Description + ")"
	}
	agentContext += fmt.Sprintf(" [%s team] agent conversation", agent.Team)

	// Retain
	bankID := r.BankID(identity.ID)
	_, err = r.client.Retain(ctx, bankID, &RetainRequest{
		Items: []RetainItem{{
			Content:    transcript,
			Context:    agentContext,
			DocumentID: "conv-" + convID.String(),
			Tags:       []string{"team:" + agent.Team, "agent:" + agent.ID.String(), "conv:" + convID.String()},
			Timestamp:  conv.CreatedAt.Format(time.RFC3339),
		}},
		Async: true,
	})
	if err != nil {
		slog.Error("hindsight retainer: retain failed",
			"conversation_id", convID,
			"bank_id", bankID,
			"error", err)
		return
	}

	slog.Debug("hindsight retainer: retained conversation",
		"conversation_id", convID,
		"bank_id", bankID,
		"team", agent.Team)
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

	// Sort by sequence_number from payload
	sort.Slice(events, func(i, j int) bool {
		seqI := extractSequenceNumber(events[i].Payload)
		seqJ := extractSequenceNumber(events[j].Payload)
		return seqI < seqJ
	})

	var buf strings.Builder
	for _, e := range events {
		data, _ := e.Payload["data"].(map[string]any)
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

// ensureBankConfigured creates and configures the Hindsight bank for an identity
// if it doesn't exist yet, or re-applies config if it has changed.
func (r *Retainer) ensureBankConfigured(ctx context.Context, identity *model.Identity) error {
	bankID := r.BankID(identity.ID)
	memCfg := ParseMemoryConfig(identity.MemoryConfig)

	// Collect distinct teams from this identity's agents for observation scopes
	var teams []string
	r.db.Model(&model.Agent{}).
		Where("identity_id = ? AND team != '' AND status = 'active'", identity.ID).
		Distinct("team").Pluck("team", &teams)

	// Build observation scopes from teams
	var scopes [][]string
	for _, t := range teams {
		scopes = append(scopes, []string{"team:" + t})
	}
	scopes = append(scopes, []string{"shared"})

	// Compute config hash (includes teams so new teams trigger re-config)
	hashInput := memCfg.Hash() + "|" + strings.Join(teams, ",")
	configHash := fmt.Sprintf("%x", hashInput)

	// Check existing bank record
	var bank model.HindsightBank
	err := r.db.Where("identity_id = ?", identity.ID).First(&bank).Error

	if err == gorm.ErrRecordNotFound {
		// First time — create bank, apply config, create mental model
		if err := r.client.ConfigureBank(ctx, bankID, memCfg.ToBankConfigUpdate(scopes)); err != nil {
			return fmt.Errorf("configuring bank: %w", err)
		}

		// Create default mental model
		_ = r.client.CreateMentalModel(ctx, bankID, &CreateMentalModelRequest{
			Name:        "Identity Profile",
			SourceQuery: "Summarize everything known about this user/identity: preferences, context, history, ongoing work, and relationships.",
			Trigger:     &MentalModelTrigger{RefreshAfterConsolidation: true},
		})

		// Record in DB
		bank = model.HindsightBank{
			IdentityID: identity.ID,
			BankID:     bankID,
			ConfigHash: configHash,
		}
		if err := r.db.Create(&bank).Error; err != nil {
			// Duplicate key = another goroutine created it first — that's fine
			if !isDuplicateKey(err) {
				return fmt.Errorf("recording bank: %w", err)
			}
		}
		slog.Info("hindsight retainer: bank created",
			"bank_id", bankID, "identity_id", identity.ID, "teams", teams)
		return nil
	}

	if err != nil {
		return fmt.Errorf("checking bank: %w", err)
	}

	// Bank exists — check if config changed
	if bank.ConfigHash != configHash {
		if err := r.client.ConfigureBank(ctx, bankID, memCfg.ToBankConfigUpdate(scopes)); err != nil {
			return fmt.Errorf("updating bank config: %w", err)
		}
		r.db.Model(&bank).Update("config_hash", configHash)
		slog.Info("hindsight retainer: bank config updated",
			"bank_id", bankID, "identity_id", identity.ID)
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

		r.bus.Redis().XGroupCreateMkStream(ctx, streamKey, retainerGroup, "0").Err()

		streams, err := r.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    retainerGroup,
			Consumer: r.consumer,
			Streams:  []string{streamKey, "0"},
			Count:    retainerBatchSize,
		}).Result()
		if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}

		// Re-process the stream
		r.processStream(ctx, convID)
	}
}

// extractSequenceNumber pulls the sequence_number from an event payload.
func extractSequenceNumber(payload model.JSON) float64 {
	seq, _ := payload["sequence_number"].(float64)
	return seq
}

// isDuplicateKey checks if an error is a Postgres unique constraint violation.
func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key")
}
