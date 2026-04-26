package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	gormpkg "gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

type recordingEnqueuer struct {
	tasks       []*asynq.Task
	seenUnique  map[string]bool
	failNonUniq bool
}

func newRecordingEnqueuer() *recordingEnqueuer {
	return &recordingEnqueuer{seenUnique: map[string]bool{}}
}

func (r *recordingEnqueuer) Enqueue(t *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	key := t.Type() + "|" + string(t.Payload())
	if r.seenUnique[key] {
		return nil, asynq.ErrDuplicateTask
	}
	r.seenUnique[key] = true
	r.tasks = append(r.tasks, t)
	return &asynq.TaskInfo{}, nil
}

func (r *recordingEnqueuer) Close() error { return nil }

type ragHarness struct {
	DB           *gormpkg.DB
	Org          *model.Org
	OtherOrg     *model.Org
	User         *model.User
	OtherUser    *model.User
	Integ        *model.InIntegration
	Conn         *model.InConnection
	OtherConn    *model.InConnection
	Enq          *recordingEnqueuer
	Router       *chi.Mux
	CapsAllowAll bool
	Caps         func(kind string) bool
}

func newRAGHarness(t *testing.T) *ragHarness {
	t.Helper()
	d := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, d)
	other := testhelpers.NewTestOrg(t, d)
	user := testhelpers.NewTestUser(t, d, org.ID)
	otherUser := testhelpers.NewTestUser(t, d, other.ID)
	integ := testhelpers.NewTestInIntegration(t, d, "rag-handler")
	if err := d.Model(&model.InIntegration{}).Where("id = ?", integ.ID).
		Update("supports_rag_source", true).Error; err != nil {
		t.Fatalf("flag supports_rag_source: %v", err)
	}
	conn := testhelpers.NewTestInConnection(t, d, org.ID, user.ID, integ.ID)
	otherConn := testhelpers.NewTestInConnection(t, d, other.ID, otherUser.ID, integ.ID)

	enq := newRecordingEnqueuer()
	h := &ragHarness{
		DB:        d,
		Org:       org,
		OtherOrg:  other,
		User:      user,
		OtherUser: otherUser,
		Integ:     integ,
		Conn:      conn,
		OtherConn: otherConn,
		Enq:       enq,
	}
	h.Caps = func(kind string) bool { return h.CapsAllowAll }

	rh := handler.NewRAGSourceHandler(d, enq, func(kind string) bool { return h.Caps(kind) })
	r := chi.NewRouter()
	r.Route("/v1/rag", func(r chi.Router) {
		r.Get("/integrations", rh.ListIntegrations)
		r.Post("/sources", rh.Create)
		r.Get("/sources", rh.List)
		r.Get("/sources/{id}", rh.Get)
		r.Patch("/sources/{id}", rh.Update)
		r.Delete("/sources/{id}", rh.Delete)
		r.Post("/sources/{id}/sync", rh.TriggerSync)
		r.Post("/sources/{id}/prune", rh.TriggerPrune)
		r.Post("/sources/{id}/perm-sync", rh.TriggerPermSync)
		r.Get("/sources/{id}/attempts", rh.ListAttempts)
		r.Get("/sources/{id}/attempts/{attempt_id}", rh.GetAttempt)
	})
	h.Router = r

	return h
}

func (h *ragHarness) do(t *testing.T, method, path string, body any, org *model.Org, user *model.User) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	if user != nil {
		req = middleware.WithUser(req, user)
		claims := &auth.AuthClaims{UserID: user.ID.String(), OrgID: orgIDStr(org)}
		req = middleware.WithAuthClaims(req, claims)
	}
	rr := httptest.NewRecorder()
	h.Router.ServeHTTP(rr, req)
	return rr
}

func orgIDStr(o *model.Org) string {
	if o == nil {
		return ""
	}
	return o.ID.String()
}

func (h *ragHarness) createSource(t *testing.T, opts ...func(*ragmodel.RAGSource)) *ragmodel.RAGSource {
	t.Helper()
	conn := testhelpers.NewTestInConnection(t, h.DB, h.Org.ID, h.User.ID, h.Integ.ID)
	connID := conn.ID
	src := &ragmodel.RAGSource{
		OrgIDValue:     h.Org.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "src-" + uuid.New().String()[:8],
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		AccessType:     ragmodel.AccessTypePrivate,
		InConnectionID: &connID,
	}
	for _, o := range opts {
		o(src)
	}
	if err := h.DB.Create(src).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}
	t.Cleanup(func() { h.DB.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })
	return src
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(into); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
}

func mustStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("expected status %d, got %d; body: %s", want, rr.Code, rr.Body.String())
	}
}

// adminGuardedRouter mounts the same handler chain but layers RequireOrgAdmin
// on top, so we can verify the middleware-level reject path.
func adminGuardedRouter(t *testing.T, h *ragHarness) *chi.Mux {
	t.Helper()
	rh := handler.NewRAGSourceHandler(h.DB, h.Enq, func(kind string) bool { return h.Caps(kind) })
	r := chi.NewRouter()
	r.Route("/v1/rag", func(r chi.Router) {
		r.Use(middleware.RequireOrgAdmin(h.DB))
		r.Post("/sources", rh.Create)
	})
	return r
}

// makeAdminUser swaps the harness user's membership to admin role.
func makeAdminUser(t *testing.T, h *ragHarness, user *model.User, org *model.Org, role string) {
	t.Helper()
	if err := h.DB.Model(&model.OrgMembership{}).
		Where("user_id = ? AND org_id = ?", user.ID, org.ID).
		Update("role", role).Error; err != nil {
		t.Fatalf("update role: %v", err)
	}
}

func bodyContains(rr *httptest.ResponseRecorder, sub string) bool {
	return bytes.Contains(rr.Body.Bytes(), []byte(sub))
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func withClaimsFor(req *http.Request, user *model.User, org *model.Org) *http.Request {
	claims := &auth.AuthClaims{UserID: user.ID.String(), OrgID: orgIDStr(org)}
	return middleware.WithAuthClaims(req, claims)
}

// HTTP helpers used by the suite.
func get(t *testing.T, h *ragHarness, path string) *httptest.ResponseRecorder {
	return h.do(t, http.MethodGet, path, nil, h.Org, h.User)
}
func post(t *testing.T, h *ragHarness, path string, body any) *httptest.ResponseRecorder {
	return h.do(t, http.MethodPost, path, body, h.Org, h.User)
}
func patch(t *testing.T, h *ragHarness, path string, body any) *httptest.ResponseRecorder {
	return h.do(t, http.MethodPatch, path, body, h.Org, h.User)
}
func del(t *testing.T, h *ragHarness, path string) *httptest.ResponseRecorder {
	return h.do(t, http.MethodDelete, path, nil, h.Org, h.User)
}
