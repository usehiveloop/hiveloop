package paystack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.paystack.co"

// client is the tiny HTTP wrapper around Paystack's REST API. It handles the
// bearer auth header and the standard Paystack response envelope so endpoint
// files only deal with their own request/response shapes.
type client struct {
	secretKey string
	http      *http.Client
	baseURL   string // overridable from tests
}

// newClient builds a client with a 20s default timeout. Use an override via
// baseURL when testing against httptest servers.
func newClient(secretKey string) *client {
	return &client{
		secretKey: secretKey,
		http:      &http.Client{Timeout: 20 * time.Second},
		baseURL:   defaultBaseURL,
	}
}

// envelope is Paystack's standard response wrapper.
//
//	{"status": true, "message": "…", "data": { … }}
//
// Endpoints decode `data` into their own shapes via json.RawMessage.
type envelope struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// do executes a request against Paystack and unmarshals the envelope.
//
// When out is non-nil and the response is successful, Data is decoded into
// out. On HTTP or API failure, the returned error includes Paystack's own
// message so callers can log it verbatim ("Customer already exists", etc.).
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("paystack %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap is ample for Paystack
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("paystack %s %s: http %d: %s",
			method, path, resp.StatusCode, truncate(string(raw), 200))
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if !env.Status {
		return fmt.Errorf("paystack %s %s: %s", method, path, env.Message)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
