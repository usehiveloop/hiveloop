package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/system"
)

func (h *SystemTaskHandler) pickCredential(ctx context.Context, task system.Task) (*model.Credential, error) {
	if task.ModelTier == system.ModelNamed {
		return h.picker.PickByModel(ctx, task.Model)
	}
	return h.picker.Pick(ctx, task.ProviderGroup)
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
	// If we already started writing the SSE stream the response body has
	// chunks in it; we can't switch to a JSON error. Emit a final SSE error
	// frame so the client knows the stream died.
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

// afterCompletion writes the cache entry and the generations row. Called
// after a successful upstream call, never on cache hit.
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
		ID:             "gen_" + ulid.Make().String(),
		OrgID:          orgID,
		CredentialID:   cred.ID,
		TokenJTI:       "system:" + taskName,
		ProviderID:     cred.ProviderID,
		Model:          modelID,
		RequestPath:    "/v1/system/tasks/" + taskName,
		IsStreaming:    streaming,
		InputTokens:    res.Usage.InputTokens,
		OutputTokens:   res.Usage.OutputTokens,
		CachedTokens:   res.Usage.CachedTokens,
		UpstreamStatus: http.StatusOK,
		UserID:         userID,
		CreatedAt:      time.Now(),
	}
	if err := h.db.WithContext(ctx).Create(&gen).Error; err != nil {
		_ = err
	}
	// Per-token cost lookup + credit Spend goes through the existing
	// billing pipeline (a follow-up wires this into BillingTokenSpend like
	// the proxy path already does).
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
