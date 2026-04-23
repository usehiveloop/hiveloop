package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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

	authHandler := handler.NewAuthHandler(h.db, nil, privKey, []byte("invite-e2e-hmac"),
		inviteTestIssuer, inviteTestAudience, 15*time.Minute, 24*time.Hour,
		&email.LogSender{}, "http://localhost:3000", true)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	authHandler.StartCleanup(ctx)

	orgInviteHandler := handler.NewOrgInviteHandler(h.db, &email.LogSender{}, "http://localhost:3000")

	r := chi.NewRouter()

	r.Post("/auth/register", authHandler.Register)
	r.Get("/v1/invites/{token}", orgInviteHandler.Preview)

	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(pubKey, inviteTestIssuer, inviteTestAudience))

		r.Post("/invites/{token}/accept", orgInviteHandler.Accept)
		r.Post("/invites/{token}/decline", orgInviteHandler.Decline)

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
