package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// HTTPTriggerHandler receives HTTP requests at /incoming/triggers/{triggerID}
// and dispatches them through the router pipeline. Unlike provider webhooks,
// these bypass connection/event-key matching — the trigger is already known.
//
// Security: the trigger's unguessable UUID acts as a bearer token. If a
// SecretKey is configured, the handler also verifies HMAC-SHA256 via the
// X-Signature-256 header.
type HTTPTriggerHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

// NewHTTPTriggerHandler creates an HTTP trigger handler.
func NewHTTPTriggerHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *HTTPTriggerHandler {
	return &HTTPTriggerHandler{db: db, enqueuer: enqueuer}
}

// Handle processes POST /incoming/triggers/{triggerID}.
// @Summary Receive HTTP trigger request
// @Description Receives an HTTP request and dispatches it through the router pipeline for the specified trigger. The trigger UUID acts as a bearer token. If the trigger has a secret key configured, the request must include a valid HMAC-SHA256 signature in the X-Signature-256 header.
// @Tags triggers
// @Accept json
// @Produce json
// @Param triggerID path string true "Trigger UUID"
// @Param X-Signature-256 header string false "HMAC-SHA256 signature (sha256=hex). Required when the trigger has a secret key configured."
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse "Invalid or missing HMAC signature"
// @Failure 404 {object} errorResponse
// @Router /incoming/triggers/{triggerID} [post]
func (handler *HTTPTriggerHandler) Handle(writer http.ResponseWriter, request *http.Request) {
	triggerIDStr := chi.URLParam(request, "triggerID")

	triggerID, err := uuid.Parse(triggerIDStr)
	if err != nil {
		slog.Warn("http trigger: invalid trigger ID", "trigger_id_raw", triggerIDStr)
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: "invalid trigger ID"})
		return
	}

	// Load the trigger and verify it's an HTTP trigger.
	var trigger model.RouterTrigger
	if err := handler.db.Where("id = ? AND enabled = TRUE", triggerID).First(&trigger).Error; err != nil {
		slog.Warn("http trigger: trigger not found",
			"trigger_id", triggerID,
			"error", err,
		)
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}

	if trigger.TriggerType != "http" {
		slog.Warn("http trigger: wrong trigger type",
			"trigger_id", triggerID,
			"trigger_type", trigger.TriggerType,
		)
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}

	// Read the request body.
	body, err := io.ReadAll(request.Body)
	if err != nil {
		slog.Error("http trigger: failed to read body",
			"trigger_id", triggerID,
			"error", err,
		)
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: "failed to read body"})
		return
	}

	// If no body is provided, use an empty JSON object so the dispatcher
	// can still extract refs and evaluate conditions.
	if len(body) == 0 {
		body = []byte("{}")
	}

	// Verify HMAC signature if SecretKey is configured.
	if trigger.SecretKey != "" {
		signature := request.Header.Get("X-Signature-256")
		if signature == "" {
			slog.Warn("http trigger: missing signature",
				"trigger_id", triggerID,
			)
			writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "missing X-Signature-256 header"})
			return
		}
		if !verifyHMAC(body, trigger.SecretKey, signature) {
			slog.Warn("http trigger: invalid signature",
				"trigger_id", triggerID,
			)
			writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "invalid signature"})
			return
		}
	}

	slog.Info("http trigger: received",
		"trigger_id", triggerID,
		"org_id", trigger.OrgID,
		"body_size", len(body),
	)

	// Return 200 immediately, then dispatch asynchronously.
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})

	deliveryID := triggerID.String() + ":" + uuid.New().String()
	task, err := tasks.NewRouterDispatchTask(tasks.TriggerDispatchPayload{
		Provider:        "http",
		EventType:       "http",
		EventAction:     "",
		DeliveryID:      deliveryID,
		OrgID:           trigger.OrgID,
		PayloadJSON:     body,
		RouterTriggerID: &triggerID,
	})
	if err != nil {
		slog.Error("http trigger: failed to build dispatch task",
			"trigger_id", triggerID,
			"error", err,
		)
		return
	}

	if _, enqueueErr := handler.enqueuer.Enqueue(task); enqueueErr != nil {
		slog.Error("http trigger: failed to enqueue dispatch task",
			"trigger_id", triggerID,
			"error", enqueueErr,
		)
		return
	}

	slog.Info("http trigger: dispatched",
		"trigger_id", triggerID,
		"delivery_id", deliveryID,
	)
}

// verifyHMAC checks an HMAC-SHA256 signature. The expected format is
// "sha256=<hex>" (GitHub-style) or just the raw hex digest.
func verifyHMAC(body []byte, secret, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	// Strip "sha256=" prefix if present.
	cleaned := signature
	if len(cleaned) > 7 && cleaned[:7] == "sha256=" {
		cleaned = cleaned[7:]
	}

	return hmac.Equal([]byte(expected), []byte(cleaned))
}
