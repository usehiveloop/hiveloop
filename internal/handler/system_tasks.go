package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/system"
)

// CreditsSpender is the slice of billing.CreditsService the handler depends
// on. Kept narrow so tests can fake it without dragging in the full ledger.
type CreditsSpender interface {
	Spend(orgID uuid.UUID, amount int64, reason, refType, refID string) error
}

// SystemTaskHandler dispatches /v1/system/tasks/{taskName} requests.
type SystemTaskHandler struct {
	db             *gorm.DB
	picker         credentials.Picker
	kms            *crypto.KeyWrapper
	registry       *registry.Registry
	cache          system.Cache
	forwarder      *system.Forwarder
	credits        CreditsSpender
	actionsCatalog *catalog.Catalog
}

// NewSystemTaskHandler builds the handler. All deps are required except
// `cache`, which may be nil to disable caching across all tasks, and
// `actionsCatalog`, which may be nil for tasks that don't need it.
func NewSystemTaskHandler(
	db *gorm.DB,
	picker credentials.Picker,
	kms *crypto.KeyWrapper,
	reg *registry.Registry,
	cache system.Cache,
	forwarder *system.Forwarder,
	credits CreditsSpender,
	actionsCatalog *catalog.Catalog,
) *SystemTaskHandler {
	return &SystemTaskHandler{
		db:             db,
		picker:         picker,
		kms:            kms,
		registry:       reg,
		cache:          cache,
		forwarder:      forwarder,
		credits:        credits,
		actionsCatalog: actionsCatalog,
	}
}

type systemTaskRequest struct {
	Stream *bool          `json:"stream,omitempty"`
	Args   map[string]any `json:"args"`
}

type systemTaskJSONResponse struct {
	Text   string       `json:"text"`
	Usage  system.Usage `json:"usage"`
	Model  string       `json:"model"`
	Cached bool         `json:"cached,omitempty"`
}

// systemTaskError is the typed error envelope this endpoint returns.
// `error_code` is a stable machine-readable string that the frontend can
// switch on (e.g. distinguishing "system_credential_unavailable" from a
// generic 5xx). The plain `errorResponse` used elsewhere in this package
// has no code field — callers of /v1/system/tasks/{taskName} get the
// richer shape instead.
type systemTaskError struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code,omitempty"`
}

// Run is the HTTP handler entry point.
//
// @Summary Run a system task
// @Description Executes a registered server-side LLM task using platform credentials. Each task name maps to a hard-coded definition (model tier, prompt, args). Caller may opt into streaming.
// @Tags system
// @Accept json
// @Produce json
// @Produce text/event-stream
// @Param taskName path string true "Task name"
// @Param body body systemTaskRequest true "Task arguments"
// @Success 200 {object} systemTaskJSONResponse
// @Success 200 {string} string "SSE stream when stream=true"
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/system/tasks/{taskName} [post]
func (h *SystemTaskHandler) Run(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "taskName")
	task, ok := system.Lookup(taskName)
	if !ok {
		writeJSON(w, http.StatusNotFound, systemTaskError{
			Error:     fmt.Sprintf("system task %q is not defined", taskName),
			ErrorCode: "task_not_found",
		})
		return
	}

	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, systemTaskError{
			Error:     "missing auth claims",
			ErrorCode: "unauthorized",
		})
		return
	}
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, systemTaskError{
			Error:     "invalid org id in token",
			ErrorCode: "unauthorized",
		})
		return
	}

	var req systemTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, systemTaskError{
			Error:     "invalid request body",
			ErrorCode: "invalid_request_body",
		})
		return
	}

	if errs := system.ValidateArgs(req.Args, task.Args); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, systemTaskError{
			Error:     errs[0].Error(),
			ErrorCode: "invalid_args",
		})
		return
	}

	// Resolve runs after validation and before template rendering. Tasks use
	// it to translate IDs into rich data via org-scoped DB lookups (skill IDs
	// → name+description, integration connection IDs → provider + actions
	// catalog, etc.). Tasks without a resolver pass req.Args through
	// unchanged.
	resolvedArgs := req.Args
	if task.Resolve != nil {
		out, err := task.Resolve(r.Context(), system.ResolveDeps{
			DB:             h.db,
			OrgID:          orgID,
			Registry:       h.registry,
			ActionsCatalog: h.actionsCatalog,
		}, req.Args)
		if err != nil {
			var rerr *system.ResolveError
			if errors.As(err, &rerr) {
				writeJSON(w, http.StatusBadRequest, systemTaskError{
					Error:     rerr.Message,
					ErrorCode: rerr.Code,
				})
				return
			}
			writeJSON(w, http.StatusBadRequest, systemTaskError{
				Error:     err.Error(),
				ErrorCode: "invalid_args",
			})
			return
		}
		resolvedArgs = out
	}

	stream := task.DefaultStream
	if req.Stream != nil {
		stream = *req.Stream
	}

	cred, err := h.picker.Pick(r.Context(), task.ProviderGroup)
	if err != nil {
		if errors.Is(err, credentials.ErrNoSystemCredential) {
			writeJSON(w, http.StatusServiceUnavailable, systemTaskError{
				Error:     fmt.Sprintf("no platform credential available for provider group %q", task.ProviderGroup),
				ErrorCode: "system_credential_unavailable",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, systemTaskError{
			Error:     "failed to resolve system credential",
			ErrorCode: "internal_error",
		})
		return
	}

	modelID, err := h.resolveModel(task, cred.ProviderID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, systemTaskError{
			Error:     err.Error(),
			ErrorCode: "system_model_unavailable",
		})
		return
	}

	cacheKey := ""
	if h.cache != nil && task.CacheTTL > 0 {
		key, err := system.CacheKey(task, modelID, resolvedArgs)
		if err == nil {
			cacheKey = key
			if hit, ok, _ := h.cache.Get(r.Context(), key); ok {
				h.serveCached(w, hit, stream)
				return
			}
		}
	}

	apiKey, err := decryptCredentialKey(r.Context(), h.kms, cred)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, systemTaskError{
			Error:     "failed to decrypt system credential",
			ErrorCode: "internal_error",
		})
		return
	}

	llmReq, err := system.BuildLLMRequest(task, modelID, resolvedArgs, stream)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, systemTaskError{
			Error:     "failed to build LLM request",
			ErrorCode: "internal_error",
		})
		return
	}

	call := system.ForwardCall{
		BaseURL:    cred.BaseURL,
		APIKey:     string(apiKey),
		AuthScheme: cred.AuthScheme,
		Request:    llmReq,
		Stream:     stream,
	}

	if stream {
		res, err := h.forwarder.ForwardStream(r.Context(), call, w)
		if err != nil {
			h.handleForwardError(w, err, true)
			return
		}
		h.afterCompletion(r.Context(), &task, taskName, modelID, cred, orgID, claims.UserID, res, cacheKey, true)
		return
	}

	res, err := h.forwarder.ForwardJSON(r.Context(), call)
	if err != nil {
		h.handleForwardError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, systemTaskJSONResponse{
		Text:  res.Text,
		Usage: res.Usage,
		Model: res.Model,
	})
	h.afterCompletion(r.Context(), &task, taskName, modelID, cred, orgID, claims.UserID, res, cacheKey, false)
}

// (Helper methods serveCached, handleForwardError, afterCompletion,
// resolveModel, pickCheapestActiveModel, and decryptCredentialKey live in
// system_tasks_helpers.go.)
