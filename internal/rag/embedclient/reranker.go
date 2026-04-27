package embedclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RerankerConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	MaxRetries int
}

type Reranker struct {
	cfg  RerankerConfig
	http *http.Client
}

func NewReranker(cfg RerankerConfig) *Reranker {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Reranker{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

type RerankResult struct {
	Index int     `json:"index"`
	Score float64 `json:"relevance_score"`
}

type rerankResponse struct {
	Results []RerankResult `json:"results"`
}

// Rerank reorders documents by their relevance to the query. The returned
// indices reference the input slice in descending score order; len(out) <= topN.
func (r *Reranker) Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	if topN <= 0 || topN > len(documents) {
		topN = len(documents)
	}
	body, err := json.Marshal(map[string]any{
		"model":     r.cfg.Model,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	})
	if err != nil {
		return nil, fmt.Errorf("rerank: marshal: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt <= r.cfg.MaxRetries; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
			r.cfg.BaseURL+"/rerank", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)

		resp, err := r.http.Do(req)
		if err != nil {
			lastErr = err
			backoff(attempt)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var out rerankResponse
			if err := json.Unmarshal(respBody, &out); err != nil {
				return nil, fmt.Errorf("rerank: decode: %w", err)
			}
			return out.Results, nil
		}
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		lastErr = fmt.Errorf("rerank: %d: %s", resp.StatusCode, preview)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			break
		}
		backoff(attempt)
	}
	return nil, lastErr
}
