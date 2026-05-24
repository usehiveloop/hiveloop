package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/bridgeevents"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

// BridgeWebhookHandler receives webhook events from Bridge instances.
type BridgeWebhookHandler struct {
	db                      *gorm.DB
	encKey                  *crypto.SymmetricKey
	eventBus                EventPublisher       // nil-safe: if nil, events go directly to Postgres
	enqueuer                enqueue.TaskEnqueuer // nil-safe: if nil, conversation naming is skipped
	employeeCallbackRuntime employeeCallbackSandboxSpecialists
}

// EventPublisher is the interface for publishing events to the streaming bus.
type EventPublisher interface {
	Publish(ctx context.Context, convID string, eventType string, data json.RawMessage) (string, error)
}

// NewBridgeWebhookHandler creates a webhook handler.
func NewBridgeWebhookHandler(db *gorm.DB, encKey *crypto.SymmetricKey, eventBus EventPublisher, enqueuer enqueue.TaskEnqueuer) *BridgeWebhookHandler {
	return &BridgeWebhookHandler{db: db, encKey: encKey, eventBus: eventBus, enqueuer: enqueuer}
}

// NewBridgeWebhookHandlerWithEmployeeRuntime creates a webhook handler that can
// refresh and wake employee runtimes before forwarding specialist callbacks.
func NewBridgeWebhookHandlerWithEmployeeRuntime(db *gorm.DB, encKey *crypto.SymmetricKey, eventBus EventPublisher, enqueuer enqueue.TaskEnqueuer, runtime employeeCallbackSandboxSpecialists) *BridgeWebhookHandler {
	h := NewBridgeWebhookHandler(db, encKey, eventBus, enqueuer)
	h.employeeCallbackRuntime = runtime
	return h
}

// webhookEvent is a single event in a Bridge webhook batch.
type webhookEvent struct {
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	EmployeeID     string          `json:"employee_id"`
	ConversationID string          `json:"conversation_id"`
	Timestamp      time.Time       `json:"timestamp"`
	SequenceNumber int64           `json:"sequence_number"`
	Data           json.RawMessage `json:"data"`
}

// Handle processes POST /internal/webhooks/bridge/{sandboxID}.
// Bridge sends batched webhook events as a JSON array, signed with HMAC-SHA256.
func (h *BridgeWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sandboxID := chi.URLParam(r, "sandboxID")
	if sandboxID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing sandbox_id"})
		return
	}

	sbUUID, err := uuid.Parse(sandboxID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_id"})
		return
	}

	var sb model.Sandbox
	if err := h.db.WithContext(ctx).Where("id = ?", sbUUID).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		captureSpecialistFailure(ctx, "bridge_webhook", err, specialistSentryContext{
			Operation: "load_sandbox",
			SandboxID: sbUUID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	if h.encKey != nil {
		secret, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "webhook: failed to decrypt bridge api key", "sandbox_id", sandboxID, "error", err)
			captureSpecialistFailure(ctx, "bridge_webhook", err, specialistSentryContext{
				Operation: "decrypt_bridge_key",
				OrgID:     uuidValue(sb.OrgID),
				SandboxID: sb.ID,
			})
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

	var events []webhookEvent
	if err := json.Unmarshal(body, &events); err != nil {
		captureBridgeWebhookIngest(ctx, "decode_payload", &sb, nil, uuid.Nil, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}

	if err := h.db.WithContext(ctx).Model(&sb).Update("last_active_at", time.Now()).Error; err != nil {
		captureBridgeWebhookIngest(ctx, "update_sandbox_last_active", &sb, nil, uuid.Nil, err)
	}

	for _, event := range events {
		h.processEvent(ctx, &sb, &event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *BridgeWebhookHandler) processEvent(ctx context.Context, sb *model.Sandbox, event *webhookEvent) {
	var conv model.EmployeeConversation
	if err := h.db.WithContext(ctx).Where("runtime_conversation_id = ? AND sandbox_id = ?",
		event.ConversationID, sb.ID).First(&conv).Error; err != nil {
		stage := "load_conversation"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			stage = "conversation_not_found"
		}
		captureBridgeWebhookIngest(ctx, stage, sb, event, uuid.Nil, err)
		return
	}

	var specialistTask *model.SpecialistTask
	if task, ok := h.specialistTaskForConversation(ctx, conv.ID); ok {
		specialistTask = task
	}

	redisPayloadMap := map[string]any{
		"event_id":        event.EventID,
		"event_type":      event.EventType,
		"employee_id":     event.EmployeeID,
		"conversation_id": event.ConversationID,
		"timestamp":       event.Timestamp.Format(time.RFC3339),
		"sequence_number": event.SequenceNumber,
		"data":            json.RawMessage(event.Data),
	}
	if specialistTask != nil {
		redisPayloadMap["task_id"] = specialistTask.ID.String()
		redisPayloadMap["metadata"] = map[string]any(specialistTask.Metadata)
	}
	redisPayload, _ := json.Marshal(redisPayloadMap)

	if h.eventBus != nil {
		_, err := h.eventBus.Publish(ctx, conv.ID.String(), event.EventType, redisPayload)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "webhook: Redis publish failed, falling back to direct DB write",
				"conversation_id", conv.ID,
				"error", err,
			)
			captureBridgeWebhookIngest(ctx, "publish_stream", sb, event, conv.ID, err)
			h.writeEventToPostgres(ctx, &conv, event)
		}
	} else {
		h.writeEventToPostgres(ctx, &conv, event)
	}

	switch event.EventType {
	case bridgeevents.EventConversationEnded:
		now := time.Now()
		if err := h.db.WithContext(ctx).Model(&conv).Updates(map[string]any{
			"status":   "ended",
			"ended_at": now,
		}).Error; err != nil {
			captureBridgeWebhookIngest(ctx, "mark_conversation_ended", sb, event, conv.ID, err)
		}
	case bridgeevents.EventAgentError:
		if err := h.db.WithContext(ctx).Model(&conv).Update("status", "error").Error; err != nil {
			captureBridgeWebhookIngest(ctx, "mark_conversation_error", sb, event, conv.ID, err)
		}
		logging.FromContext(ctx).WarnContext(ctx, "webhook: agent error",
			"conversation_id", conv.ID,
			"error", string(event.Data),
		)
	case bridgeevents.EventMessageReceived:
		h.maybeEnqueueConversationNaming(ctx, &conv)
	}

	if specialistTask != nil && shouldForwardSpecialistEvent(event.EventType) {
		h.forwardSpecialistEvent(ctx, *specialistTask, &conv, event)
	}
}

// maybeEnqueueConversationNaming fires the async title-generation job when a
// message_received event arrives for a still-unnamed conversation. The job
// itself re-checks conv.Name and no-ops if it's been set by another path, so
// this is best-effort — if the enqueuer isn't configured or enqueue fails, we
// log and move on. The frontend falls back to deriving a title from the
// message content.
func (h *BridgeWebhookHandler) maybeEnqueueConversationNaming(ctx context.Context, conv *model.EmployeeConversation) {
	if h.enqueuer == nil {
		return
	}
	if conv.Name != "" {
		return
	}
	task, err := tasks.NewConversationNameTask(conv.ID)
	if err != nil {
		captureBridgeWebhookIngest(ctx, "build_conversation_naming_task", nil, nil, conv.ID, err)
		return
	}
	if _, err := h.enqueuer.Enqueue(task); err != nil {
		captureBridgeWebhookIngest(ctx, "enqueue_conversation_naming_task", nil, nil, conv.ID, err)
	}
}

func (h *BridgeWebhookHandler) writeEventToPostgres(ctx context.Context, conv *model.EmployeeConversation, event *webhookEvent) {
	eventType := event.EventType
	if !shouldStoreConversationEvent(eventType) {
		return
	}
	dbEvent := model.ConversationEvent{
		OrgID:                 conv.OrgID,
		ConversationID:        conv.ID,
		EventID:               event.EventID,
		EventType:             eventType,
		EmployeeID:            event.EmployeeID,
		RuntimeConversationID: event.ConversationID,
		Timestamp:             event.Timestamp,
		SequenceNumber:        event.SequenceNumber,
		Data:                  model.RawJSON(event.Data),
	}
	if err := h.db.WithContext(ctx).Create(&dbEvent).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "webhook: failed to store event",
			"event_type", event.EventType,
			"conversation_id", conv.ID,
			"error", err,
		)
		sb := model.Sandbox{ID: conv.SandboxID, OrgID: &conv.OrgID, EmployeeID: &conv.EmployeeID}
		captureBridgeWebhookIngest(ctx, "store_conversation_event", &sb, event, conv.ID, err)
	}
}

func shouldStoreConversationEvent(eventType string) bool {
	return eventType != bridgeevents.EventResponseChunk && eventType != bridgeevents.EventReasoningDelta
}

func shouldForwardSpecialistEvent(eventType string) bool {
	switch eventType {
	case bridgeevents.EventConversationEnded, bridgeevents.EventDone, bridgeevents.EventTodoUpdated:
		return true
	default:
		return false
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
