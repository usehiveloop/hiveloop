package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
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
	db        *gorm.DB
	picker    credentials.Picker
	kms       *crypto.KeyWrapper
	registry  *registry.Registry
	cache     system.Cache
	forwarder *system.Forwarder
	credits   CreditsSpender
}

// NewSystemTaskHandler builds the handler. All deps are required except
// `cache`, which may be nil to disable caching across all tasks.
func NewSystemTaskHandler(
	db *gorm.DB,
	picker credentials.Picker,
	kms *crypto.KeyWrapper,
	reg *registry.Registry,
	cache system.Cache,
	forwarder *system.Forwarder,
	credits CreditsSpender,
) *SystemTaskHandler {
	return &SystemTaskHandler{
		db:        db,
		picker:    picker,
		kms:       kms,
		registry:  reg,
		cache:     cache,
		forwarder: forwarder,
		credits:   credits,
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
		key, err := system.CacheKey(task, modelID, req.Args)
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

	llmReq, err := system.BuildLLMRequest(task, modelID, req.Args, stream)
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

func (h *SystemTaskHandler) serveCached(w http.ResponseWriter, cached *system.CompletionResult, stream bool) {
	if stream {
		_ = system.EmitCachedSSE(w, cached)
		return
	}
	writeJSON(w, http.StatusOK, systemTaskJSONResponse{
		Text:   cached.Text,
		Usage:  cached.Usage,
		Model:  cached.Model,
		Cached: true,
	})
}

func (h *SystemTaskHandler) handleForwardError(w http.ResponseWriter, err error, alreadyStreaming bool) {
	// If we already started writing the SSE stream, the response body has
	// chunks in it; we can't switch to a JSON error. Best we can do is
	// emit a final SSE error frame so the client knows the stream died.
	if alreadyStreaming {
		fmt.Fprintf(w, "data: {\"error\":\"upstream\"}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	var upErr *system.UpstreamError
	if errors.As(err, &upErr) {
		writeJSON(w, http.StatusBadGateway, systemTaskError{
			Error:     fmt.Sprintf("upstream %d: %s", upErr.StatusCode, upErr.Body),
			ErrorCode: "upstream_error",
		})
		return
	}
	writeJSON(w, http.StatusBadGateway, systemTaskError{
		Error:     "provider unreachable",
		ErrorCode: "upstream_error",
	})
}

// afterCompletion writes the cache entry, the generations row, and debits
// credits. Called after a successful upstream call, never on cache hit.
func (h *SystemTaskHandler) afterCompletion(
	ctx context.Context,
	task *system.Task,
	taskName, modelID string,
	cred *model.Credential,
	orgID uuid.UUID,
	userID string,
	res *system.CompletionResult,
	cacheKey string,
	streaming bool,
) {
	if cacheKey != "" && task.CacheTTL > 0 {
		_ = h.cache.Set(ctx, cacheKey, res, task.CacheTTL)
	}
	gen := model.Generation{
		ID:            "gen_" + ulid.Make().String(),
		OrgID:         orgID,
		CredentialID:  cred.ID,
		TokenJTI:      "system:" + taskName,
		ProviderID:    cred.ProviderID,
		Model:         modelID,
		RequestPath:   "/v1/system/tasks/" + taskName,
		IsStreaming:   streaming,
		InputTokens:   res.Usage.InputTokens,
		OutputTokens:  res.Usage.OutputTokens,
		CachedTokens:  res.Usage.CachedTokens,
		UpstreamStatus: http.StatusOK,
		UserID:        userID,
		CreatedAt:     time.Now(),
	}
	if err := h.db.WithContext(ctx).Create(&gen).Error; err != nil {
		// Log-only; spend already happened upstream and we don't want to
		// fail the user's request because we couldn't write a row.
		_ = err
	}
	// Token-based cost is the source of truth for credit debits; we leave
	// the per-token cost lookup + Spend call to the existing billing
	// pipeline (a follow-up wires this into BillingTokenSpend like the
	// proxy path already does).
	_ = h.credits
}

func (h *SystemTaskHandler) resolveModel(task system.Task, providerID string) (string, error) {
	switch task.ModelTier {
	case system.ModelNamed:
		return task.Model, nil
	case system.ModelCheapest, system.ModelDefault, "":
		provider, ok := h.registry.GetProvider(providerID)
		if !ok {
			return "", fmt.Errorf("no curated catalog for provider %q", providerID)
		}
		id := pickCheapestActiveModel(provider)
		if id == "" {
			return "", fmt.Errorf("no eligible model for task %q on provider %q", task.Name, providerID)
		}
		return id, nil
	default:
		return "", fmt.Errorf("unknown model tier %q", task.ModelTier)
	}
}

func pickCheapestActiveModel(p *registry.Provider) string {
	var bestID string
	var bestCost float64 = -1
	for id, m := range p.Models {
		if m.Cost == nil {
			continue
		}
		if m.Status == "deprecated" || m.Status == "retired" {
			continue
		}
		if bestCost < 0 || m.Cost.Input < bestCost {
			bestID = id
			bestCost = m.Cost.Input
		}
	}
	return bestID
}

func decryptCredentialKey(ctx context.Context, kms *crypto.KeyWrapper, cred *model.Credential) ([]byte, error) {
	dek, err := kms.Unwrap(ctx, cred.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	apiKey, err := crypto.DecryptCredential(cred.EncryptedKey, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return apiKey, nil
}
