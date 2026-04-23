package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type otpTestHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newOTPHarness(t *testing.T) *otpTestHarness {
	t.Helper()

	db := connectTestDB(t)

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	signingKey := []byte("test-signing-key-for-refresh-tokens")

	authHandler := handler.NewAuthHandler(
		db, pk, signingKey,
		"hiveloop-test", "http://localhost:8080",
		15*time.Minute, 720*time.Hour,
		&email.LogSender{},
		"http://localhost:3000",
		true, // autoConfirmEmail
	)

	r := chi.NewRouter()
	r.Post("/auth/otp/request", authHandler.OTPRequest)
	r.Post("/auth/otp/verify", authHandler.OTPVerify)

	return &otpTestHarness{db: db, router: r}
}

func (h *otpTestHarness) doRequest(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *otpTestHarness) cleanup(t *testing.T, userEmail string) {
	t.Helper()
	var user model.User
	if err := h.db.Where("email = ?", userEmail).First(&user).Error; err == nil {
		// Collect org IDs before deleting memberships, since the subquery depends on them.
		var orgIDs []string
		h.db.Model(&model.OrgMembership{}).Where("user_id = ?", user.ID).Pluck("org_id", &orgIDs)

		h.db.Where("user_id = ?", user.ID).Delete(&model.RefreshToken{})
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		if len(orgIDs) > 0 {
			h.db.Exec("DELETE FROM orgs WHERE id IN ?", orgIDs)
		}
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	}
	h.db.Where("email = ?", userEmail).Delete(&model.OTPCode{})

	// Clean up any orphaned orgs from prior broken cleanup runs.
	// The OTP flow names orgs "<local-part>'s Workspace".
	localPart := strings.Split(userEmail, "@")[0]
	orgName := fmt.Sprintf("%s's Workspace", localPart)
	h.db.Where("name = ?", orgName).Delete(&model.Org{})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

