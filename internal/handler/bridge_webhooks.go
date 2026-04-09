package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/model"
)

// BridgeWebhookHandler receives webhook events from Bridge instances.
type BridgeWebhookHandler struct {
	db       *gorm.DB
	encKey   *crypto.SymmetricKey
	eventBus EventPublisher // nil-safe: if nil, events go directly to Postgres
}

// EventPublisher is the interface for publishing events to the streaming bus.
type EventPublisher interface {
	Publish(ctx context.Context, convID string, eventType string, data json.RawMessage) (string, error)
}

// NewBridgeWebhookHandler creates a webhook handler.
func NewBridgeWebhookHandler(db *gorm.DB, encKey *crypto.SymmetricKey, eventBus EventPublisher) *BridgeWebhookHandler {
	return &BridgeWebhookHandler{db: db, encKey: encKey, eventBus: eventBus}
}

// webhookEvent is a single event in a Bridge webhook batch.
type webhookEvent struct {
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	AgentID        string          `json:"agent_id"`
	ConversationID string          `json:"conversation_id"`
	Timestamp      time.Time       `json:"timestamp"`
	SequenceNumber int64           `json:"sequence_number"`
	Data           json.RawMessage `json:"data"`
}

// Handle processes POST /internal/webhooks/bridge/{sandboxID}.
// Bridge sends batched webhook events as a JSON array, signed with HMAC-SHA256.
func (h *BridgeWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	sandboxID := chi.URLParam(r, "sandboxID")
	if sandboxID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing sandbox_id"})
		return
	}

	// Load sandbox to get the encrypted Bridge API key (webhook secret)
	sbUUID, err := uuid.Parse(sandboxID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_id"})
		return
	}

	var sb model.Sandbox
	if err := h.db.Where("id = ?", sbUUID).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	// Verify HMAC signature
	if h.encKey != nil {
		secret, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
		if err != nil {
			slog.Error("webhook: failed to decrypt bridge api key", "sandbox_id", sandboxID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "signature verification failed"})
			return
		}

		signature := r.Header.Get("X-Webhook-Signature")
		timestampStr := r.Header.Get("X-Webhook-Timestamp")
		if signature == "" || timestampStr == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing signature headers"})
			return
		}

		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid timestamp"})
			return
		}

		if !verifyWebhookSignature(body, secret, timestamp, signature) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
			return
		}
	}

	// Parse batch of events
	var events []webhookEvent
	if err := json.Unmarshal(body, &events); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}

	// Update sandbox last_active_at
	h.db.Model(&sb).Update("last_active_at", time.Now())

	// Process each event
	for _, event := range events {
		h.processEvent(&sb, &event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *BridgeWebhookHandler) processEvent(sb *model.Sandbox, event *webhookEvent) {
	// Find our conversation record by Bridge conversation ID
	var conv model.AgentConversation
	if err := h.db.Where("bridge_conversation_id = ? AND sandbox_id = ?",
		event.ConversationID, sb.ID).First(&conv).Error; err != nil {
		slog.Warn("webhook: conversation not found",
			"bridge_conversation_id", event.ConversationID,
			"sandbox_id", sb.ID,
			"event_type", event.EventType,
		)
		return
	}

	slog.Info("webhook: event matched",
		"bridge_conversation_id", event.ConversationID,
		"agent_conversation_id", conv.ID,
		"event_type", event.EventType,
		"sandbox_id", sb.ID,
	)

	// Build the full event JSON for Redis (SSE clients receive this as-is).
	redisPayload, _ := json.Marshal(map[string]any{
		"event_id":        event.EventID,
		"event_type":      event.EventType,
		"agent_id":        event.AgentID,
		"conversation_id": event.ConversationID,
		"timestamp":       event.Timestamp.Format(time.RFC3339),
		"sequence_number": event.SequenceNumber,
		"data":            json.RawMessage(event.Data),
	})

	// Publish to Redis Streams for real-time delivery to SSE subscribers.
	// The background flusher will batch-write to Postgres.
	// If Redis is unavailable, fall back to direct Postgres write.
	if h.eventBus != nil {
		_, err := h.eventBus.Publish(context.Background(), conv.ID.String(), event.EventType, redisPayload)
		if err != nil {
			slog.Warn("webhook: Redis publish failed, falling back to direct DB write",
				"conversation_id", conv.ID,
				"error", err,
			)
			h.writeEventToPostgres(&conv, event)
		}
	} else {
		h.writeEventToPostgres(&conv, event)
	}

	// Update conversation state for terminal events
	switch event.EventType {
	case "ConversationEnded":
		now := time.Now()
		h.db.Model(&conv).Updates(map[string]any{
			"status":   "ended",
			"ended_at": now,
		})
		slog.Info("webhook: conversation ended",
			"conversation_id", conv.ID,
			"bridge_conversation_id", event.ConversationID,
		)
	case "AgentError":
		h.db.Model(&conv).Update("status", "error")
		slog.Warn("webhook: agent error",
			"conversation_id", conv.ID,
			"error", string(event.Data),
		)
	}
}

func (h *BridgeWebhookHandler) writeEventToPostgres(conv *model.AgentConversation, event *webhookEvent) {
	dbEvent := model.ConversationEvent{
		OrgID:                conv.OrgID,
		ConversationID:       conv.ID,
		EventID:              event.EventID,
		EventType:            event.EventType,
		AgentID:              event.AgentID,
		BridgeConversationID: event.ConversationID,
		Timestamp:            event.Timestamp,
		SequenceNumber:       event.SequenceNumber,
		Data:                 model.RawJSON(event.Data),
	}
	if err := h.db.Create(&dbEvent).Error; err != nil {
		slog.Error("webhook: failed to store event",
			"event_type", event.EventType,
			"conversation_id", conv.ID,
			"error", err,
		)
	}
}


// verifyWebhookSignature verifies the HMAC-SHA256 signature.
// Bridge signs with: HMAC-SHA256("{timestamp}.{payload}", secret), base64-encoded.
func verifyWebhookSignature(payload []byte, secret string, timestamp int64, signature string) bool {
	message := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
