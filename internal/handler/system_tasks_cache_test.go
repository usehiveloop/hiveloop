package handler_test

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/system"
)

// ---------------------------------------------------------------------------
// Spend / generation row
// ---------------------------------------------------------------------------

func TestSystemTask_GenerationRowWritten(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse(promptWriterPayload, 50, 25)))
	})

	rr := h.post(t, "prompt_writer", map[string]any{"args": validArgs()})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}

	var rows []model.Generation
	if err := h.db.Where("org_id = ? AND token_jti = ?", h.org.ID, "system:prompt_writer").Find(&rows).Error; err != nil {
		t.Fatalf("find generations: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d generations rows, want 1", len(rows))
	}
	gen := rows[0]
	if gen.InputTokens != 50 || gen.OutputTokens != 25 {
		t.Fatalf("token usage on generation row = %d/%d, want 50/25", gen.InputTokens, gen.OutputTokens)
	}
	if gen.RequestPath != "/v1/system/tasks/prompt_writer" {
		t.Fatalf("request_path = %q", gen.RequestPath)
	}
}

// ---------------------------------------------------------------------------
// Cache contract
// ---------------------------------------------------------------------------

func TestSystemTask_CacheHit_NoUpstreamCall(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse(promptWriterPayload, 10, 5)))
	})

	body := map[string]any{"args": validArgs()}

	rr1 := h.post(t, "prompt_writer", body)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first: %d", rr1.Code)
	}
	rr2 := h.post(t, "prompt_writer", body)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second: %d", rr2.Code)
	}
	if got := atomic.LoadInt32(h.hits); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (second call should be cached)", got)
	}
	var resp struct {
		Cached bool `json:"cached"`
		Text   string
	}
	_ = json.Unmarshal(rr2.Body.Bytes(), &resp)
	if !resp.Cached {
		t.Fatalf("second response did not signal cached:true")
	}

	// Generation row count: 1 — the cache hit must not record a second
	// generation (no real spend happened).
	var count int64
	h.db.Model(&model.Generation{}).
		Where("org_id = ? AND token_jti = ?", h.org.ID, "system:prompt_writer").
		Count(&count)
	if count != 1 {
		t.Fatalf("generations count = %d, want 1 (cache hit must not add row)", count)
	}
}

func TestSystemTask_CacheBypass_OnVersionBump(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse(promptWriterPayload, 1, 1)))
	})

	body := map[string]any{"args": validArgs()}
	_ = h.post(t, "prompt_writer", body) // populate cache @ v1
	_ = h.post(t, "prompt_writer", body) // hit cache @ v1
	if got := atomic.LoadInt32(h.hits); got != 1 {
		t.Fatalf("after two calls @ v1, hits=%d, want 1", got)
	}

	// Bump the version — same task name, new Version. Cache key changes.
	system.ResetForTest()
	bumped := freshTask("prompt_writer")
	bumped.Version = "v2"
	system.Register(bumped)

	_ = h.post(t, "prompt_writer", body)
	if got := atomic.LoadInt32(h.hits); got != 2 {
		t.Fatalf("after version bump, hits=%d, want 2", got)
	}
}

func TestSystemTask_CacheHit_StreamingShape(t *testing.T) {
	h := newSystemTaskHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse(promptWriterPayload, 1, 1)))
	})

	// Populate cache via a non-streaming call.
	_ = h.post(t, "prompt_writer", map[string]any{"args": validArgs()})

	stream := true
	rr := h.post(t, "prompt_writer", map[string]any{
		"args":   validArgs(),
		"stream": stream,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := atomic.LoadInt32(h.hits); got != 1 {
		t.Fatalf("streaming cache hit must not call upstream; hits=%d", got)
	}
	deltas, done := parseHiveloopSSE(t, rr.Body.String())
	if len(deltas) != 1 {
		t.Fatalf("cached SSE: delta count = %d, want 1", len(deltas))
	}
	if !done.Done || !done.Cached {
		t.Fatalf("done frame: %+v (Done+Cached must both be true)", done)
	}
}

func TestSystemTask_CacheTTLZero_NoCaching(t *testing.T) {
	system.ResetForTest()
	task := freshTask("no_cache_task")
	task.CacheTTL = 0
	system.Register(task)

	h := newSystemTaskHarnessWithoutTask(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse(`{"x":"y"}`, 1, 1)))
	})

	body := map[string]any{"args": validArgs()}
	_ = h.post(t, "no_cache_task", body)
	_ = h.post(t, "no_cache_task", body)
	if got := atomic.LoadInt32(h.hits); got != 2 {
		t.Fatalf("CacheTTL=0 must not cache; hits=%d, want 2", got)
	}
}
