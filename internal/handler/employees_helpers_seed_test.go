package handler_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func newTestEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 7)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("enckey: %v", err)
	}
	return sk
}

type orgWithMember struct {
	org  model.Org
	user model.User
}

func (h *employeeHarness) createOrg(t *testing.T) orgWithMember {
	return h.createOrgWithRole(t, "admin")
}

func (h *employeeHarness) createOrgWithRole(t *testing.T, role string) orgWithMember {
	t.Helper()
	user := model.User{Email: "emp-" + uuid.NewString()[:8] + "@test.com", Name: "T"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "emp-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	mem := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: role}
	if err := h.db.Create(&mem).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	orgID := org.ID
	userID := user.ID
	t.Cleanup(func() {
		h.db.Where("org_id = ?", orgID).Delete(&model.Sandbox{})
		h.db.Where("org_id = ?", orgID).Delete(&model.Agent{})
		h.db.Where("user_id = ?", userID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", orgID).Delete(&model.Org{})
		h.db.Where("id = ?", userID).Delete(&model.User{})
	})
	return orgWithMember{org: org, user: user}
}

func (h *employeeHarness) seedSystemCred(t *testing.T, providerID string, revoked bool) model.Credential {
	t.Helper()
	cred := model.Credential{
		OrgID:        credentials.PlatformOrgID,
		Label:        "sys-" + providerID,
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   providerID,
		IsSystem:     true,
	}
	if revoked {
		now := time.Now()
		cred.RevokedAt = &now
	}
	if err := h.db.Create(&cred).Error; err != nil {
		t.Fatalf("seed system cred %s: %v", providerID, err)
	}
	t.Cleanup(func() { h.db.Unscoped().Delete(&cred) })
	return cred
}

func (h *employeeHarness) post(t *testing.T, m orgWithMember, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest("POST", "/v1/employees/", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func decodeEmployeeResp(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var out map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, rr.Body.String())
	}
	return out
}

func (h *employeeHarness) seedGlobalSkill(t *testing.T, name, status string) model.Skill {
	t.Helper()
	skill := model.Skill{
		Slug:       name + "-" + uuid.NewString()[:8],
		Name:       name,
		SourceType: model.SkillSourceInline,
		Status:     status,
	}
	if err := h.db.Create(&skill).Error; err != nil {
		t.Fatalf("seed global skill %s: %v", name, err)
	}
	t.Cleanup(func() { h.db.Unscoped().Delete(&skill) })
	return skill
}

func validEmployeeBody() map[string]any {
	return map[string]any{
		"category":    "engineering",
		"name":        "agent-" + uuid.NewString()[:8],
		"avatar_url":  "https://cdn.example/a.png",
		"description": "a software engineer",
	}
}
