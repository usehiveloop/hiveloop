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

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/system"
	"github.com/usehiveloop/hiveloop/internal/system/tasks"
)

// ---------------------------------------------------------------------------
// Integration tests for the REAL prompt_writer task — the structured-args +
// org-scoped resolver path. Distinct from the generic system_tasks_test.go
// which uses a stub task fixture.
//
// What's being verified:
//
//   1. Resolution actually replaces IDs with rich data: the rendered user
//      prompt that goes upstream contains the skill name and description,
//      not the raw UUID.
//   2. Org isolation: a skill belonging to a foreign org cannot be referenced
//      by ID — the resolver returns the typed error_code "unknown_skill".
//   3. Public skills cross org boundaries when status='published'.
//
// Each test seeds its own rows + cleans up. Postgres test DB must be running
// (`make test-setup`).
// ---------------------------------------------------------------------------

// promptWriterHarness is a parallel to systemTaskHarness, but registers the
// REAL PromptWriter task and injects the real actions catalog so resolver
// paths that touch it (action descriptions, trigger key descriptions) are
// exercised end-to-end.
type promptWriterHarness struct {
	db           *gorm.DB
	router       *chi.Mux
	upstream     *httptest.Server
	hits         *int32
	upstreamBody *string // last raw upstream chat-completions request body
	org          *model.Org
	otherOrg     *model.Org
	user         *model.User
	cleanupFn    []func()
}

func (h *promptWriterHarness) cleanup(t *testing.T) {
	t.Helper()
	for _, f := range h.cleanupFn {
		f()
	}
}

func (h *promptWriterHarness) authToken() string {
	return fmt.Sprintf("Bearer org=%s;user=%s", h.org.ID, h.user.ID)
}

func (h *promptWriterHarness) post(t *testing.T, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/tasks/prompt_writer", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", h.authToken())
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func newPromptWriterHarness(t *testing.T, upstreamFn fakeUpstream) *promptWriterHarness {
	t.Helper()

	system.ResetForTest()
	system.Register(tasks.PromptWriter)

	db := connectTestDB(t)

	var hits int32
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		// Capture the upstream request body so tests can assert on the
		// rendered user prompt that resolution produced.
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		// Re-feed the body for downstream handlers if any.
		r.Body = io.NopCloser(strings.NewReader(capturedBody))
		upstreamFn(w, r)
	}))

	kms := newPromptWriterKMS(t)
	// PromptWriter uses ProviderGroup "gemini" — the picker maps
	// ProviderID="google" → group "gemini" via subagents.MapProviderToGroup.
	cred := seedSystemCredential(t, db, kms, srv.URL+"/v1", "google")

	org := &model.Org{Name: "pw-org-" + sysShortID()}
	if err := db.Create(org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	otherOrg := &model.Org{Name: "pw-otherorg-" + sysShortID()}
	if err := db.Create(otherOrg).Error; err != nil {
		t.Fatalf("create otherOrg: %v", err)
	}
	user := &model.User{
		Email:            fmt.Sprintf("pw-%s@test.local", sysShortID()),
		Name:             "pw-tester",
		EmailConfirmedAt: tptr(time.Now()),
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	cache := system.NewMemCache()
	fwd := system.NewForwarder(&http.Client{Timeout: 5 * time.Second})
	h := handler.NewSystemTaskHandler(
		db, credentials.NewPicker(db), kms,
		registry.Global(), cache, fwd, billing.NewCreditsService(db),
		catalog.Global(),
	)

	r := chi.NewRouter()
	r.Use(injectAuthClaimsMiddleware())
	r.Post("/v1/system/tasks/{taskName}", h.Run)

	out := &promptWriterHarness{
		db: db, router: r, upstream: srv, hits: &hits, upstreamBody: &capturedBody,
		org: org, otherOrg: otherOrg, user: user,
		cleanupFn: []func(){
			srv.Close,
			func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) },
			func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) },
			func() { db.Where("id = ?", otherOrg.ID).Delete(&model.Org{}) },
			func() { db.Where("id = ?", user.ID).Delete(&model.User{}) },
			func() { db.Where("token_jti LIKE ?", "system:%").Delete(&model.Generation{}) },
		},
	}
	t.Cleanup(func() { out.cleanup(t) })
	return out
}

func newPromptWriterKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	b64 := base64.StdEncoding.EncodeToString(key)
	kms, err := crypto.NewAEADWrapper(context.Background(), b64, "prompt-writer-test")
	if err != nil {
		t.Fatalf("KMS: %v", err)
	}
	return kms
}

func seedSkill(t *testing.T, db *gorm.DB, orgID *uuid.UUID, name, description, status string) *model.Skill {
	t.Helper()
	desc := description
	skill := &model.Skill{
		OrgID:       orgID,
		Slug:        "skill-" + sysShortID(),
		Name:        name,
		Description: &desc,
		SourceType:  model.SkillSourceInline,
		Status:      status,
	}
	if err := db.Create(skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("id = ?", skill.ID).Delete(&model.Skill{})
	})
	return skill
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// The point of resolution: a UUID in skill_ids becomes the skill's
// human-meaningful name + description in the rendered user prompt that goes
// to Gemini. If this regresses, every prompt the model writes will refer to
// agents' skills by opaque IDs.
func TestPromptWriter_RendersResolvedSkillsIntoUpstreamRequest(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse("## Role\nYou are deploy-watcher.\n", 10, 5)))
	})

	skill := seedSkill(t, h.db, &h.org.ID,
		"fetch-railway-logs",
		"pulls the last N lines of deployment logs from Railway",
		model.SkillStatusDraft,
	)

	stream := false
	rr := h.post(t, map[string]any{
		"stream": stream,
		"args": map[string]any{
			"name":         "deploy-watcher",
			"category":     "ops",
			"instructions": "Watch for failed Railway deployments and triage them.",
			"skill_ids":    []string{skill.ID.String()},
		},
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	upstream := *h.upstreamBody
	if upstream == "" {
		t.Fatal("upstream never received the forwarded request")
	}
	// The user message in the upstream chat-completions body must contain
	// both the skill's name AND its description — i.e., resolution actually
	// happened, not just slot-filling the UUID.
	if !strings.Contains(upstream, "fetch-railway-logs") {
		t.Errorf("upstream request missing skill name; body:\n%s", upstream)
	}
	if !strings.Contains(upstream, "pulls the last N lines of deployment logs from Railway") {
		t.Errorf("upstream request missing skill description; body:\n%s", upstream)
	}
	// Belt-and-braces: the raw UUID should NOT leak into the prompt.
	if strings.Contains(upstream, skill.ID.String()) {
		t.Errorf("raw skill UUID leaked into prompt:\n%s", upstream)
	}
}

// Org isolation: skill_ids referencing a foreign-org skill must be rejected
// before any LLM call. The error envelope must carry a stable error_code so
// the FE can switch on it.
func TestPromptWriter_ForeignOrgSkill_Returns400UnknownSkill(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be hit when resolution fails")
	})

	foreign := seedSkill(t, h.db, &h.otherOrg.ID,
		"private-skill",
		"belongs to a different org",
		model.SkillStatusPublished, // even if published-in-its-org, not visible
	)

	rr := h.post(t, map[string]any{
		"stream": false,
		"args": map[string]any{
			"name":         "deploy-watcher",
			"instructions": "irrelevant",
			"skill_ids":    []string{foreign.ID.String()},
		},
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, rr.Body.String())
	}
	if envelope.ErrorCode != "unknown_skill" {
		t.Errorf("error_code=%q want %q (body=%s)", envelope.ErrorCode, "unknown_skill", rr.Body.String())
	}
	if got := atomic.LoadInt32(h.hits); got != 0 {
		t.Errorf("upstream was hit %d times despite resolution failure", got)
	}
}

// Public-skill visibility rule: org_id IS NULL AND status='published' is
// reachable from any org. Pins the SQL filter — flipping it to org-only
// would silently break the marketplace.
func TestPromptWriter_PublicPublishedSkill_VisibleAcrossOrgs(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse("## Role\nYou are an agent.\n", 5, 3)))
	})

	publicSkill := seedSkill(t, h.db, nil,
		"public-marketplace-skill",
		"installed by anyone",
		model.SkillStatusPublished,
	)

	rr := h.post(t, map[string]any{
		"stream": false,
		"args": map[string]any{
			"name":         "any-agent",
			"instructions": "use the public skill",
			"skill_ids":    []string{publicSkill.ID.String()},
		},
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(*h.upstreamBody, "public-marketplace-skill") {
		t.Errorf("upstream request missing public skill name; body:\n%s", *h.upstreamBody)
	}
}

// Public DRAFT skills must NOT leak — the marketplace gates visibility on
// status='published'. Pairs with the test above so a single rule change
// can't pass both.
func TestPromptWriter_PublicDraftSkill_NotVisible(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be hit when resolution fails")
	})

	draft := seedSkill(t, h.db, nil,
		"public-draft",
		"not yet published",
		model.SkillStatusDraft,
	)

	rr := h.post(t, map[string]any{
		"stream": false,
		"args": map[string]any{
			"name":         "any-agent",
			"instructions": "irrelevant",
			"skill_ids":    []string{draft.ID.String()},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &envelope)
	if envelope.ErrorCode != "unknown_skill" {
		t.Errorf("error_code=%q want unknown_skill", envelope.ErrorCode)
	}
}
