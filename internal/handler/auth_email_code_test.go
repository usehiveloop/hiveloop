package handler_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

type captureTemplateSender struct {
	messages []email.TemplateMessage
}

func (s *captureTemplateSender) Send(context.Context, email.Message) error {
	return nil
}

func (s *captureTemplateSender) SendTemplate(_ context.Context, msg email.TemplateMessage) error {
	s.messages = append(s.messages, msg)
	return nil
}

type emailConfirmationHarness struct {
	db     *gorm.DB
	router *chi.Mux
	sender *captureTemplateSender
}

func newEmailConfirmationHarness(t *testing.T) *emailConfirmationHarness {
	t.Helper()
	db := connectTestDB(t)
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	sender := &captureTemplateSender{}
	authHandler := handler.NewAuthHandler(
		db, pk, []byte("test-signing-key-for-refresh-tokens"),
		"hivy-test", "http://localhost:8080",
		15*time.Minute, 720*time.Hour,
		sender,
		"http://localhost:3000",
		false,
		billing.NewCreditsService(db),
	)

	r := chi.NewRouter()
	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/confirm-email", authHandler.ConfirmEmail)
	r.Post("/auth/resend-confirmation", authHandler.ResendConfirmation)

	return &emailConfirmationHarness{db: db, router: r, sender: sender}
}

func (h *emailConfirmationHarness) doRequest(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
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

func (h *emailConfirmationHarness) cleanupEmail(t *testing.T, emailAddr string) {
	t.Helper()
	t.Cleanup(func() {
		var user model.User
		if err := h.db.Where("email = ?", emailAddr).First(&user).Error; err == nil {
			var orgIDs []string
			h.db.Model(&model.OrgMembership{}).Where("user_id = ?", user.ID).Pluck("org_id", &orgIDs)
			h.db.Where("user_id = ?", user.ID).Delete(&model.RefreshToken{})
			h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
			if len(orgIDs) > 0 {
				h.db.Where("org_id IN ?", orgIDs).Delete(&model.Agent{})
				h.db.Where("org_id IN ?", orgIDs).Delete(&model.CreditLedgerEntry{})
				h.db.Exec("DELETE FROM orgs WHERE id IN ?", orgIDs)
			}
			h.db.Where("id = ?", user.ID).Delete(&model.User{})
		}
		h.db.Where("email = ?", emailAddr).Delete(&model.OTPCode{})
	})
}

func TestEmailPasswordSignup_SendsAndConfirmsSixDigitCode(t *testing.T) {
	h := newEmailConfirmationHarness(t)
	testEmail := "confirm-code@test.usehivy.com"
	h.cleanupEmail(t, testEmail)

	rr := h.doRequest(t, http.MethodPost, "/auth/register", map[string]string{
		"email":    testEmail,
		"password": "password123",
		"name":     "Confirm Code",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("register: got %d body=%s, want 201", rr.Code, rr.Body.String())
	}

	if len(h.sender.messages) != 1 {
		t.Fatalf("sent emails: got %d, want 1", len(h.sender.messages))
	}
	msg := h.sender.messages[0]
	if msg.Slug != email.TmplAuthConfirmEmail {
		t.Fatalf("email slug: got %q, want %q", msg.Slug, email.TmplAuthConfirmEmail)
	}
	code := msg.Variables["code"]
	if len(code) != 6 {
		t.Fatalf("code length: got %q, want 6 digits", code)
	}

	var otp model.OTPCode
	if err := h.db.Where("email = ? AND used_at IS NULL", testEmail).First(&otp).Error; err != nil {
		t.Fatalf("load confirmation code: %v", err)
	}
	if model.HashOTPCode(code) != otp.TokenHash {
		t.Fatal("stored code hash does not match emailed code")
	}

	rr = h.doRequest(t, http.MethodPost, "/auth/confirm-email", map[string]string{
		"email": testEmail,
		"code":  code,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("confirm: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var user model.User
	if err := h.db.Where("email = ?", testEmail).First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if user.EmailConfirmedAt == nil {
		t.Fatal("expected email_confirmed_at to be set")
	}

	var used model.OTPCode
	if err := h.db.Where("id = ?", otp.ID).First(&used).Error; err != nil {
		t.Fatalf("reload code: %v", err)
	}
	if used.UsedAt == nil {
		t.Fatal("confirmation code should be marked used")
	}
}

func TestResendConfirmation_SendsSixDigitCode(t *testing.T) {
	h := newEmailConfirmationHarness(t)
	testEmail := "resend-code@test.usehivy.com"
	h.cleanupEmail(t, testEmail)

	user := model.User{Email: testEmail, Name: "Resend Code"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	rr := h.doRequest(t, http.MethodPost, "/auth/resend-confirmation", map[string]string{
		"email": testEmail,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("resend: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}
	if len(h.sender.messages) != 1 {
		t.Fatalf("sent emails: got %d, want 1", len(h.sender.messages))
	}
	code := h.sender.messages[0].Variables["code"]
	if len(code) != 6 {
		t.Fatalf("code length: got %q, want 6 digits", code)
	}
}
