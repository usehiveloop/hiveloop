package handler

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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
// shared secret is configured (SecretKey stores its bcrypt hash), the handler
// also requires the plaintext secret in any of:
//   Authorization: Bearer <secret>, X-Api-Key, X-Webhook-Secret, ?secret=<secret>
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
// @Description Receives an HTTP request and dispatches it through the router pipeline for the specified trigger. The trigger UUID acts as a bearer token. If the trigger has a shared secret configured, the request must include the plaintext secret in any of: Authorization: Bearer <secret>, X-Api-Key, X-Webhook-Secret, or ?secret=<secret>.
// @Tags triggers
// @Accept json
// @Produce json
// @Param triggerID path string true "Trigger UUID"
// @Param Authorization header string false "Bearer <secret>. One of the accepted ways to send the trigger's shared secret."
// @Param X-Api-Key header string false "Plaintext shared secret. One of the accepted auth header names."
// @Param X-Webhook-Secret header string false "Plaintext shared secret. One of the accepted auth header names."
// @Param secret query string false "Plaintext shared secret as a query param. Last-resort transport when headers can't be customized."
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse "Missing or invalid shared secret"
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

	// Verify shared secret if configured. SecretKey stores a bcrypt hash; the
	// caller sends plaintext via one of the accepted transports.
	if trigger.SecretKey != "" {
		provided := extractTriggerSecret(request)
		if provided == "" {
			slog.Warn("http trigger: missing secret",
				"trigger_id", triggerID,
			)
			writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "missing shared secret"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(trigger.SecretKey), []byte(provided)); err != nil {
			slog.Warn("http trigger: invalid secret",
				"trigger_id", triggerID,
			)
			writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "invalid shared secret"})
			return
		}
	}

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

// extractTriggerSecret pulls the plaintext shared secret from the first place
// it finds it. Order: Authorization: Bearer, X-Api-Key, X-Webhook-Secret,
// ?secret= query param. Returns "" when none are set.
func extractTriggerSecret(request *http.Request) string {
	if auth := request.Header.Get("Authorization"); auth != "" {
		if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
			if trimmed := strings.TrimSpace(token); trimmed != "" {
				return trimmed
			}
		}
	}
	if value := strings.TrimSpace(request.Header.Get("X-Api-Key")); value != "" {
		return value
	}
	if value := strings.TrimSpace(request.Header.Get("X-Webhook-Secret")); value != "" {
		return value
	}
	if value := strings.TrimSpace(request.URL.Query().Get("secret")); value != "" {
		return value
	}
	return ""
}
