package handler_test

import (
	"bytes"
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
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOTP_FullFlow_NewUser(t *testing.T) {
	h := newOTPHarness(t)
	testEmail := "otp-new-user@test.hiveloop.com"
	h.cleanup(t, testEmail) // Remove stale data from prior runs
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
	testEmail := "otp-existing@test.hiveloop.com"
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

