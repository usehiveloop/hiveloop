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
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type oauthTestHarness struct {
	db      *gorm.DB
	handler *handler.OAuthHandler
	router  *chi.Mux
}

func TestOAuth_Exchange_Success(t *testing.T) {
	h := newOAuthHarness(t)

	user := createOAuthTestUser(t, h.db, fmt.Sprintf("exchange-ok-%s@test.com", uuid.New().String()[:8]),
		"Exchange OK", "github", fmt.Sprintf("gh-%s", uuid.New().String()[:8]))

	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	et := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	if err := h.db.Create(&et).Error; err != nil {
		t.Fatalf("create exchange token: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", et.ID).Delete(&model.OAuthExchangeToken{}) })

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["access_token"] == nil || resp["access_token"] == "" {
		t.Error("missing access_token")
	}
	if resp["refresh_token"] == nil || resp["refresh_token"] == "" {
		t.Error("missing refresh_token")
	}
	userResp := resp["user"].(map[string]any)
	if userResp["email"] != user.Email {
		t.Errorf("expected email %s, got %v", user.Email, userResp["email"])
	}
}

func TestOAuth_Exchange_MissingToken(t *testing.T) {
	h := newOAuthHarness(t)

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": ""})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestOAuth_Exchange_InvalidToken(t *testing.T) {
	h := newOAuthHarness(t)

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": "bogus-token"})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestOAuth_Exchange_ExpiredToken(t *testing.T) {
	h := newOAuthHarness(t)

	user := createOAuthTestUser(t, h.db, fmt.Sprintf("exchange-exp-%s@test.com", uuid.New().String()[:8]),
		"Exchange Expired", "github", fmt.Sprintf("gh-%s", uuid.New().String()[:8]))

	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	et := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(-1 * time.Minute), // already expired
	}
	if err := h.db.Create(&et).Error; err != nil {
		t.Fatalf("create exchange token: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", et.ID).Delete(&model.OAuthExchangeToken{}) })

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestOAuth_Exchange_UsedToken(t *testing.T) {
	h := newOAuthHarness(t)

	user := createOAuthTestUser(t, h.db, fmt.Sprintf("exchange-used-%s@test.com", uuid.New().String()[:8]),
		"Exchange Used", "github", fmt.Sprintf("gh-%s", uuid.New().String()[:8]))

	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	now := time.Now()
	et := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		UsedAt:    &now, // already used
	}
	if err := h.db.Create(&et).Error; err != nil {
		t.Fatalf("create exchange token: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", et.ID).Delete(&model.OAuthExchangeToken{}) })

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestOAuth_Exchange_InvalidBody(t *testing.T) {
	h := newOAuthHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/oauth/exchange", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestOAuth_Exchange_EmailConfirmedField(t *testing.T) {
	h := newOAuthHarness(t)

	// User with confirmed email
	confirmedUser := createOAuthTestUser(t, h.db,
		fmt.Sprintf("confirmed-%s@test.com", uuid.New().String()[:8]),
		"Confirmed", "google", fmt.Sprintf("goog-%s", uuid.New().String()[:8]))
	now := time.Now()
	h.db.Model(&confirmedUser).Update("email_confirmed_at", &now)

	plaintext1, hash1, _ := model.GenerateExchangeToken()
	h.db.Create(&model.OAuthExchangeToken{UserID: confirmedUser.ID, TokenHash: hash1, ExpiresAt: time.Now().Add(5 * time.Minute)})
	t.Cleanup(func() { h.db.Where("token_hash = ?", hash1).Delete(&model.OAuthExchangeToken{}) })

	rr1 := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext1})
	var resp1 map[string]any
	json.NewDecoder(rr1.Body).Decode(&resp1)
	user1 := resp1["user"].(map[string]any)
	if user1["email_confirmed"] != true {
		t.Errorf("expected email_confirmed=true, got %v", user1["email_confirmed"])
	}

	// User with placeholder email (unconfirmed)
	placeholderUser := createOAuthTestUser(t, h.db,
		fmt.Sprintf("xuser-%s@placeholder-email.com", uuid.New().String()[:8]),
		"X User", "x", fmt.Sprintf("x-%s", uuid.New().String()[:8]))

	plaintext2, hash2, _ := model.GenerateExchangeToken()
	h.db.Create(&model.OAuthExchangeToken{UserID: placeholderUser.ID, TokenHash: hash2, ExpiresAt: time.Now().Add(5 * time.Minute)})
	t.Cleanup(func() { h.db.Where("token_hash = ?", hash2).Delete(&model.OAuthExchangeToken{}) })

	rr2 := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext2})
	var resp2 map[string]any
	json.NewDecoder(rr2.Body).Decode(&resp2)
	user2 := resp2["user"].(map[string]any)
	if user2["email_confirmed"] != false {
		t.Errorf("expected email_confirmed=false for placeholder email, got %v", user2["email_confirmed"])
	}
}
