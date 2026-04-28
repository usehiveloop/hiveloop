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

type EmbedderConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Dim        uint32
	Timeout    time.Duration
	MaxRetries int
}

type Embedder struct {
	cfg  EmbedderConfig
	http *http.Client
}

func NewEmbedder(cfg EmbedderConfig) *Embedder {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Embedder{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (e *Embedder) Dim() uint32 { return e.cfg.Dim }

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (e *Embedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	payload := map[string]any{
		"model": e.cfg.Model,
		"input": inputs,
	}
	if e.cfg.Dim > 0 {
		payload["dimensions"] = e.cfg.Dim
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("embed: marshal: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt <= e.cfg.MaxRetries; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
			e.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)

		resp, err := e.http.Do(req)
		if err != nil {
			lastErr = err
			backoff(attempt)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var out embedResponse
			if err := json.Unmarshal(respBody, &out); err != nil {
				return nil, fmt.Errorf("embed: decode: %w", err)
			}
			if out.Error != nil && out.Error.Message != "" {
				return nil, fmt.Errorf("embed: upstream error: %s (type=%s code=%s)",
					out.Error.Message, out.Error.Type, out.Error.Code)
			}
			if len(out.Data) != len(inputs) {
				preview := string(respBody)
				if len(preview) > 300 {
					preview = preview[:300]
				}
				return nil, fmt.Errorf("embed: got %d vectors for %d inputs (body: %s)",
					len(out.Data), len(inputs), preview)
			}
			vectors := make([][]float32, len(out.Data))
			for i := range out.Data {
				vectors[i] = out.Data[i].Embedding
			}
			return vectors, nil
		}
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		lastErr = fmt.Errorf("embed: %d: %s", resp.StatusCode, preview)
		// Don't retry on 4xx (apart from 429).
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			break
		}
		backoff(attempt)
	}
	return nil, lastErr
}

func backoff(attempt int) {
	d := 250 * time.Millisecond
	for i := 0; i < attempt; i++ {
		d *= 2
	}
	if d > 4*time.Second {
		d = 4 * time.Second
	}
	time.Sleep(d)
}
