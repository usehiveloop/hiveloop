package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

const maxHTTPTriggerBodyBytes int64 = 256 * 1024

var httpTriggerRedactedValue = "[redacted]"

// HTTPTriggerHandler receives HTTP requests at /incoming/triggers/{triggerID}
// and dispatches them to the owning employee runtime. Unlike provider webhooks,
// these bypass connection/event-key matching — the trigger is already known.
//
// Security: the trigger's unguessable UUID acts as a bearer token. If a
// shared secret is configured (SecretKey stores its bcrypt hash), the handler
// also requires the plaintext secret in any of:
//
//	Authorization: Bearer <secret>, X-Api-Key, X-Webhook-Secret, ?secret=<secret>
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
// @Description Receives an HTTP request and dispatches it to the owning employee runtime for the specified trigger. The trigger UUID acts as a bearer token. If the trigger has a shared secret configured, the request must include the plaintext secret in any of: Authorization: Bearer <secret>, X-Api-Key, X-Webhook-Secret, or ?secret=<secret>.
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
// @Failure 413 {object} errorResponse
// @Router /incoming/triggers/{triggerID} [post]
func (handler *HTTPTriggerHandler) Handle(writer http.ResponseWriter, request *http.Request) {
	triggerIDStr := chi.URLParam(request, "triggerID")

	triggerID, err := uuid.Parse(triggerIDStr)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: "invalid trigger ID"})
		return
	}

	var trigger model.EmployeeTrigger
	if err := handler.db.
		Joins("JOIN employees ON employees.id = employee_triggers.employee_id").
		Where("employee_triggers.id = ? AND employee_triggers.enabled = TRUE AND employee_triggers.trigger_type = ? AND employees.status <> ?", triggerID, "http", "archived").
		First(&trigger).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}

	if trigger.SecretKey != "" {
		provided := extractTriggerSecret(request)
		if provided == "" {
			writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "missing shared secret"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(trigger.SecretKey), []byte(provided)); err != nil {
			writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "invalid shared secret"})
			return
		}
	}

	bodyReader := http.MaxBytesReader(writer, request.Body, maxHTTPTriggerBodyBytes)
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		logging.FromContext(request.Context()).ErrorContext(request.Context(), "http trigger: failed to read body",
			"trigger_id", triggerID,
			"error", err,
		)
		if strings.Contains(err.Error(), "request body too large") {
			writeJSON(writer, http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
			return
		}
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: "failed to read body"})
		return
	}

	if len(body) == 0 {
		body = []byte("{}")
	}
	body = sanitizeHTTPTriggerPayload(body, request.Header.Get("Content-Type"))

	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})

	deliveryID := triggerID.String() + ":" + uuid.New().String()
	task, err := tasks.NewEmployeeTriggerDispatchTask(tasks.EmployeeTriggerDispatchPayload{
		Provider:    "http",
		EventType:   "http",
		DeliveryID:  deliveryID,
		OrgID:       trigger.OrgID,
		PayloadJSON: body,
		TriggerID:   &triggerID,
	})
	if err != nil {
		logging.FromContext(request.Context()).ErrorContext(request.Context(), "http trigger: failed to build dispatch task",
			"trigger_id", triggerID,
			"error", err,
		)
		logging.CaptureWithFields(request.Context(), fmt.Errorf("http trigger: failed to build dispatch task: %w", err), map[string]any{
			"org_id":      trigger.OrgID.String(),
			"employee_id": trigger.EmployeeID.String(),
			"trigger_id":  triggerID.String(),
			"delivery_id": deliveryID,
			"event_key":   "http",
		})
		return
	}

	if _, enqueueErr := handler.enqueuer.Enqueue(task); enqueueErr != nil {
		logging.FromContext(request.Context()).ErrorContext(request.Context(), "http trigger: failed to enqueue dispatch task",
			"trigger_id", triggerID,
			"error", enqueueErr,
		)
		logging.CaptureWithFields(request.Context(), fmt.Errorf("http trigger: failed to enqueue dispatch task: %w", enqueueErr), map[string]any{
			"org_id":      trigger.OrgID.String(),
			"employee_id": trigger.EmployeeID.String(),
			"trigger_id":  triggerID.String(),
			"delivery_id": deliveryID,
			"event_key":   "http",
		})
		return
	}
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

func sanitizeHTTPTriggerPayload(body []byte, contentType string) []byte {
	if !strings.Contains(strings.ToLower(contentType), "json") && !json.Valid(body) {
		return body
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return body
	}
	redacted := redactSensitiveJSON(decoded)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return body
	}
	return encoded
}

func redactSensitiveJSON(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			if isSensitivePayloadKey(key) {
				out[key] = httpTriggerRedactedValue
				continue
			}
			out[key] = redactSensitiveJSON(nested)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, nested := range typed {
			out[index] = redactSensitiveJSON(nested)
		}
		return out
	default:
		return typed
	}
}

func isSensitivePayloadKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	for _, marker := range []string{"authorization", "password", "secret", "token", "api_key", "apikey", "credential"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
