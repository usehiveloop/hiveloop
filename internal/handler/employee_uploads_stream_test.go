package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type employeeStreamFixture struct {
	agentID   uuid.UUID
	sandboxID uuid.UUID
	bridgeKey string
}

type employeeStreamHarness struct {
	db     *gorm.DB
	router *chi.Mux
	orgID  uuid.UUID
}

func newEmployeeStreamHarness(t *testing.T) *employeeStreamHarness {
	t.Helper()
	db := connectTestDB(t)
	presigner := newRealPresigner(t)
	encKey := testSymmetricKey(t)

	h := handler.NewUploadsHandler(db, presigner)
	h.WithStreamer(presigner, encKey)

	r := chi.NewRouter()
	r.Put("/internal/employees/{employeeID}/assets/*", h.StreamEmployeeAsset)
	r.Post("/internal/employees/{employeeID}/assets/move", h.MoveEmployeeAsset)
	r.Delete("/internal/employees/{employeeID}/assets/*", h.DeleteEmployeeAsset)

	orgID := uuid.New()
	if err := db.Create(&model.Org{
		ID: orgID, Name: "emp-stream-" + uuid.New().String()[:8], RateLimit: 1000, Active: true,
	}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.CloudAgentTask{})
		db.Where("org_id = ?", orgID).Delete(&model.AgentConversation{})
		db.Where("org_id = ?", orgID).Delete(&model.EmployeeAsset{})
		db.Where("org_id = ?", orgID).Delete(&model.Sandbox{})
		db.Exec("DELETE FROM agent_subagents WHERE agent_id IN (SELECT id FROM agents WHERE org_id = ?) OR subagent_id IN (SELECT id FROM agents WHERE org_id = ?)", orgID, orgID)
		db.Where("org_id = ?", orgID).Delete(&model.Agent{})
		db.Where("id = ?", orgID).Delete(&model.Org{})
	})

	return &employeeStreamHarness{db: db, router: r, orgID: orgID}
}

// seedEmployee creates an employee agent + its sandbox with a fresh bridge
// key. isEmployee=false produces a regular agent (with a sandbox that still
// has a bearer) so we can prove the endpoint rejects non-employees.
func (h *employeeStreamHarness) seedEmployee(t *testing.T, isEmployee bool) employeeStreamFixture {
	t.Helper()
	encKey := testSymmetricKey(t)

	agentID := uuid.New()
	if err := h.db.Create(&model.Agent{
		ID:         agentID,
		OrgID:      &h.orgID,
		Name:       "agent-" + uuid.New().String()[:8],
		Status:     "active",
		IsEmployee: isEmployee,
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	bridgeKey := "bridge-" + uuid.New().String()
	encrypted, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	sandboxID := uuid.New()
	if err := h.db.Create(&model.Sandbox{
		ID:                    sandboxID,
		OrgID:                 &h.orgID,
		AgentID:               &agentID,
		EncryptedBridgeAPIKey: encrypted,
		Status:                "running",
		ExternalID:            "mock-ext-" + uuid.New().String()[:8],
		BridgeURL:             "http://localhost:25434",
	}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	return employeeStreamFixture{agentID: agentID, sandboxID: sandboxID, bridgeKey: bridgeKey}
}

func (h *employeeStreamHarness) seedSubagentTaskSandbox(t *testing.T, employeeID uuid.UUID) string {
	t.Helper()
	encKey := testSymmetricKey(t)

	cloudAgentID := uuid.New()
	if err := h.db.Create(&model.Agent{
		ID:     cloudAgentID,
		OrgID:  &h.orgID,
		Name:   "cloud-agent-" + uuid.New().String()[:8],
		Status: "active",
	}).Error; err != nil {
		t.Fatalf("create cloud agent: %v", err)
	}
	if err := h.db.Create(&model.AgentSubagent{AgentID: employeeID, SubagentID: cloudAgentID}).Error; err != nil {
		t.Fatalf("create subagent link: %v", err)
	}

	bridgeKey := "cloud-bridge-" + uuid.New().String()
	encrypted, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt cloud key: %v", err)
	}
	sandboxID := uuid.New()
	if err := h.db.Create(&model.Sandbox{
		ID:                    sandboxID,
		OrgID:                 &h.orgID,
		AgentID:               &cloudAgentID,
		EncryptedBridgeAPIKey: encrypted,
		Status:                "running",
		ExternalID:            "cloud-ext-" + uuid.New().String()[:8],
		BridgeURL:             "http://cloud.local",
	}).Error; err != nil {
		t.Fatalf("create cloud sandbox: %v", err)
	}
	convID := uuid.New()
	if err := h.db.Create(&model.AgentConversation{
		ID:                   convID,
		OrgID:                h.orgID,
		AgentID:              cloudAgentID,
		SandboxID:            sandboxID,
		RuntimeConversationID: "bridge-" + uuid.New().String(),
		Status:               "active",
	}).Error; err != nil {
		t.Fatalf("create cloud conversation: %v", err)
	}
	if err := h.db.Create(&model.CloudAgentTask{
		ID:                     uuid.New(),
		OrgID:                  h.orgID,
		EmployeeAgentID:        employeeID,
		CloudAgentID:           cloudAgentID,
		SandboxID:              sandboxID,
		ConversationID:         convID,
		ParentConversationType: "agent_conversation",
		ParentConversationID:   "session-123",
		Brief:                  "research",
		Metadata:               model.JSON{},
	}).Error; err != nil {
		t.Fatalf("create cloud task: %v", err)
	}
	return bridgeKey
}

func (h *employeeStreamHarness) put(t *testing.T, path string, body io.Reader, contentType, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestEmployeeStream_HappyPath(t *testing.T) {
	h := newEmployeeStreamHarness(t)
	emp := h.seedEmployee(t, true)
	body := []byte("# notes\n")

	rr := h.put(t,
		fmt.Sprintf("/internal/employees/%s/assets/notes/intro.md", emp.agentID),
		bytes.NewReader(body),
		"text/markdown",
		emp.bridgeKey,
	)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string `json:"id"`
		Key       string `json:"key"`
		Path      string `json:"path"`
		Filename  string `json:"filename"`
		Bytes     int64  `json:"bytes"`
		PublicURL string `json:"asset_url"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	wantKey := fmt.Sprintf("pub/e/%s/notes/intro.md", emp.agentID)
	if resp.Key != wantKey {
		t.Errorf("key = %q want %q", resp.Key, wantKey)
	}
	if resp.Path != "notes" || resp.Filename != "intro.md" {
		t.Errorf("path/filename mismatch: %+v", resp)
	}
	if resp.Bytes != int64(len(body)) {
		t.Errorf("bytes = %d want %d", resp.Bytes, len(body))
	}

	var row model.EmployeeAsset
	if err := h.db.Where("key = ?", wantKey).First(&row).Error; err != nil {
		t.Fatalf("row not persisted: %v", err)
	}
	if row.AgentID != emp.agentID || row.OrgID != h.orgID || row.SandboxID != emp.sandboxID {
		t.Errorf("scoping wrong: %+v", row)
	}
}

func TestEmployeeStream_NonEmployeeAgentReturns404(t *testing.T) {
	h := newEmployeeStreamHarness(t)
	regular := h.seedEmployee(t, false)

	rr := h.put(t,
		fmt.Sprintf("/internal/employees/%s/assets/file.txt", regular.agentID),
		bytes.NewReader([]byte("x")),
		"text/plain",
		regular.bridgeKey,
	)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (regular agents not addressable on /employees): %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.EmployeeAsset{}).Where("agent_id = ?", regular.agentID).Count(&count)
	if count != 0 {
		t.Errorf("no asset row should be created for rejected request, got %d", count)
	}
}

func TestEmployeeStream_CrossEmployeeBearerRejected(t *testing.T) {
	h := newEmployeeStreamHarness(t)
	a := h.seedEmployee(t, true)
	b := h.seedEmployee(t, true)

	rr := h.put(t,
		fmt.Sprintf("/internal/employees/%s/assets/probe.txt", b.agentID),
		bytes.NewReader([]byte("x")),
		"text/plain",
		a.bridgeKey,
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when employee A's bearer hits employee B's URL: %s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeStream_SubagentTaskBearerCanUploadToEmployeeDrive(t *testing.T) {
	h := newEmployeeStreamHarness(t)
	emp := h.seedEmployee(t, true)
	subagentKey := h.seedSubagentTaskSandbox(t, emp.agentID)

	rr := h.put(t,
		fmt.Sprintf("/internal/employees/%s/assets/tasks/%s/report.md", emp.agentID, uuid.New()),
		bytes.NewReader([]byte("# report")),
		"text/markdown",
		subagentKey,
	)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for employee-owned subagent bearer: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if !strings.HasPrefix(resp.Key, "pub/e/"+emp.agentID.String()+"/tasks/") {
		t.Fatalf("uploaded to wrong employee drive key: %q", resp.Key)
	}
}

func TestEmployeeStream_OverwriteByPath(t *testing.T) {
	h := newEmployeeStreamHarness(t)
	emp := h.seedEmployee(t, true)
	urlPath := fmt.Sprintf("/internal/employees/%s/assets/exports/data.csv", emp.agentID)

	first := h.put(t, urlPath, bytes.NewReader([]byte("v1")), "text/csv", emp.bridgeKey)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	var firstResp struct{ ID string }
	_ = json.Unmarshal(first.Body.Bytes(), &firstResp)

	second := h.put(t, urlPath, bytes.NewReader([]byte("v2-much-longer")), "text/csv", emp.bridgeKey)
	if second.Code != http.StatusCreated {
		t.Fatalf("second: %d %s", second.Code, second.Body.String())
	}
	var secondResp struct {
		ID    string `json:"id"`
		Bytes int64  `json:"bytes"`
	}
	_ = json.Unmarshal(second.Body.Bytes(), &secondResp)

	if secondResp.ID != firstResp.ID {
		t.Fatalf("upsert: expected same row id, got first=%s second=%s", firstResp.ID, secondResp.ID)
	}

	var count int64
	h.db.Model(&model.EmployeeAsset{}).Where("agent_id = ?", emp.agentID).Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 row after re-upload, got %d", count)
	}

	var row model.EmployeeAsset
	h.db.Where("id = ?", secondResp.ID).First(&row)
	if row.Bytes != secondResp.Bytes {
		t.Errorf("row bytes %d != response bytes %d", row.Bytes, secondResp.Bytes)
	}
}

func TestEmployeeAssets_DeleteRequiresValidBearer(t *testing.T) {
	h := newEmployeeStreamHarness(t)
	emp := h.seedEmployee(t, true)
	urlPath := fmt.Sprintf("/internal/employees/%s/assets/cache/blob.bin", emp.agentID)

	if rr := h.put(t, urlPath, bytes.NewReader([]byte("x")), "application/octet-stream", emp.bridgeKey); rr.Code != http.StatusCreated {
		t.Fatalf("seed upload: %d", rr.Code)
	}

	denied := h.delete(t, urlPath, "wrong-key")
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("wrong bearer should 401, got %d", denied.Code)
	}
	var stillThere int64
	h.db.Model(&model.EmployeeAsset{}).Where("agent_id = ?", emp.agentID).Count(&stillThere)
	if stillThere != 1 {
		t.Errorf("row was deleted despite 401: count=%d", stillThere)
	}

	ok := h.delete(t, urlPath, emp.bridgeKey)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("delete should 204, got %d: %s", ok.Code, ok.Body.String())
	}
	var gone int64
	h.db.Model(&model.EmployeeAsset{}).Where("agent_id = ?", emp.agentID).Count(&gone)
	if gone != 0 {
		t.Errorf("row should be deleted, got count=%d", gone)
	}
}

func TestEmployeeAssets_MoveRelabelsFolderOnly(t *testing.T) {
	h := newEmployeeStreamHarness(t)
	emp := h.seedEmployee(t, true)
	originalPath := fmt.Sprintf("/internal/employees/%s/assets/inbox/report.pdf", emp.agentID)

	createRR := h.put(t, originalPath, bytes.NewReader([]byte("pdf-bytes")), "application/pdf", emp.bridgeKey)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("seed: %d %s", createRR.Code, createRR.Body.String())
	}
	var createResp struct {
		Key       string `json:"key"`
		PublicURL string `json:"asset_url"`
	}
	_ = json.Unmarshal(createRR.Body.Bytes(), &createResp)

	moveRR := h.move(t, emp.agentID, map[string]string{
		"asset":    "inbox/report.pdf",
		"new_path": "archive/2026",
	}, emp.bridgeKey)
	if moveRR.Code != http.StatusOK {
		t.Fatalf("move: %d %s", moveRR.Code, moveRR.Body.String())
	}
	var moveResp struct {
		Path      string `json:"path"`
		Key       string `json:"key"`
		PublicURL string `json:"asset_url"`
	}
	_ = json.Unmarshal(moveRR.Body.Bytes(), &moveResp)

	if moveResp.Path != "archive/2026" {
		t.Errorf("new path = %q", moveResp.Path)
	}
	if moveResp.Key != createResp.Key {
		t.Errorf("S3 key should not change on move: was %q now %q", createResp.Key, moveResp.Key)
	}
	if moveResp.PublicURL != createResp.PublicURL {
		t.Errorf("asset_url should not change on move")
	}
}

func (h *employeeStreamHarness) delete(t *testing.T, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *employeeStreamHarness) move(t *testing.T, agentID uuid.UUID, body map[string]string, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/internal/employees/%s/assets/move", agentID),
		bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
