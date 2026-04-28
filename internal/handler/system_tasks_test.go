package handler_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/system"
)

// systemTaskHarness wires up everything the handler needs against a real
// Postgres + a fake LLM upstream. Each test creates its own harness so the
// system task registry, DB rows, and upstream hit count stay isolated.
type systemTaskHarness struct {
	db        *gorm.DB
	router    *chi.Mux
	upstream  *httptest.Server
	hits      *int32
	cache     *system.MemCache
	org       *model.Org
	user      *model.User
	signKey   []byte
	cleanupFn []func()
}

func (h *systemTaskHarness) Cleanup(t *testing.T) {
	t.Helper()
	for _, f := range h.cleanupFn {
		f()
	}
}

// fakeUpstream is the test handler the in-process LLM server runs. Each
// test injects its own to control responses.
type fakeUpstream func(w http.ResponseWriter, r *http.Request)

// chatCompletionResponse is the canonical OpenAI shape; tests use this
// helper rather than constructing JSON literals over and over.
func chatCompletionResponse(content string, in, out int) string {
	return fmt.Sprintf(`{
		"model":"fake-model",
		"choices":[{"message":{"content":%q}}],
		"usage":{"prompt_tokens":%d,"completion_tokens":%d}
	}`, content, in, out)
}

func newSystemTaskHarness(t *testing.T, upstream fakeUpstream) *systemTaskHarness {
	t.Helper()

	db := connectTestDB(t)
	system.ResetForTest()

	// Register the real prompt_writer task — exercises the actual prompt
	// + arg schema we ship.
	system.Register(system.Task{
		Name:               "prompt_writer",
		Version:            "v1",
		ProviderGroup:      "openai",
		ModelTier:          system.ModelCheapest,
		ResponseFormat:     system.ResponseJSON,
		SystemPrompt:       "You write production-grade system prompts.",
		UserPromptTemplate: "target_model: {{.target_model}}\ngoal: {{.goal}}\naudience: {{.audience}}",
		Args: []system.ArgSpec{
			{Name: "target_model", Type: system.ArgString, Required: true, MaxLen: 80},
			{Name: "goal", Type: system.ArgString, Required: true, MaxLen: 1000},
			{Name: "audience", Type: system.ArgString, Required: true, MaxLen: 200},
			{Name: "constraints", Type: system.ArgStringList, Required: false, MaxLen: 200},
		},
		MaxOutputTokens: 1024,
		CacheTTL:        24 * time.Hour,
	})

	// Fake upstream LLM. We track hit count for cache assertions.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		upstream(w, r)
	}))

	// KMS + system credential. Same provider_id as the task's group so
	// MapProviderToGroup("openai", "") == "openai".
	kms := newSystemTaskKMS(t)
	cred := seedSystemCredential(t, db, kms, srv.URL+"/v1", "openai")

	// Seed a real org + user for the handler's claim resolution + the
	// generations row's foreign keys.
	org := &model.Org{Name: "system-task-org-" + sysShortID()}
	if err := db.Create(org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := &model.User{
		Email:            fmt.Sprintf("u-%s@test.local", sysShortID()),
		Name:             "tester",
		EmailConfirmedAt: tptr(time.Now()),
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	cache := system.NewMemCache()
	httpClient := &http.Client{Timeout: 5 * time.Second}
	fwd := system.NewForwarder(httpClient)

	h := handler.NewSystemTaskHandler(
		db,
		credentials.NewPicker(db),
		kms,
		registry.Global(),
		cache,
		fwd,
		billing.NewCreditsService(db),
	)

	signKey := []byte("system-tasks-test-signing-key")
	r := chi.NewRouter()
	// Inject claims in front of the handler. Tests can override the
	// injected claims via the X-Test-Skip-Auth header for the no-auth case.
	r.Use(injectAuthClaimsMiddleware(signKey))
	r.Post("/v1/system/tasks/{taskName}", h.Run)

	out := &systemTaskHarness{
		db:       db,
		router:   r,
		upstream: srv,
		hits:     &hits,
		cache:    cache,
		org:      org,
		user:     user,
		signKey:  signKey,
		cleanupFn: []func(){
			srv.Close,
			func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) },
			func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) },
			func() { db.Where("id = ?", user.ID).Delete(&model.User{}) },
			func() {
				db.Where("token_jti LIKE ?", "system:%").Delete(&model.Generation{})
			},
		},
	}
	t.Cleanup(func() { out.Cleanup(t) })
	return out
}

func newSystemTaskKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	b64 := base64.StdEncoding.EncodeToString(key)
	kms, err := crypto.NewAEADWrapper(context.Background(), b64, "system-tasks-test")
	if err != nil {
		t.Fatalf("KMS: %v", err)
	}
	return kms
}

func seedSystemCredential(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, baseURL, providerID string) *model.Credential {
	t.Helper()
	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("dek: %v", err)
	}
	encKey, err := crypto.EncryptCredential([]byte("sk-fake"), dek)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	wrapped, err := kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	for i := range dek {
		dek[i] = 0
	}
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed platform org: %v", err)
	}
	cred := &model.Credential{
		OrgID:        credentials.PlatformOrgID,
		Label:        "test-system-" + sysShortID(),
		BaseURL:      baseURL,
		AuthScheme:   "bearer",
		ProviderID:   providerID,
		EncryptedKey: encKey,
		WrappedDEK:   wrapped,
		IsSystem:     true,
	}
	if err := db.Create(cred).Error; err != nil {
		t.Fatalf("create system credential: %v", err)
	}
	return cred
}

// injectAuthClaimsMiddleware drops the harness's user+org claims onto the
// request context if a Bearer token is present. It also short-circuits with
// 401 when no Authorization header is set, simulating RequireAuth.
//
// We keep this in test code rather than wiring real RequireAuth because
// minting a real JWT just to test a handler that already trusts claims
// would add a bunch of test-only key plumbing for no business value.
func injectAuthClaimsMiddleware(signKey []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if authz == "" {
				http.Error(w, `{"error":"missing auth"}`, http.StatusUnauthorized)
				return
			}
			// "Bearer org=<uuid>;user=<uuid>" — wire format used only in
			// tests.
			parts := strings.SplitN(strings.TrimPrefix(authz, "Bearer "), ";", 2)
			orgID, userID := "", ""
			for _, p := range parts {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) != 2 {
					continue
				}
				switch kv[0] {
				case "org":
					orgID = kv[1]
				case "user":
					userID = kv[1]
				}
			}
			r = middleware.WithAuthClaims(r, &auth.AuthClaims{
				OrgID:  orgID,
				UserID: userID,
			})
			next.ServeHTTP(w, r)
		})
	}
}

func (h *systemTaskHarness) authToken() string {
	return fmt.Sprintf("Bearer org=%s;user=%s", h.org.ID, h.user.ID)
}

func (h *systemTaskHarness) post(t *testing.T, taskName string, body any, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/tasks/"+taskName, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", h.authToken())
	for _, opt := range opts {
		opt(req)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// withoutAuth strips the Authorization header so the test can exercise the
// 401 branch.
func withoutAuth(req *http.Request) { req.Header.Del("Authorization") }

func sysShortID() string { return uuid.New().String()[:8] }
func tptr(t time.Time) *time.Time { return &t }

// validArgs returns the canonical request body for prompt_writer.
func validArgs() map[string]any {
	return map[string]any{
		"target_model": "gpt-4.1-mini",
		"goal":         "summarise GitHub PR diffs into one-paragraph release notes",
		"audience":     "engineering managers",
	}
}

// promptWriterPayload is the JSON the LLM is expected to return for
// prompt_writer (the task asks for json_object response_format).
const promptWriterPayload = `{"title":"PR diff summariser","system_prompt":"You take git diffs and produce one paragraph of release notes for engineering managers. Output plain text only.","rationale":"Open with role+goal; declare format; constrain audience."}`

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
	// The text we get back is the JSON the LLM produced. The prompt_writer
	// contract says it must be valid JSON with title/system_prompt/rationale.
	var inner struct {
		Title        string `json:"title"`
		SystemPrompt string `json:"system_prompt"`
		Rationale    string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &inner); err != nil {
		t.Fatalf("LLM payload not valid JSON: %v\n%s", err, resp.Text)
	}
	if inner.SystemPrompt == "" {
		t.Fatalf("LLM payload missing system_prompt")
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
		_, _ = io.WriteString(w, `data: {"model":"m","choices":[{"delta":{"content":"`+escapeJSON(`{"title":"x",`)+`"}}]}

data: {"model":"m","choices":[{"delta":{"content":"`+escapeJSON(`"system_prompt":"x","rationale":"x"}`)+`"}}]}

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
	var inner struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(full), &inner); err != nil {
		t.Fatalf("streamed text is not valid JSON: %v\n%s", err, full)
	}
	if inner.Title == "" {
		t.Fatalf("streamed JSON missing title field")
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
	// Wipe system credentials so the picker has nothing to return.
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
	// Harness already seeded one active credential. Revoke it, then add a
	// second active one — the handler must pick the active one.
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
			"target_model": "m", "goal": "x",
		}},
		{"wrong-type", map[string]any{
			"target_model": 42, "goal": "x", "audience": "y",
		}},
		{"unknown-arg", map[string]any{
			"target_model": "m", "goal": "x", "audience": "y", "extra": "z",
		}},
		{"too-long", map[string]any{
			"target_model": "m",
			"goal":         strings.Repeat("x", 1001),
			"audience":     "y",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := h.post(t, "prompt_writer", map[string]any{"args": c.args})
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
			}
			var er struct{ ErrorCode string `json:"error_code"` }
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
	var er struct{ ErrorCode string `json:"error_code"` }
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

	// First call → upstream.
	rr1 := h.post(t, "prompt_writer", body)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first: %d", rr1.Code)
	}
	// Second call → cache.
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
	_ = h.post(t, "prompt_writer", body)            // populate cache @ v1
	_ = h.post(t, "prompt_writer", body)            // hit cache @ v1
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

// ---------------------------------------------------------------------------
// Helpers used by multiple cases
// ---------------------------------------------------------------------------

// freshTask returns a fully-valid system.Task with the given name. Used by
// version-bump and cache-disabled tests so they don't depend on the real
// prompt_writer's prompt text.
func freshTask(name string) system.Task {
	return system.Task{
		Name:               name,
		Version:            "v1",
		ProviderGroup:      "openai",
		ModelTier:          system.ModelCheapest,
		ResponseFormat:     system.ResponseJSON,
		SystemPrompt:       "test",
		UserPromptTemplate: "target_model: {{.target_model}}\ngoal: {{.goal}}\naudience: {{.audience}}",
		Args: []system.ArgSpec{
			{Name: "target_model", Type: system.ArgString, Required: true, MaxLen: 80},
			{Name: "goal", Type: system.ArgString, Required: true, MaxLen: 1000},
			{Name: "audience", Type: system.ArgString, Required: true, MaxLen: 200},
			{Name: "constraints", Type: system.ArgStringList, Required: false, MaxLen: 200},
		},
		MaxOutputTokens: 256,
		CacheTTL:        24 * time.Hour,
	}
}

// newSystemTaskHarnessWithoutTask is for tests that registered the task
// themselves before constructing the harness — it skips re-registering
// prompt_writer.
func newSystemTaskHarnessWithoutTask(t *testing.T, upstream fakeUpstream) *systemTaskHarness {
	t.Helper()
	// Same as newSystemTaskHarness but skips the ResetForTest+Register block.
	db := connectTestDB(t)

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		upstream(w, r)
	}))

	kms := newSystemTaskKMS(t)
	cred := seedSystemCredential(t, db, kms, srv.URL+"/v1", "openai")
	org := &model.Org{Name: "system-task-org-" + sysShortID()}
	if err := db.Create(org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := &model.User{
		Email:            fmt.Sprintf("u-%s@test.local", sysShortID()),
		Name:             "tester",
		EmailConfirmedAt: tptr(time.Now()),
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	cache := system.NewMemCache()
	fwd := system.NewForwarder(&http.Client{Timeout: 5 * time.Second})
	h := handler.NewSystemTaskHandler(db, credentials.NewPicker(db), kms, registry.Global(), cache, fwd, billing.NewCreditsService(db))
	signKey := []byte("k")
	r := chi.NewRouter()
	r.Use(injectAuthClaimsMiddleware(signKey))
	r.Post("/v1/system/tasks/{taskName}", h.Run)

	out := &systemTaskHarness{
		db: db, router: r, upstream: srv, hits: &hits, cache: cache,
		org: org, user: user, signKey: signKey,
		cleanupFn: []func(){
			srv.Close,
			func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) },
			func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) },
			func() { db.Where("id = ?", user.ID).Delete(&model.User{}) },
			func() { db.Where("token_jti LIKE ?", "system:%").Delete(&model.Generation{}) },
		},
	}
	t.Cleanup(func() { out.Cleanup(t) })
	return out
}

type parsedDoneFrame struct {
	Done   bool         `json:"done"`
	Usage  system.Usage `json:"usage"`
	Cached bool         `json:"cached"`
}

func parseHiveloopSSE(t *testing.T, body string) ([]string, parsedDoneFrame) {
	t.Helper()
	var deltas []string
	var done parsedDoneFrame
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var d struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &d); err == nil && d.Delta != "" {
			deltas = append(deltas, d.Delta)
			continue
		}
		_ = json.Unmarshal([]byte(payload), &done)
	}
	return deltas, done
}

// escapeJSON escapes inner JSON for embedding in another JSON string field.
func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	// b is now `"..."`, strip the wrapping quotes.
	return string(b[1 : len(b)-1])
}
