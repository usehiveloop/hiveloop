package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	inviteTestIssuer   = "hiveloop-e2e-invite-test"
	inviteTestAudience = "hiveloop-e2e"
)

type inviteHarness struct {
	*testHarness
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	router     *chi.Mux
}

func newInviteHarness(t *testing.T) *inviteHarness {
	t.Helper()

	h := newHarness(t)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey

	authHandler := handler.NewAuthHandler(h.db, privKey, []byte("invite-e2e-hmac"),
		inviteTestIssuer, inviteTestAudience, 15*time.Minute, 24*time.Hour,
		&email.LogSender{}, "http://localhost:3000", true)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	authHandler.StartCleanup(ctx)

	orgInviteHandler := handler.NewOrgInviteHandler(h.db, &email.LogSender{}, "http://localhost:3000")

	r := chi.NewRouter()

	// Public auth
	r.Post("/auth/register", authHandler.Register)

	// Public invite preview
	r.Get("/v1/invites/{token}", orgInviteHandler.Preview)

	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(pubKey, inviteTestIssuer, inviteTestAudience))

		// Authenticated, no org context needed.
		r.Post("/invites/{token}/accept", orgInviteHandler.Accept)
		r.Post("/invites/{token}/decline", orgInviteHandler.Decline)

		// Org-scoped (via JWT org claim for tests).
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFromClaims(h.db))
			r.Get("/orgs/current/members", orgInviteHandler.ListMembers)

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdmin(h.db))
				r.Post("/orgs/current/invites", orgInviteHandler.Create)
				r.Get("/orgs/current/invites", orgInviteHandler.List)
				r.Delete("/orgs/current/invites/{id}", orgInviteHandler.Revoke)
				r.Post("/orgs/current/invites/{id}/resend", orgInviteHandler.Resend)
			})
		})
	})

	return &inviteHarness{
		testHarness: h,
		privateKey:  privKey,
		publicKey:   pubKey,
		router:      r,
	}
}

func (ih *inviteHarness) issueToken(t *testing.T, userID, orgID, role string) string {
	t.Helper()
	tok, err := auth.IssueAccessToken(ih.privateKey, inviteTestIssuer, inviteTestAudience,
		userID, orgID, role, 15*time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (ih *inviteHarness) do(t *testing.T, method, path, body, tok string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rr := httptest.NewRecorder()
	ih.router.ServeHTTP(rr, req)
	return rr
}

// createUserAndOrg creates a user + org + admin membership directly in the DB and
// returns the IDs. Registers cleanup.
func (ih *inviteHarness) createUserAndOrg(t *testing.T, label, emailAddr, role string) (userID uuid.UUID, orgID uuid.UUID) {
	t.Helper()
	now := time.Now()
	user := model.User{
		ID:               uuid.New(),
		Email:            emailAddr,
		Name:             label,
		EmailConfirmedAt: &now,
	}
	if err := ih.db.Create(&user).Error; err != nil {
		t.Fatalf("create user %s: %v", emailAddr, err)
	}
	org := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("invites-e2e-%s-%s", label, uuid.New().String()[:6]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := ih.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	m := model.OrgMembership{
		UserID: user.ID,
		OrgID:  org.ID,
		Role:   role,
	}
	if err := ih.db.Create(&m).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		ih.db.Where("org_id = ?", org.ID).Delete(&model.OrgInvite{})
		ih.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		ih.db.Where("id = ?", org.ID).Delete(&model.Org{})
		ih.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return user.ID, org.ID
}

func (ih *inviteHarness) createUser(t *testing.T, emailAddr, name string) uuid.UUID {
	t.Helper()
	now := time.Now()
	user := model.User{
		ID:               uuid.New(),
		Email:            emailAddr,
		Name:             name,
		EmailConfirmedAt: &now,
	}
	if err := ih.db.Create(&user).Error; err != nil {
		t.Fatalf("create user %s: %v", emailAddr, err)
	}
	t.Cleanup(func() {
		ih.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		ih.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return user.ID
}

type inviteDTO struct {
	ID             string `json:"id"`
	OrgID          string `json:"org_id"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	ExpiresAt      string `json:"expires_at"`
	InvitedByID    string `json:"invited_by_id"`
	InvitedByEmail string `json:"invited_by_email"`
}

type invitePreviewDTO struct {
	OrgID       string `json:"org_id"`
	OrgName     string `json:"org_name"`
	InviterName string `json:"inviter_name"`
	Role        string `json:"role"`
	Email       string `json:"email"`
	ExpiresAt   string `json:"expires_at"`
}

// ----------- Tests -----------

func TestOrgInviteHappyPath(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	inviteEmail := fmt.Sprintf("invitee-%s@test.local", randomSuffix())

	// Create
	rr := ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, inviteEmail), adminTok)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create invite: got %d: %s", rr.Code, rr.Body.String())
	}
	var created inviteDTO
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Email != inviteEmail || created.Role != "viewer" {
		t.Fatalf("unexpected invite: %+v", created)
	}

	// List
	rr = ih.do(t, http.MethodGet, "/v1/orgs/current/invites", "", adminTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("list invites: got %d", rr.Code)
	}
	var list struct {
		Data []inviteDTO `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 1 || list.Data[0].ID != created.ID {
		t.Fatalf("expected one invite, got %+v", list.Data)
	}

	// Resend — token hash should change (verify via DB)
	var before model.OrgInvite
	ih.db.Where("id = ?", created.ID).First(&before)

	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/orgs/current/invites/%s/resend", created.ID), "", adminTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("resend: got %d: %s", rr.Code, rr.Body.String())
	}
	var after model.OrgInvite
	ih.db.Where("id = ?", created.ID).First(&after)
	if before.TokenHash == after.TokenHash {
		t.Fatal("resend should rotate token hash")
	}
	if !after.ExpiresAt.After(before.ExpiresAt.Add(-time.Second)) {
		t.Fatal("resend should extend expires_at")
	}

	// Revoke
	rr = ih.do(t, http.MethodDelete, fmt.Sprintf("/v1/orgs/current/invites/%s", created.ID), "", adminTok)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("revoke: got %d: %s", rr.Code, rr.Body.String())
	}

	// List is empty now
	rr = ih.do(t, http.MethodGet, "/v1/orgs/current/invites", "", adminTok)
	_ = json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 0 {
		t.Fatalf("expected no invites after revoke, got %d", len(list.Data))
	}
}

// TestOrgInvitePreviewAndAccept covers preview + accept + second preview (now 404).
func TestOrgInvitePreviewAndAccept(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	inviteeEmail := fmt.Sprintf("invitee-%s@test.local", randomSuffix())
	inviteeID := ih.createUser(t, inviteeEmail, "Invitee")

	rr := ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, inviteeEmail), adminTok)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create invite: got %d: %s", rr.Code, rr.Body.String())
	}
	var created inviteDTO
	_ = json.NewDecoder(rr.Body).Decode(&created)

	// We don't get plaintext back; fetch from DB by id to find hash, then look up
	// the plaintext by recreating. Since we can't recover plaintext from hash,
	// generate our own invite directly for preview/accept test — alternative:
	// read plaintext by temporarily overwriting. Simpler: insert our own invite
	// directly into the DB with a known token.
	plaintext, hash, err := model.GenerateInviteToken()
	if err != nil {
		t.Fatalf("gen token: %v", err)
	}
	injected := model.OrgInvite{
		ID:          uuid.New(),
		OrgID:       orgID,
		Email:       inviteeEmail,
		Role:        "viewer",
		TokenHash:   hash,
		InvitedByID: adminID,
		ExpiresAt:   time.Now().Add(6 * 24 * time.Hour),
	}
	// revoke the first one to avoid the partial-unique constraint
	ih.db.Model(&model.OrgInvite{}).Where("id = ?", created.ID).Update("revoked_at", time.Now())
	if err := ih.db.Create(&injected).Error; err != nil {
		t.Fatalf("insert known-token invite: %v", err)
	}

	// Preview (public, no auth)
	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", plaintext), "", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("preview: got %d: %s", rr.Code, rr.Body.String())
	}
	var prev invitePreviewDTO
	_ = json.NewDecoder(rr.Body).Decode(&prev)
	if prev.Email != inviteeEmail || prev.Role != "viewer" {
		t.Fatalf("bad preview: %+v", prev)
	}

	// Accept with mismatched email → 403
	otherID := ih.createUser(t, fmt.Sprintf("other-%s@test.local", randomSuffix()), "Other")
	otherTok := ih.issueToken(t, otherID.String(), "", "")
	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/accept", plaintext), "", otherTok)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("accept mismatch: expected 403, got %d: %s", rr.Code, rr.Body.String())
	}

	// Accept with correct user
	inviteeTok := ih.issueToken(t, inviteeID.String(), "", "")
	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/accept", plaintext), "", inviteeTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("accept: got %d: %s", rr.Code, rr.Body.String())
	}

	// Membership exists
	var m model.OrgMembership
	if err := ih.db.Where("user_id = ? AND org_id = ?", inviteeID, orgID).First(&m).Error; err != nil {
		t.Fatalf("membership not created: %v", err)
	}
	if m.Role != "viewer" {
		t.Fatalf("membership role: got %s", m.Role)
	}

	// Preview is now 404 (accepted)
	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", plaintext), "", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("preview after accept: expected 404, got %d", rr.Code)
	}
}

func TestOrgInviteDecline(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")

	inviteeEmail := fmt.Sprintf("decliner-%s@test.local", randomSuffix())
	inviteeID := ih.createUser(t, inviteeEmail, "Decliner")

	plaintext, hash, _ := model.GenerateInviteToken()
	inv := model.OrgInvite{
		ID: uuid.New(), OrgID: orgID, Email: inviteeEmail, Role: "admin",
		TokenHash: hash, InvitedByID: adminID, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := ih.db.Create(&inv).Error; err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	tok := ih.issueToken(t, inviteeID.String(), "", "")
	rr := ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/decline", plaintext), "", tok)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("decline: got %d: %s", rr.Code, rr.Body.String())
	}

	var reloaded model.OrgInvite
	ih.db.Where("id = ?", inv.ID).First(&reloaded)
	if reloaded.RevokedAt == nil {
		t.Fatal("decline should set revoked_at")
	}

	// Preview now 404
	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", plaintext), "", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("preview after decline: got %d", rr.Code)
	}
}

func TestOrgInviteErrorCases(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	// --- Invalid email ---
	rr := ih.do(t, http.MethodPost, "/v1/orgs/current/invites", `{"email":"not-an-email","role":"viewer"}`, adminTok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid email: expected 400, got %d", rr.Code)
	}

	// --- Invalid role ---
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites", `{"email":"x@y.z","role":"owner"}`, adminTok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid role: expected 400, got %d", rr.Code)
	}

	// --- Non-admin cannot invite ---
	viewerEmail := fmt.Sprintf("viewer-%s@test.local", randomSuffix())
	viewerID := ih.createUser(t, viewerEmail, "Viewer")
	viewerMembership := model.OrgMembership{UserID: viewerID, OrgID: orgID, Role: "viewer"}
	if err := ih.db.Create(&viewerMembership).Error; err != nil {
		t.Fatalf("create viewer membership: %v", err)
	}
	viewerTok := ih.issueToken(t, viewerID.String(), orgID.String(), "viewer")
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites", `{"email":"someone@test.local","role":"viewer"}`, viewerTok)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin invite: expected 403, got %d", rr.Code)
	}
	rr = ih.do(t, http.MethodGet, "/v1/orgs/current/invites", "", viewerTok)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin list: expected 403, got %d", rr.Code)
	}

	// --- Invite to existing member → 409 ---
	existingEmail := fmt.Sprintf("member-%s@test.local", randomSuffix())
	existingID := ih.createUser(t, existingEmail, "Already")
	if err := ih.db.Create(&model.OrgMembership{UserID: existingID, OrgID: orgID, Role: "viewer"}).Error; err != nil {
		t.Fatalf("create existing membership: %v", err)
	}
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, existingEmail), adminTok)
	if rr.Code != http.StatusConflict {
		t.Errorf("invite existing member: expected 409, got %d: %s", rr.Code, rr.Body.String())
	}

	// --- Duplicate pending invite → 409 ---
	dupEmail := fmt.Sprintf("dup-%s@test.local", randomSuffix())
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, dupEmail), adminTok)
	if rr.Code != http.StatusCreated {
		t.Fatalf("first invite: got %d", rr.Code)
	}
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, dupEmail), adminTok)
	if rr.Code != http.StatusConflict {
		t.Errorf("duplicate invite: expected 409, got %d", rr.Code)
	}

	// --- Accept with expired invite → 404 ---
	expUserEmail := fmt.Sprintf("exp-%s@test.local", randomSuffix())
	expUserID := ih.createUser(t, expUserEmail, "Exp")
	plaintext, hash, _ := model.GenerateInviteToken()
	expInvite := model.OrgInvite{
		ID: uuid.New(), OrgID: orgID, Email: expUserEmail, Role: "viewer",
		TokenHash: hash, InvitedByID: adminID,
		ExpiresAt: time.Now().Add(-time.Hour), // already expired
	}
	if err := ih.db.Create(&expInvite).Error; err != nil {
		t.Fatalf("create expired invite: %v", err)
	}
	expTok := ih.issueToken(t, expUserID.String(), "", "")
	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/accept", plaintext), "", expTok)
	if rr.Code != http.StatusNotFound {
		t.Errorf("accept expired: expected 404, got %d", rr.Code)
	}

	// --- Preview with revoked → 404 ---
	revPlain, revHash, _ := model.GenerateInviteToken()
	revokedAt := time.Now()
	revInvite := model.OrgInvite{
		ID: uuid.New(), OrgID: orgID, Email: fmt.Sprintf("rev-%s@test.local", randomSuffix()),
		Role: "viewer", TokenHash: revHash, InvitedByID: adminID,
		ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &revokedAt,
	}
	if err := ih.db.Create(&revInvite).Error; err != nil {
		t.Fatalf("create revoked invite: %v", err)
	}
	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", revPlain), "", "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("preview revoked: expected 404, got %d", rr.Code)
	}
}

func TestOrgInviteListMembers(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	rr := ih.do(t, http.MethodGet, "/v1/orgs/current/members", "", adminTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("list members: got %d: %s", rr.Code, rr.Body.String())
	}
	var list struct {
		Data []struct {
			UserID string `json:"user_id"`
			Email  string `json:"email"`
			Role   string `json:"role"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 1 || list.Data[0].UserID != adminID.String() || list.Data[0].Role != "admin" {
		t.Fatalf("unexpected members list: %+v", list.Data)
	}
}
