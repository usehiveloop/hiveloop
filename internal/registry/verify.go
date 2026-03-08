package registry

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// VerifyResult contains the outcome of a provider key verification.
type VerifyResult struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// verificationEndpoints maps provider IDs to lightweight GET endpoints
// that return 200 with a valid API key.
var verificationEndpoints = map[string]string{
	"openai":       "/v1/models",
	"anthropic":    "/v1/models",
	"google":       "/v1beta/models",
	"groq":         "/openai/v1/models",
	"mistral":      "/v1/models",
	"cohere":       "/v2/models",
	"fireworks-ai": "/inference/v1/models",
	"togetherai":   "/v1/models",
	"perplexity":   "/v1/models",
	"xai":          "/v1/models",
	"deepinfra":    "/v1/models",
	"deepseek":     "/v1/models",
	"openrouter":   "/models",
	"cerebras":     "/v1/models",
}

// Verify checks if an API key is valid for a given provider by making
// a lightweight HTTP request to the provider's verification endpoint.
func Verify(ctx context.Context, providerID, baseURL, authScheme string, apiKey []byte) VerifyResult {
	endpoint, ok := verificationEndpoints[providerID]
	if !ok {
		endpoint = "/v1/models"
	}

	target := strings.TrimRight(baseURL, "/") + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return VerifyResult{Valid: false, Error: "failed to create request"}
	}

	attachVerifyAuth(req, authScheme, apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return VerifyResult{Valid: false, Error: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return VerifyResult{Valid: true}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return VerifyResult{Valid: false, Error: "invalid API key"}
	}
	return VerifyResult{Valid: false, Error: fmt.Sprintf("unexpected status: %d", resp.StatusCode)}
}

func attachVerifyAuth(req *http.Request, scheme string, apiKey []byte) {
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
