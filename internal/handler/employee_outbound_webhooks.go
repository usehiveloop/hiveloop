package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

var obviousSecretPattern = regexp.MustCompile(`(?i)(ptok_|xox[baprs]-|sk-[a-z0-9]|api[_-]?key|secret|token|password)\s*[:=]\s*\S+`)

type EmployeeOutboundWebhookHandler struct {
	db       *gorm.DB
	encKey   *crypto.SymmetricKey
	enqueuer enqueue.TaskEnqueuer
	writer   *EmployeeEventWriter
	now      func() time.Time
	maxBytes int64
}

type employeeOutboundEvent struct {
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	At        time.Time       `json:"at"`
}

func NewEmployeeOutboundWebhookHandler(db *gorm.DB, encKey *crypto.SymmetricKey, enqueuer enqueue.TaskEnqueuer, writers ...*EmployeeEventWriter) *EmployeeOutboundWebhookHandler {
	h := &EmployeeOutboundWebhookHandler{
		db:       db,
		encKey:   encKey,
		enqueuer: enqueuer,
		now:      time.Now,
		maxBytes: 512 * 1024,
	}
	if len(writers) > 0 {
		h.writer = writers[0]
	}
	return h
}

func (h *EmployeeOutboundWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sandboxID, err := uuid.Parse(chi.URLParam(r, "sandboxID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_id"})
		return
	}
	var sb model.Sandbox
	if err := h.db.WithContext(ctx).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if !h.verifySignature(ctx, &sb, body, r.Header.Get("X-Hiveloop-Signature")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}
	var event employeeOutboundEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}
	if event.At.IsZero() {
		event.At = h.now().UTC()
	}
	h.storeAndMaybeEnqueue(ctx, &sb, &event)
	h.db.WithContext(ctx).Model(&sb).Update("last_active_at", h.now())
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *EmployeeOutboundWebhookHandler) verifySignature(ctx context.Context, sb *model.Sandbox, body []byte, signature string) bool {
	if h.encKey == nil {
		return true
	}
	secret, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "employee outbound webhook: failed to decrypt runtime secret",
			"sandbox_id", sb.ID, "error", err)
		return false
	}
	signature = strings.TrimSpace(strings.TrimPrefix(signature, "sha256="))
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (h *EmployeeOutboundWebhookHandler) storeAndMaybeEnqueue(ctx context.Context, sb *model.Sandbox, event *employeeOutboundEvent) {
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payload)
	}
	sessionID := stringValue(payload, "session_id")
	stored, ok := employeeMemoryEventFromOutbound(sb, event, payload, sessionID)
	if !ok {
		return
	}
	if h.writer != nil {
		h.writer.Write(ctx, stored)
	} else {
		if err := h.db.WithContext(ctx).Create(&stored).Error; err != nil {
			logging.Capture(ctx, fmt.Errorf("employee outbound webhook: store memory event sandbox_id=%s event_type=%s: %w", sb.ID, event.EventType, err))
			return
		}
	}
	if event.EventType == "skill.synced" {
		if err := h.syncSkillEvent(ctx, sb, payload); err != nil {
			logging.Capture(ctx, fmt.Errorf("employee outbound webhook: sync skill sandbox_id=%s: %w", sb.ID, err))
		}
	}
	if h.enqueuer == nil || sessionID == "" || !shouldTriggerEmployeeMemoryCheckpoint(event.EventType) {
		return
	}
	task, err := tasks.NewEmployeeMemoryRetainTask(tasks.EmployeeMemoryRetainPayload{
		AgentID:     *sb.AgentID,
		SandboxID:   sb.ID,
		SessionID:   sessionID,
		Reason:      "employee_outbound_checkpoint",
		SourceEvent: event.EventType,
	})
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.ProcessIn(3*time.Second),
		asynq.Unique(90*time.Second),
		asynq.TaskID("employee-memory-retain:"+sb.ID.String()+":"+sessionID),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		logging.Capture(ctx, fmt.Errorf("employee outbound webhook: enqueue memory retain: %w", err))
	}
}

func employeeMemoryEventFromOutbound(sb *model.Sandbox, event *employeeOutboundEvent, payload map[string]any, sessionID string) (model.EmployeeMemoryEvent, bool) {
	if sb.OrgID == nil || sb.AgentID == nil {
		return model.EmployeeMemoryEvent{}, false
	}
	return model.EmployeeMemoryEvent{
		OrgID:     *sb.OrgID,
		AgentID:   *sb.AgentID,
		SandboxID: sb.ID,
		SessionID: sessionID,
		EventType: event.EventType,
		Source:    employeeEventSource(payload),
		Payload:   model.RawJSON(event.Payload),
		EventAt:   event.At.UTC(),
	}, true
}

func employeeEventSource(payload map[string]any) string {
	source := sanitizeTagValue(stringValue(payload, "source"))
	if source == "" {
		source = sanitizeTagValue(stringValue(payload, "gateway"))
	}
	if source == "" {
		source = sanitizeTagValue(stringValue(payload, "platform"))
	}
	if source == "" {
		return "manual"
	}
	return source
}

func sanitizeTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '/':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_")
}

func shouldTriggerEmployeeMemoryCheckpoint(eventType string) bool {
	switch eventType {
	case "agent.message.sent", "session.completed":
		return true
	default:
		return false
	}
}

func payloadLooksSensitive(payload map[string]any) bool {
	for _, key := range []string{"text", "result_summary", "message", "error"} {
		if obviousSecretPattern.MatchString(stringValue(payload, key)) {
			return true
		}
	}
	return false
}

func stringValue(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
