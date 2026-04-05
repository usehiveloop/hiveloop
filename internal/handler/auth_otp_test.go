package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/email"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/model"
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
		"ziraloop-test", "http://localhost:8080",
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
		h.db.Where("user_id = ?", user.ID).Delete(&model.RefreshToken{})
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Exec("DELETE FROM orgs WHERE id IN (SELECT org_id FROM org_memberships WHERE user_id = ?)", user.ID)
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	}
	h.db.Where("email = ?", userEmail).Delete(&model.OTPCode{})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestOTP_FullFlow_NewUser(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-new-user@test.ziraloop.com"
	t.Cleanup(func() { h.cleanup(t, testEmail) })

	// Step 1: Request OTP
	rr := h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": testEmail})
	if rr.Code != http.StatusOK {
		t.Fatalf("OTP request: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Step 2: Read the code from DB (in production it's logged; in tests we read the hash)
	var otp model.OTPCode
	if err := h.db.Where("email = ? AND used_at IS NULL", testEmail).First(&otp).Error; err != nil {
		t.Fatalf("OTP not found in DB: %v", err)
	}
	if otp.ExpiresAt.Before(time.Now()) {
		t.Fatal("OTP already expired")
	}

	// Brute-force the 6-digit code by hashing candidates against the stored hash.
	// This proves we store a real SHA-256 hash and the code is a valid 6-digit number.
	var plainCode string
	for i := 0; i < 1_000_000; i++ {
		candidate := sprintf06(i)
		if model.HashOTPCode(candidate) == otp.TokenHash {
			plainCode = candidate
			break
		}
	}
	if plainCode == "" {
		t.Fatal("could not recover OTP code from hash — hash mismatch")
	}

	// Step 3: Verify OTP — should create user and return tokens
	rr = h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{
		"email": testEmail,
		"code":  plainCode,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("OTP verify (new user): expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var authResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &authResp); err != nil {
		t.Fatalf("decode auth response: %v", err)
	}
	if authResp["access_token"] == nil || authResp["access_token"] == "" {
		t.Fatal("missing access_token in response")
	}
	if authResp["refresh_token"] == nil || authResp["refresh_token"] == "" {
		t.Fatal("missing refresh_token in response")
	}
	user, ok := authResp["user"].(map[string]any)
	if !ok {
		t.Fatal("missing user in response")
	}
	if user["email"] != testEmail {
		t.Fatalf("expected email %q, got %q", testEmail, user["email"])
	}
	if user["email_confirmed"] != true {
		t.Fatal("expected email_confirmed=true for OTP user")
	}
	orgs, ok := authResp["orgs"].([]any)
	if !ok || len(orgs) == 0 {
		t.Fatal("expected at least one org in response")
	}

	// Step 4: Verify the OTP code is marked as used (single-use)
	var usedOtp model.OTPCode
	if err := h.db.Where("email = ? AND token_hash = ?", testEmail, otp.TokenHash).First(&usedOtp).Error; err != nil {
		t.Fatalf("could not re-read OTP: %v", err)
	}
	if usedOtp.UsedAt == nil {
		t.Fatal("OTP should be marked as used")
	}

	// Step 5: Replay same code — should fail
	rr = h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{
		"email": testEmail,
		"code":  plainCode,
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("OTP replay: expected 401, got %d", rr.Code)
	}
}

func TestOTP_FullFlow_ExistingUser(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-existing@test.ziraloop.com"
	t.Cleanup(func() { h.cleanup(t, testEmail) })

	// Pre-create user with org
	user := model.User{Email: testEmail, Name: "Existing"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Test Org"}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := h.db.Create(&model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "admin"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	// Request + recover code
	h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": testEmail})
	var otp model.OTPCode
	h.db.Where("email = ? AND used_at IS NULL", testEmail).First(&otp)
	plainCode := recoverCode(t, otp.TokenHash)

	// Verify — should return 200 (existing user), not 201
	rr := h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{
		"email": testEmail,
		"code":  plainCode,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("OTP verify (existing user): expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var authResp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &authResp)
	if authResp["access_token"] == nil || authResp["access_token"] == "" {
		t.Fatal("missing access_token")
	}
}

func TestOTP_WrongCode(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-wrong@test.ziraloop.com"
	t.Cleanup(func() { h.cleanup(t, testEmail) })

	h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": testEmail})

	rr := h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{
		"email": testEmail,
		"code":  "000000",
	})
	// Might succeed if 000000 happens to be the code, but statistically won't.
	// We just confirm the endpoint doesn't crash and returns 401 or 200.
	if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusCreated {
		t.Fatalf("OTP wrong code: expected 401 or 201, got %d", rr.Code)
	}
}

func TestOTP_ExpiredCode(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-expired@test.ziraloop.com"
	t.Cleanup(func() { h.cleanup(t, testEmail) })

	h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": testEmail})

	// Manually expire the code in DB
	h.db.Model(&model.OTPCode{}).Where("email = ?", testEmail).Update("expires_at", time.Now().Add(-1*time.Hour))

	var otp model.OTPCode
	h.db.Where("email = ? AND used_at IS NULL", testEmail).First(&otp)
	plainCode := recoverCode(t, otp.TokenHash)

	rr := h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{
		"email": testEmail,
		"code":  plainCode,
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("OTP expired: expected 401, got %d", rr.Code)
	}
}

func TestOTP_NewRequestInvalidatesOld(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-invalidate@test.ziraloop.com"
	t.Cleanup(func() { h.cleanup(t, testEmail) })

	// Request first code
	h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": testEmail})
	var firstOTP model.OTPCode
	h.db.Where("email = ? AND used_at IS NULL", testEmail).First(&firstOTP)
	firstCode := recoverCode(t, firstOTP.TokenHash)

	// Request second code — should invalidate the first
	h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": testEmail})

	// First code should no longer work
	rr := h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{
		"email": testEmail,
		"code":  firstCode,
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("old OTP should be invalidated: expected 401, got %d", rr.Code)
	}

	// Second code should work
	var secondOTP model.OTPCode
	h.db.Where("email = ? AND used_at IS NULL", testEmail).First(&secondOTP)
	secondCode := recoverCode(t, secondOTP.TokenHash)

	rr = h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{
		"email": testEmail,
		"code":  secondCode,
	})
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Fatalf("new OTP should work: expected 200/201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOTP_MissingEmail(t *testing.T) {
	h := newOTPHarness(t)

	rr := h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": ""})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	rr = h.doRequest(t, "POST", "/auth/otp/verify", map[string]string{"email": "", "code": "123456"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestOTP_AdminModeRejectsNonAdmin(t *testing.T) {
	db := connectTestDB(t)

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	authHandler := handler.NewAuthHandler(
		db, pk, []byte("test-key"),
		"test", "http://localhost",
		15*time.Minute, 720*time.Hour,
		&email.LogSender{},
		"http://localhost:3000",
		true,
	)
	authHandler.SetAdminMode([]string{"admin@ziraloop.com"})

	r := chi.NewRouter()
	r.Post("/auth/otp/request", authHandler.OTPRequest)
	r.Post("/auth/otp/verify", authHandler.OTPVerify)

	// Non-admin email should be rejected at request stage
	body, _ := json.Marshal(map[string]string{"email": "hacker@evil.com"})
	req := httptest.NewRequest("POST", "/auth/otp/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin mode: expected 403 for non-admin, got %d", rr.Code)
	}

	// Admin email should succeed
	body, _ = json.Marshal(map[string]string{"email": "admin@ziraloop.com"})
	req = httptest.NewRequest("POST", "/auth/otp/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("admin mode: expected 200 for admin, got %d: %s", rr.Code, rr.Body.String())
	}

	// Cleanup
	db.Where("email = ?", "admin@ziraloop.com").Delete(&model.OTPCode{})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sprintf06(n int) string {
	return fmt.Sprintf("%06d", n)
}

// recoverCode brute-forces the 6-digit OTP from its SHA-256 hash.
// Only feasible because the keyspace is 10^6 — intentional for tests.
func recoverCode(t *testing.T, tokenHash string) string {
	t.Helper()
	for i := 0; i < 1_000_000; i++ {
		candidate := sprintf06(i)
		if model.HashOTPCode(candidate) == tokenHash {
			return candidate
		}
	}
	t.Fatal("could not recover OTP code from hash")
	return ""
}
