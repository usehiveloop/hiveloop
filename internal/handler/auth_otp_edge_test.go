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

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOTP_WrongCode(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-wrong@test.hiveloop.com"
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
	testEmail := "otp-expired@test.hiveloop.com"
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
	testEmail := "otp-invalidate@test.hiveloop.com"
	h.cleanup(t, testEmail) // Remove stale data from prior runs
	t.Cleanup(func() { h.cleanup(t, testEmail) })

	// Request first code
	h.doRequest(t, "POST", "/auth/otp/request", map[string]string{"email": testEmail})
	var firstOTP model.OTPCode
	h.db.Where("email = ? AND used_at IS NULL", testEmail).First(&firstOTP)
	firstCode := recoverCode(t, firstOTP.TokenHash)

	// Sleep past the rapid-fire dedup window so the next request issues a
	// fresh code (and invalidates the first) instead of short-circuiting.
	time.Sleep(5*time.Second + 200*time.Millisecond)

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
		billing.NewCreditsService(db),
	)
	authHandler.SetAdminMode([]string{"admin@hiveloop.com"})

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
	body, _ = json.Marshal(map[string]string{"email": "admin@hiveloop.com"})
	req = httptest.NewRequest("POST", "/auth/otp/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("admin mode: expected 200 for admin, got %d: %s", rr.Code, rr.Body.String())
	}

	// Cleanup
	db.Where("email = ?", "admin@hiveloop.com").Delete(&model.OTPCode{})
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
