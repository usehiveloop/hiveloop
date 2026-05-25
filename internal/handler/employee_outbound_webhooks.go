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
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type EmployeeOutboundWebhookHandler struct {
	db            *gorm.DB
	encKey        *crypto.SymmetricKey
	enqueuer      enqueue.TaskEnqueuer
	writer        *EmployeeEventWriter
	gateway       *gateway.Service
	now           func() time.Time
	maxBytes      int64
	maxBatchBytes int64
}

type employeeOutboundEvent struct {
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	At        time.Time       `json:"at"`
}

func NewEmployeeOutboundWebhookHandler(db *gorm.DB, encKey *crypto.SymmetricKey, enqueuer enqueue.TaskEnqueuer, writers ...*EmployeeEventWriter) *EmployeeOutboundWebhookHandler {
	h := &EmployeeOutboundWebhookHandler{
		db:            db,
		encKey:        encKey,
		enqueuer:      enqueuer,
		now:           time.Now,
		maxBytes:      512 * 1024,
		maxBatchBytes: 10 * 1024 * 1024,
	}
	if len(writers) > 0 {
		h.writer = writers[0]
	}
	return h
}

func (h *EmployeeOutboundWebhookHandler) SetGatewayService(service *gateway.Service) {
	h.gateway = service
}

func (h *EmployeeOutboundWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sb, ok := h.loadSandbox(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBytes))
	if err != nil {
		captureEmployeeWebhookIngest(ctx, "read_body", sb, nil, "", "", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if !h.verifySignature(ctx, sb, body, r.Header.Get("X-Hivy-Signature")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}
	var event employeeOutboundEvent
	if err := json.Unmarshal(body, &event); err != nil {
		captureEmployeeWebhookIngest(ctx, "decode_webhook_payload", sb, nil, "", "", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}
	if event.At.IsZero() {
		event.At = h.now().UTC()
	}
	h.storeAndMaybeEnqueue(ctx, sb, &event)
	if err := h.db.WithContext(ctx).Model(sb).Update("last_active_at", h.now()).Error; err != nil {
		captureEmployeeWebhookIngest(ctx, "update_last_active", sb, &event, "", "", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *EmployeeOutboundWebhookHandler) loadSandbox(w http.ResponseWriter, r *http.Request) (*model.Sandbox, bool) {
	sandboxID, err := uuid.Parse(chi.URLParam(r, "sandboxID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_id"})
		return nil, false
	}
	var sb model.Sandbox
	if err := h.db.WithContext(r.Context()).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return nil, false
		}
		captureEmployeeWebhookIngest(r.Context(), "load_sandbox", &model.Sandbox{ID: sandboxID}, nil, "", "", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return nil, false
	}
	return &sb, true
}

func (h *EmployeeOutboundWebhookHandler) verifySignature(ctx context.Context, sb *model.Sandbox, body []byte, signature string) bool {
	if h.encKey == nil {
		return true
	}
	secret, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "employee outbound webhook: failed to decrypt runtime secret",
			"sandbox_id", sb.ID, "error", err)
		captureEmployeeWebhookIngest(ctx, "decrypt_runtime_secret", sb, nil, "", "", err)
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
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			captureEmployeeWebhookIngest(ctx, "decode_event_payload", sb, event, "", "", err)
		}
	}
	sessionID := stringValue(payload, "session_id")
	source := employeeEventSource(payload)
	specialistTask, taskFound := h.specialistTaskForSandbox(ctx, sb.ID)
	if taskFound {
		sessionID = specialistTask.EmployeeSessionID
		payload["mode"] = "specialist"
		payload["specialist_slug"] = specialistTask.SpecialistSlug
		payload["specialist_task_id"] = specialistTask.ID.String()
		if enriched, err := json.Marshal(payload); err == nil {
			event.Payload = enriched
		}
	} else if _, ok := payload["mode"]; !ok {
		payload["mode"] = "employee"
	}
	if event.EventType == "agent.run.model.usage" {
		if err := h.recordRuntimeModelUsageGeneration(ctx, sb, event, payload); err != nil {
			captureEmployeeWebhookIngest(ctx, "record_runtime_generation", sb, event, sessionID, source, err)
		}
	}
	if event.EventType == "skill.synced" {
		if err := h.syncSkillEvent(ctx, sb, payload); err != nil {
			captureEmployeeWebhookIngest(ctx, "sync_skill", sb, event, sessionID, source, err)
		}
	}
	if event.EventType == "session.created" {
		session, createdSession, err := h.ensureEmployeeSession(ctx, sb, sessionID, source, payload, specialistTask)
		if err != nil {
			captureEmployeeWebhookIngest(ctx, "ensure_employee_session", sb, event, sessionID, source, err)
			return
		}
		if createdSession {
			h.enqueueEmployeeMemoryRetain(ctx, sb, session, sessionID, "session_created", "session.created")
		}
		return
	}
	if !shouldStoreEmployeeSessionEvent(event.EventType) {
		return
	}
	session, createdSession, err := h.ensureEmployeeSession(ctx, sb, sessionID, source, payload, specialistTask)
	if err != nil {
		captureEmployeeWebhookIngest(ctx, "ensure_employee_session", sb, event, sessionID, source, err)
		return
	}
	if createdSession {
		h.enqueueEmployeeMemoryRetain(ctx, sb, session, sessionID, "session_created", "session.created")
	}
	stored, ok := employeeSessionEventFromOutbound(sb, event, payload, session.ID, sessionID)
	if !ok {
		captureEmployeeWebhookIngest(ctx, "drop_missing_sandbox_owner", sb, event, sessionID, source, fmt.Errorf("employee sandbox missing org_id or employee_id"))
		return
	}
	if taskFound {
		stored.Mode = "specialist"
		stored.SpecialistSlug = specialistTask.SpecialistSlug
		stored.SpecialistTaskID = &specialistTask.ID
	} else {
		stored.Mode = "employee"
	}
	if h.writer != nil {
		h.writer.Write(ctx, stored)
	} else {
		err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&stored).Error; err != nil {
				return err
			}
			if err := syncEmployeeScheduleEvent(tx, stored); err != nil {
				captureEmployeeSessionEventFailure(ctx, "sync_schedule", stored, err)
			}
			return nil
		})
		if err != nil {
			captureEmployeeSessionEventFailure(ctx, "store_memory_event", stored, err)
			return
		}
	}
	if event.EventType == "agent.message.sent" && source == gateway.Source && h.gateway != nil {
		if _, err := h.gateway.HandleRuntimeFinal(ctx, gateway.AgentResponse{
			EmployeeSession:  *session,
			RuntimeSessionID: sessionID,
			TraceID:          stringValue(payload, "trace_id"),
			TurnID:           stringValue(payload, "turn_id"),
			ChannelID:        stringValue(payload, "channel_id"),
			ThreadID:         stringValue(payload, "thread_id"),
			Text:             stringValue(payload, "text"),
			Raw:              payload,
		}); err != nil {
			captureEmployeeWebhookIngest(ctx, "gateway_deliver_runtime_final", sb, event, sessionID, source, err)
		}
	}
	if event.EventType == "session.completed" {
		h.markEmployeeSessionEnded(ctx, session.ID, event.At)
	}
}

func (h *EmployeeOutboundWebhookHandler) enqueueEmployeeMemoryRetain(ctx context.Context, sb *model.Sandbox, session *model.EmployeeSession, sessionID, reason, sourceEvent string) {
	if h.enqueuer == nil || sb == nil || session == nil || sb.EmployeeID == nil || sessionID == "" {
		return
	}
	task, err := tasks.NewEmployeeMemoryRetainTask(tasks.EmployeeMemoryRetainPayload{
		EmployeeID:        *sb.EmployeeID,
		SandboxID:         sb.ID,
		EmployeeSessionID: session.ID,
		SessionID:         sessionID,
		Reason:            reason,
		SourceEvent:       sourceEvent,
	})
	if err != nil {
		captureEmployeeWebhookIngest(ctx, "build_memory_retain_task", sb, nil, sessionID, session.Source, err)
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.ProcessIn(10*time.Minute),
		asynq.TaskID("employee-memory-retain:"+session.ID.String()),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		captureEmployeeWebhookIngest(ctx, "enqueue_memory_retain", sb, nil, sessionID, session.Source, err)
	}
}

func (h *EmployeeOutboundWebhookHandler) specialistTaskForSandbox(ctx context.Context, sandboxID uuid.UUID) (*model.SpecialistTask, bool) {
	var task model.SpecialistTask
	if err := h.db.WithContext(ctx).
		Where("sandbox_id = ?", sandboxID).
		Order("created_at DESC").
		First(&task).Error; err != nil {
		return nil, false
	}
	return &task, true
}
