package handler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
	googleOAuth "golang.org/x/oauth2/google"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/auth"
	"github.com/llmvault/llmvault/internal/model"
)

// oauthProfile holds the normalised user info fetched from an OAuth provider.
type oauthProfile struct {
	ProviderUserID string
	Email          string
	Name           string
}

// OAuthHandler implements the social-login OAuth flow for GitHub and Google.
type OAuthHandler struct {
	db           *gorm.DB
	privateKey   *rsa.PrivateKey
	signingKey   []byte
	issuer       string
	audience     string
	accessTTL    time.Duration
	refreshTTL   time.Duration
	frontendURL  string
	secure       bool // true when cookies should be Secure (HTTPS)
	githubConfig *oauth2.Config
	googleConfig *oauth2.Config
}

// NewOAuthHandler creates an OAuthHandler. If a provider's client ID or secret
// is empty the corresponding oauth2.Config is left nil and the login endpoint
// returns 404.
func NewOAuthHandler(
	db *gorm.DB,
	privateKey *rsa.PrivateKey,
	signingKey []byte,
	issuer, audience string,
	accessTTL, refreshTTL time.Duration,
	frontendURL string,
	githubClientID, githubClientSecret string,
	googleClientID, googleClientSecret string,
) *OAuthHandler {
	h := &OAuthHandler{
		db:          db,
		privateKey:  privateKey,
		signingKey:  signingKey,
		issuer:      issuer,
		audience:    audience,
		accessTTL:   accessTTL,
		refreshTTL:  refreshTTL,
		frontendURL: strings.TrimRight(frontendURL, "/"),
		secure:      strings.HasPrefix(audience, "https://"),
	}

	base := strings.TrimRight(audience, "/")

	if githubClientID != "" && githubClientSecret != "" {
		h.githubConfig = &oauth2.Config{
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			Endpoint:     githubOAuth.Endpoint,
			RedirectURL:  base + "/oauth/github/callback",
			Scopes:       []string{"user:email"},
		}
	}

	if googleClientID != "" && googleClientSecret != "" {
		h.googleConfig = &oauth2.Config{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			Endpoint:     googleOAuth.Endpoint,
			RedirectURL:  base + "/oauth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
		}
	}

	return h
}

// ---------------------------------------------------------------------------
// Login endpoints — redirect the browser to the provider's authorize URL.
// ---------------------------------------------------------------------------

func (h *OAuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	h.beginLogin(w, r, h.githubConfig)
}

func (h *OAuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	h.beginLogin(w, r, h.googleConfig)
}

func (h *OAuthHandler) beginLogin(w http.ResponseWriter, r *http.Request, cfg *oauth2.Config) {
	if cfg == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not configured"})
		return
	}

	state, err := randomHex(32)
	if err != nil {
		slog.Error("failed to generate oauth state", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/oauth/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// Callback endpoints — handle the redirect back from the provider.
// ---------------------------------------------------------------------------

func (h *OAuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	h.handleCallback(w, r, "github", h.githubConfig)
}

func (h *OAuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	h.handleCallback(w, r, "google", h.googleConfig)
}

func (h *OAuthHandler) handleCallback(w http.ResponseWriter, r *http.Request, provider string, cfg *oauth2.Config) {
	if cfg == nil {
		h.redirectError(w, r, "provider_not_configured")
		return
	}

	// 1. Validate state (CSRF).
	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		h.redirectError(w, r, "invalid_state")
		return
	}
	h.clearStateCookie(w)

	// 2. Check for error from provider.
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		h.redirectError(w, r, errMsg)
		return
	}

	// 3. Exchange authorisation code for a token.
	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectError(w, r, "missing_code")
		return
	}

	token, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("oauth code exchange failed", "provider", provider, "error", err)
		h.redirectError(w, r, "exchange_failed")
		return
	}

	// 4. Fetch user profile from provider.
	profile, err := h.fetchProfile(r.Context(), provider, token)
	if err != nil {
		slog.Error("oauth profile fetch failed", "provider", provider, "error", err)
		h.redirectError(w, r, "profile_fetch_failed")
		return
	}

	if profile.Email == "" {
		h.redirectError(w, r, "email_not_available")
		return
	}

	// 5. Find or create user + link OAuth account.
	user, err := h.findOrCreateUser(provider, profile)
	if err != nil {
		slog.Error("oauth user creation failed", "provider", provider, "error", err)
		h.redirectError(w, r, "account_creation_failed")
		return
	}

	// 6. Generate a short-lived exchange token.
	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		slog.Error("failed to generate exchange token", "error", err)
		h.redirectError(w, r, "internal_error")
		return
	}

	exchangeToken := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	if err := h.db.Create(&exchangeToken).Error; err != nil {
		slog.Error("failed to store exchange token", "error", err)
		h.redirectError(w, r, "internal_error")
		return
	}

	// 7. Redirect to frontend with the exchange token.
	redirectURL := fmt.Sprintf("%s/oauth/%s/callback?token=%s", h.frontendURL, provider, plaintext)
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// Exchange endpoint — swap the one-time token for access + refresh tokens.
// ---------------------------------------------------------------------------

type exchangeRequest struct {
	Token string `json:"token"`
}

// Exchange handles POST /oauth/exchange.
func (h *OAuthHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	var req exchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	tokenHash := model.HashExchangeToken(req.Token)

	var et model.OAuthExchangeToken
	err := h.db.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, time.Now()).First(&et).Error
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	// Mark as used.
	now := time.Now()
	if err := h.db.Model(&et).Update("used_at", &now).Error; err != nil {
		slog.Error("failed to mark exchange token as used", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Load user.
	var user model.User
	if err := h.db.Where("id = ?", et.UserID).First(&user).Error; err != nil {
		slog.Error("oauth exchange: user not found", "user_id", et.UserID, "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	// Load memberships.
	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role

	h.issueTokensAndRespond(w, http.StatusOK, user, orgID, role, memberships)
}

// ---------------------------------------------------------------------------
// Token issuance (mirrors AuthHandler.issueTokensAndRespond)
// ---------------------------------------------------------------------------

func (h *OAuthHandler) issueTokensAndRespond(w http.ResponseWriter, status int, user model.User, orgID, role string, memberships []model.OrgMembership) {
	accessToken, err := auth.IssueAccessToken(h.privateKey, h.issuer, h.audience, user.ID.String(), orgID, role, h.accessTTL)
	if err != nil {
		slog.Error("failed to issue access token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	refreshToken, err := auth.IssueRefreshToken(h.signingKey, user.ID.String(), h.refreshTTL)
	if err != nil {
		slog.Error("failed to issue refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Store refresh token hash for revocation tracking.
	sum := sha256.Sum256([]byte(refreshToken))
	storedRefresh := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: time.Now().Add(h.refreshTTL),
	}
	if err := h.db.Create(&storedRefresh).Error; err != nil {
		slog.Error("failed to store refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, m := range memberships {
		orgs = append(orgs, orgMemberDTO{
			ID:   m.OrgID.String(),
			Name: m.Org.Name,
			Role: m.Role,
		})
	}

	writeJSON(w, status, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(h.accessTTL.Seconds()),
		User: userResponse{
			ID:             user.ID.String(),
			Email:          user.Email,
			Name:           user.Name,
			EmailConfirmed: user.EmailConfirmedAt != nil,
		},
		Orgs: orgs,
	})
}

// ---------------------------------------------------------------------------
// Account linking
// ---------------------------------------------------------------------------

func (h *OAuthHandler) findOrCreateUser(provider string, profile *oauthProfile) (*model.User, error) {
	// 1. Check if this OAuth account already exists.
	var existing model.OAuthAccount
	err := h.db.Where("provider = ? AND provider_user_id = ?", provider, profile.ProviderUserID).First(&existing).Error
	if err == nil {
		// Returning user — just load them.
		var user model.User
		if err := h.db.Where("id = ?", existing.UserID).First(&user).Error; err != nil {
			return nil, fmt.Errorf("loading linked user: %w", err)
		}
		return &user, nil
	}

	// 2. No existing link — check if a user with this email exists.
	email := strings.ToLower(strings.TrimSpace(profile.Email))
	var user model.User
	err = h.db.Where("email = ?", email).First(&user).Error

	if err == nil {
		// User exists — link the provider.
		oauthAcct := model.OAuthAccount{
			UserID:         user.ID,
			Provider:       provider,
			ProviderUserID: profile.ProviderUserID,
		}
		if err := h.db.Create(&oauthAcct).Error; err != nil {
			return nil, fmt.Errorf("linking oauth account: %w", err)
		}
		// Mark email as confirmed if not already (provider verified it).
		if user.EmailConfirmedAt == nil {
			now := time.Now()
			h.db.Model(&user).Update("email_confirmed_at", &now)
			user.EmailConfirmedAt = &now
		}
		return &user, nil
	}

	// 3. Brand new user — create everything in a transaction.
	now := time.Now()
	name := profile.Name
	if name == "" {
		name = strings.Split(email, "@")[0]
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		user = model.User{
			Email:            email,
			Name:             name,
			EmailConfirmedAt: &now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("creating user: %w", err)
		}

		org := model.Org{
			Name: fmt.Sprintf("%s's Workspace", name),
		}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("creating org: %w", err)
		}

		membership := model.OrgMembership{
			UserID: user.ID,
			OrgID:  org.ID,
			Role:   "admin",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("creating membership: %w", err)
		}

		oauthAcct := model.OAuthAccount{
			UserID:         user.ID,
			Provider:       provider,
			ProviderUserID: profile.ProviderUserID,
		}
		if err := tx.Create(&oauthAcct).Error; err != nil {
			return fmt.Errorf("creating oauth account: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// ---------------------------------------------------------------------------
// Provider profile fetchers
// ---------------------------------------------------------------------------

func (h *OAuthHandler) fetchProfile(ctx context.Context, provider string, token *oauth2.Token) (*oauthProfile, error) {
	switch provider {
	case "github":
		return fetchGitHubProfile(ctx, token)
	case "google":
		return fetchGoogleProfile(ctx, token)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func fetchGitHubProfile(ctx context.Context, token *oauth2.Token) (*oauthProfile, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	// GET /user
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("fetching github user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github /user returned %d", resp.StatusCode)
	}

	var ghUser struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("decoding github user: %w", err)
	}

	email := ghUser.Email

	// If email is not public, fetch from /user/emails.
	if email == "" {
		email, err = fetchGitHubPrimaryEmail(ctx, client)
		if err != nil {
			return nil, err
		}
	}

	name := ghUser.Name
	if name == "" {
		name = ghUser.Login
	}

	return &oauthProfile{
		ProviderUserID: strconv.FormatInt(ghUser.ID, 10),
		Email:          email,
		Name:           name,
	}, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", fmt.Errorf("fetching github emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github /user/emails returned %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decoding github emails: %w", err)
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	return "", fmt.Errorf("no verified primary email found on github account")
}

func fetchGoogleProfile(ctx context.Context, token *oauth2.Token) (*oauthProfile, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("fetching google userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo returned %d", resp.StatusCode)
	}

	var info struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		VerifiedEmail bool   `json:"verified_email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding google userinfo: %w", err)
	}

	if !info.VerifiedEmail {
		return nil, fmt.Errorf("google email is not verified")
	}

	return &oauthProfile{
		ProviderUserID: info.ID,
		Email:          info.Email,
		Name:           info.Name,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *OAuthHandler) redirectError(w http.ResponseWriter, r *http.Request, errCode string) {
	h.clearStateCookie(w)
	http.Redirect(w, r, fmt.Sprintf("%s/auth?error=%s", h.frontendURL, errCode), http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/oauth/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
