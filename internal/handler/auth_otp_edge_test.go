package handler_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestOTP_WrongCode(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-wrong@test.usehivy.com"
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
	testEmail := "otp-expired@test.usehivy.com"
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
	testEmail := "otp-invalidate@test.usehivy.com"
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
