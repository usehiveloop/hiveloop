package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

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

// TestOAuth_Exchange_UsedToken_NoNewSession verifies that a used token cannot be exchanged
// and that no new access token can be created from it. This tests business logic.
func TestOAuth_Exchange_UsedToken_NoNewSession(t *testing.T) {
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

	// Count refresh tokens before
	var tokenCountBefore int64
	h.db.Model(&model.RefreshToken{}).Where("user_id = ?", user.ID).Count(&tokenCountBefore)

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify no new refresh token was created
	var tokenCountAfter int64
	h.db.Model(&model.RefreshToken{}).Where("user_id = ?", user.ID).Count(&tokenCountAfter)
	if tokenCountAfter != tokenCountBefore {
		t.Errorf("no new token should be created for used token, before=%d after=%d", tokenCountBefore, tokenCountAfter)
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