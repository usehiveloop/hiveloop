package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/system"
)

// ---------------------------------------------------------------------------
// Happy paths
// ---------------------------------------------------------------------------

func TestSystemTask_NonStreaming_HappyPath(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse(promptWriterPayload, 42, 18)))
	})

	rr := h.post(t, "prompt_writer", map[string]any{
		"args": validArgs(),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Text   string       `json:"text"`
		Usage  system.Usage `json:"usage"`
		Model  string       `json:"model"`
		Cached bool         `json:"cached"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// prompt_writer streams markdown — the response text is the prompt
	// itself. Sanity-check it has the required Role section.
	if !strings.Contains(resp.Text, "## Role") {
		t.Fatalf("LLM payload missing '## Role' section:\n%s", resp.Text)
	}
	if resp.Usage.InputTokens != 42 || resp.Usage.OutputTokens != 18 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if resp.Cached {
		t.Fatalf("first call should not be cached")
	}
	if got := atomic.LoadInt32(h.hits); got != 1 {
		t.Fatalf("upstream hits = %d, want 1", got)
	}
}

func TestSystemTask_Streaming_RewritesToHiveloopShape(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"model":"m","choices":[{"delta":{"content":"`+escapeJSON("## Role\nYou are deploy-watcher.\n\n")+`"}}]}

data: {"model":"m","choices":[{"delta":{"content":"`+escapeJSON("## Workflow\n1. Receive deployment_id.\n")+`"}}]}

data: {"model":"m","choices":[{"delta":{}}],"usage":{"prompt_tokens":11,"completion_tokens":7}}

data: [DONE]

`)
	})

	stream := true
	rr := h.post(t, "prompt_writer", map[string]any{
		"args":   validArgs(),
		"stream": stream,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}
	deltas, done := parseHiveloopSSE(t, rr.Body.String())
	if len(deltas) != 2 {
		t.Fatalf("delta count = %d, want 2; body=%q", len(deltas), rr.Body.String())
	}
	full := strings.Join(deltas, "")
	if !strings.Contains(full, "## Role") || !strings.Contains(full, "## Workflow") {
		t.Fatalf("streamed markdown missing expected sections:\n%s", full)
	}
	if !done.Done {
		t.Fatalf("done frame missing")
	}
	if done.Usage.OutputTokens != 7 {
		t.Fatalf("usage in done = %+v", done.Usage)
	}
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestSystemTask_NoSystemCredential_Returns503(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if err := h.db.Where("is_system = ?", true).Delete(&model.Credential{}).Error; err != nil {
		t.Fatalf("wipe creds: %v", err)
	}

	rr := h.post(t, "prompt_writer", map[string]any{"args": validArgs()})
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var er struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &er)
	if er.ErrorCode != "system_credential_unavailable" {
		t.Fatalf("error_code = %q (body=%s)", er.ErrorCode, rr.Body.String())
	}
	if got := atomic.LoadInt32(h.hits); got != 0 {
		t.Fatalf("upstream should NOT be called when no cred; hits=%d", got)
	}
}

func TestSystemTask_RevokedCredentialIgnored(t *testing.T) {
	revokedAt := time.Now().Add(-time.Hour)
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse(promptWriterPayload, 5, 5)))
	})
	if err := h.db.Model(&model.Credential{}).Where("is_system = ?", true).Update("revoked_at", &revokedAt).Error; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	kms := newSystemTaskKMS(t)
	active := seedSystemCredential(t, h.db, kms, h.upstream.URL+"/v1", "openai")
	t.Cleanup(func() { h.db.Where("id = ?", active.ID).Delete(&model.Credential{}) })

	rr := h.post(t, "prompt_writer", map[string]any{"args": validArgs()})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if got := atomic.LoadInt32(h.hits); got != 1 {
		t.Fatalf("hits = %d (must be 1: revoked cred ignored, active used)", got)
	}
}

func TestSystemTask_ArgValidation(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("upstream must not be called when args are invalid")
	})

	cases := []struct {
		name string
		args map[string]any
	}{
		{"missing-required", map[string]any{
			"agent_name": "n", "category": "c",
			// instructions missing
		}},
		{"wrong-type", map[string]any{
			"agent_name": 42, "category": "c", "instructions": "i",
		}},
		{"unknown-arg", map[string]any{
			"agent_name": "n", "category": "c", "instructions": "i", "extra": "z",
		}},
		{"too-long", map[string]any{
			"agent_name":   "n",
			"category":     "c",
			"instructions": strings.Repeat("x", 4001),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := h.post(t, "prompt_writer", map[string]any{"args": c.args})
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
			}
			var er struct {
				ErrorCode string `json:"error_code"`
			}
			_ = json.Unmarshal(rr.Body.Bytes(), &er)
			if er.ErrorCode != "invalid_args" {
				t.Fatalf("error_code = %q", er.ErrorCode)
			}
		})
	}
}

func TestSystemTask_UnknownTask_Returns404(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {})
	rr := h.post(t, "no-such-task", map[string]any{"args": map[string]any{}})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
	var er struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &er)
	if er.ErrorCode != "task_not_found" {
		t.Fatalf("error_code = %q", er.ErrorCode)
	}
}

func TestSystemTask_AuthRequired(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {})
	rr := h.post(t, "prompt_writer", map[string]any{"args": validArgs()}, withoutAuth)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
}
