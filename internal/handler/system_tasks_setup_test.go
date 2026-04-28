package handler_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

	system.ResetForTest()
	system.Register(promptWriterTestTask())

	return buildHarness(t, upstream)
}

// newSystemTaskHarnessWithoutTask skips registering prompt_writer; the test
// is responsible for calling system.Register before invoking this.
func newSystemTaskHarnessWithoutTask(t *testing.T, upstream fakeUpstream) *systemTaskHarness {
	t.Helper()
	return buildHarness(t, upstream)
}

func buildHarness(t *testing.T, upstream fakeUpstream) *systemTaskHarness {
	t.Helper()
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
	h := handler.NewSystemTaskHandler(
		db, credentials.NewPicker(db), kms,
		registry.Global(), cache, fwd, billing.NewCreditsService(db),
	)

	r := chi.NewRouter()
	r.Use(injectAuthClaimsMiddleware())
	r.Post("/v1/system/tasks/{taskName}", h.Run)

	out := &systemTaskHarness{
		db: db, router: r, upstream: srv, hits: &hits, cache: cache,
		org: org, user: user,
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
// request context if a Bearer token is present. Short-circuits 401 when no
// Authorization header is set, simulating RequireAuth.
func injectAuthClaimsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if authz == "" {
				http.Error(w, `{"error":"missing auth"}`, http.StatusUnauthorized)
				return
			}
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
			r = middleware.WithAuthClaims(r, &auth.AuthClaims{OrgID: orgID, UserID: userID})
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

func withoutAuth(req *http.Request) { req.Header.Del("Authorization") }

func sysShortID() string           { return uuid.New().String()[:8] }
func tptr(t time.Time) *time.Time { return &t }

// validArgs returns the canonical request body for prompt_writer.
func validArgs() map[string]any {
	return map[string]any{
		"agent_name":   "deploy-watcher",
		"category":     "ops",
		"instructions": "Watch Railway deployments and open a GitHub issue when one fails.",
		"skills_md":    "- fetch-railway-logs: pulls deployment logs\n- create-github-issue: opens an issue",
		"tools_md":     "- slack_post: posts a status line back to the trigger source",
		"triggers_md":  "- railway.deployment.failed: payload includes deployment_id",
	}
}

// promptWriterPayload is the markdown the LLM is expected to stream back.
// Prompt_writer streams the produced system prompt directly — no JSON
// wrapper. The harness uses this as the canned upstream response.
const promptWriterPayload = "## Role\nYou are deploy-watcher. You triage failed Railway deployments.\n\n## Workflow\n1. Receive the deployment_id.\n2. Load the `fetch-railway-logs` skill.\n3. Open an issue with `create-github-issue`.\n4. Post a summary via `slack_post`.\n\n## Stop conditions\n- Issue created.\n- Two consecutive empty fetches.\n"

// promptWriterTestTask is the prompt_writer task definition used by tests.
// Mirrors internal/system/tasks/prompt_writer.go but kept local so the
// harness can register it independently of import side-effects, and pinned
// to ProviderGroup "openai" so the harness's seeded openai system credential
// resolves it.
func promptWriterTestTask() system.Task {
	return system.Task{
		Name:               "prompt_writer",
		Version:            "v2",
		ProviderGroup:      "openai",
		ModelTier:          system.ModelCheapest,
		SystemPrompt:       "You write production-grade system prompts.",
		UserPromptTemplate: "agent: {{.agent_name}}\ncategory: {{.category}}\ninstructions: {{.instructions}}\n{{if .skills_md}}skills:\n{{.skills_md}}\n{{end}}{{if .tools_md}}tools:\n{{.tools_md}}\n{{end}}{{if .triggers_md}}triggers:\n{{.triggers_md}}\n{{end}}",
		Args: []system.ArgSpec{
			{Name: "agent_name", Type: system.ArgString, Required: true, MaxLen: 80},
			{Name: "category", Type: system.ArgString, Required: true, MaxLen: 80},
			{Name: "instructions", Type: system.ArgString, Required: true, MaxLen: 4000},
			{Name: "skills_md", Type: system.ArgString, Required: false, MaxLen: 4000},
			{Name: "sub_agents_md", Type: system.ArgString, Required: false, MaxLen: 4000},
			{Name: "triggers_md", Type: system.ArgString, Required: false, MaxLen: 4000},
			{Name: "tools_md", Type: system.ArgString, Required: false, MaxLen: 2000},
		},
		MaxOutputTokens: 1024,
		CacheTTL:        24 * time.Hour,
	}
}

// freshTask returns a fully-valid system.Task with the given name. Used by
// version-bump and cache-disabled tests so they don't depend on the real
// prompt_writer's prompt text.
func freshTask(name string) system.Task {
	t := promptWriterTestTask()
	t.Name = name
	t.SystemPrompt = "test"
	t.MaxOutputTokens = 256
	return t
}

