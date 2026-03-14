package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// VerifyResult contains the outcome of a provider key verification.
type VerifyResult struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// Verify checks if an API key is valid by making a real inference call
// to the cheapest model available for the provider.
func (r *Registry) Verify(ctx context.Context, providerID, baseURL, authScheme string, apiKey []byte) VerifyResult {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return VerifyResult{Valid: false, Error: "unknown provider"}
	}

	model := cheapestModel(provider)
	if model == nil {
		return VerifyResult{Valid: false, Error: "no model available for verification"}
	}

	return verifyWithInference(ctx, providerID, baseURL, authScheme, apiKey, model.ID)
}

// cheapestModel picks the model with the lowest input cost.
// If no models have cost data, returns any model.
func cheapestModel(provider *Provider) *Model {
	if len(provider.Models) == 0 {
		return nil
	}

	var best *Model
	bestCost := math.MaxFloat64

	for _, m := range provider.Models {
		if m.Cost != nil && m.Cost.Input > 0 && m.Cost.Input < bestCost {
			bestCost = m.Cost.Input
			picked := m
			best = &picked
		}
	}

	// No cost data — just pick any model.
	if best == nil {
		for _, m := range provider.Models {
			picked := m
			return &picked
		}
	}

	return best
}

func verifyWithInference(ctx context.Context, providerID, baseURL, authScheme string, apiKey []byte, modelID string) VerifyResult {
	base := strings.TrimRight(baseURL, "/")

	var url string
	var body map[string]any
	var extraHeaders map[string]string

	switch providerID {
	case "anthropic":
		url = base + "/v1/messages"
		body = map[string]any{
			"model":      modelID,
			"messages":   []map[string]string{{"role": "user", "content": "hi"}},
			"max_tokens": 1,
		}
		extraHeaders = map[string]string{"anthropic-version": "2023-06-01"}

	case "google":
		url = fmt.Sprintf("%s/v1beta/models/%s:generateContent", base, modelID)
		body = map[string]any{
			"contents": []map[string]any{
				{"parts": []map[string]string{{"text": "hi"}}},
			},
			"generationConfig": map[string]any{"maxOutputTokens": 1},
		}

	case "cohere":
		url = base + "/v2/chat"
		body = map[string]any{
			"model":    modelID,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		}

	default:
		// OpenAI-compatible (vast majority of providers).
		// Most registry base URLs already end with /v1, so avoid doubling it.
		if strings.HasSuffix(base, "/v1") {
			url = base + "/chat/completions"
		} else {
			url = base + "/v1/chat/completions"
		}
		body = map[string]any{
			"model":      modelID,
			"messages":   []map[string]string{{"role": "user", "content": "hi"}},
			"max_tokens": 1,
		}
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return VerifyResult{Valid: false, Error: "failed to create request"}
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	attachAuth(req, authScheme, apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return VerifyResult{Valid: false, Error: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return VerifyResult{Valid: true}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return VerifyResult{Valid: false, Error: "invalid API key"}
	}
	return VerifyResult{Valid: false, Error: fmt.Sprintf("unexpected status: %d", resp.StatusCode)}
}

func attachAuth(req *http.Request, scheme string, apiKey []byte) {
	switch scheme {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+string(apiKey))
	case "x-api-key":
		req.Header.Set("x-api-key", string(apiKey))
	case "api-key":
		req.Header.Set("api-key", string(apiKey))
	case "query_param":
		q := req.URL.Query()
		q.Set("key", string(apiKey))
		req.URL.RawQuery = q.Encode()
	}
}
