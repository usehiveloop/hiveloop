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
	xEndpoints "golang.org/x/oauth2/endpoints"
	githubOAuth "golang.org/x/oauth2/github"
	googleOAuth "golang.org/x/oauth2/google"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/auth"
	"github.com/ziraloop/ziraloop/internal/model"
)

// oauthProfile holds the normalised user info fetched from an OAuth provider.
type oauthProfile struct {
	ProviderUserID string
	Email          string
	Name           string
}

// placeholderEmailDomain is the domain used for placeholder emails when a
// provider (e.g. X/Twitter) does not return a user email.
const placeholderEmailDomain = "@placeholder-email.com"

// isPlaceholderEmail reports whether the email is a generated placeholder.
func isPlaceholderEmail(email string) bool {
	return strings.HasSuffix(email, placeholderEmailDomain)
}

// OAuthHandler implements the social-login OAuth flow for GitHub, Google, and X.
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
	xConfig      *oauth2.Config
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
	xClientID, xClientSecret string,
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

	if xClientID != "" && xClientSecret != "" {
		h.xConfig = &oauth2.Config{
			ClientID:     xClientID,
			ClientSecret: xClientSecret,
			Endpoint:     xEndpoints.X,
			RedirectURL:  base + "/oauth/x/callback",
			Scopes:       []string{"tweet.read", "users.read", "offline.access"},
		}
	}

	return h
}

// ---------------------------------------------------------------------------
// Login endpoints — redirect the browser to the provider's authorize URL.
// ---------------------------------------------------------------------------

// GitHubLogin handles GET /oauth/github.
// @Summary Start GitHub OAuth login
// @Description Redirects the browser to GitHub's authorization page. Sets a state cookie for CSRF protection.
// @Tags oauth
// @Success 307 "Redirect to GitHub"
// @Failure 404 {object} errorResponse "Provider not configured"
// @Router /oauth/github [get]
func (h *OAuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	h.beginLogin(w, r, h.githubConfig)
}

// GoogleLogin handles GET /oauth/google.
// @Summary Start Google OAuth login
// @Description Redirects the browser to Google's authorization page. Sets a state cookie for CSRF protection.
// @Tags oauth
// @Success 307 "Redirect to Google"
// @Failure 404 {object} errorResponse "Provider not configured"
// @Router /oauth/google [get]
func (h *OAuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	h.beginLogin(w, r, h.googleConfig)
}

// XLogin handles GET /oauth/x.
// @Summary Start X (Twitter) OAuth login
// @Description Redirects the browser to X's authorization page. Sets state and PKCE verifier cookies.
// @Tags oauth
// @Success 307 "Redirect to X"
// @Failure 404 {object} errorResponse "Provider not configured"
// @Router /oauth/x [get]
func (h *OAuthHandler) XLogin(w http.ResponseWriter, r *http.Request) {
	h.beginLogin(w, r, h.xConfig)
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

	// Generate PKCE verifier (required by X, harmless for GitHub/Google).
	verifier := oauth2.GenerateVerifier()

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/oauth/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
		Value:    verifier,
		Path:     "/oauth/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// Callback endpoints — handle the redirect back from the provider.
// ---------------------------------------------------------------------------

// GitHubCallback handles GET /oauth/github/callback.
// @Summary GitHub OAuth callback
// @Description Handles the redirect from GitHub after authorization. Exchanges the code for a token, creates or links the user account, and redirects to the frontend with a short-lived exchange token.
// @Tags oauth
// @Param code query string true "Authorization code from GitHub"
// @Param state query string true "CSRF state parameter"
// @Success 307 "Redirect to frontend with exchange token"
// @Failure 307 "Redirect to frontend with error"
// @Router /oauth/github/callback [get]
func (h *OAuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	h.handleCallback(w, r, "github", h.githubConfig)
}

// GoogleCallback handles GET /oauth/google/callback.
// @Summary Google OAuth callback
// @Description Handles the redirect from Google after authorization. Exchanges the code for a token, creates or links the user account, and redirects to the frontend with a short-lived exchange token.
// @Tags oauth
// @Param code query string true "Authorization code from Google"
// @Param state query string true "CSRF state parameter"
// @Success 307 "Redirect to frontend with exchange token"
// @Failure 307 "Redirect to frontend with error"
// @Router /oauth/google/callback [get]
func (h *OAuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	h.handleCallback(w, r, "google", h.googleConfig)
}

// XCallback handles GET /oauth/x/callback.
// @Summary X (Twitter) OAuth callback
// @Description Handles the redirect from X after authorization. Exchanges the code for a token using PKCE, creates or links the user account, and redirects to the frontend with a short-lived exchange token.
// @Tags oauth
// @Param code query string true "Authorization code from X"
// @Param state query string true "CSRF state parameter"
// @Success 307 "Redirect to frontend with exchange token"
// @Failure 307 "Redirect to frontend with error"
// @Router /oauth/x/callback [get]
func (h *OAuthHandler) XCallback(w http.ResponseWriter, r *http.Request) {
	h.handleCallback(w, r, "x", h.xConfig)
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

	// 2. Read PKCE verifier.
	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil || verifierCookie.Value == "" {
		h.redirectError(w, r, "missing_verifier")
		return
	}
	h.clearVerifierCookie(w)

	// 3. Check for error from provider.
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		h.redirectError(w, r, errMsg)
		return
	}

	// 4. Exchange authorisation code for a token (with PKCE).
	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectError(w, r, "missing_code")
		return
	}

	token, err := cfg.Exchange(r.Context(), code, oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		slog.Error("oauth code exchange failed", "provider", provider, "error", err)
		h.redirectError(w, r, "exchange_failed")
		return
	}

	// 5. Fetch user profile from provider.
	profile, err := h.fetchProfile(r.Context(), provider, token)
	if err != nil {
		slog.Error("oauth profile fetch failed", "provider", provider, "error", err)
		h.redirectError(w, r, "profile_fetch_failed")
		return
	}

	// 6. If the provider didn't return an email (e.g. X/Twitter), generate a placeholder.
	if profile.Email == "" {
		name := profile.Name
		if name == "" {
			name = profile.ProviderUserID
		}
		profile.Email = strings.ToLower(name) + placeholderEmailDomain
	}

	// 7. Find or create user + link OAuth account.
	user, err := h.findOrCreateUser(provider, profile)
	if err != nil {
		slog.Error("oauth user creation failed", "provider", provider, "error", err)
		h.redirectError(w, r, "account_creation_failed")
		return
	}

	// 8. Issue exchange token and redirect to frontend.
	h.issueExchangeTokenAndRedirect(w, r, provider, user)
}

func (h *OAuthHandler) issueExchangeTokenAndRedirect(w http.ResponseWriter, r *http.Request, provider string, user *model.User) {
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
// @Summary Exchange OAuth token for access and refresh tokens
// @Description Exchanges a short-lived, single-use OAuth exchange token for an access/refresh token pair. The exchange token is obtained from the OAuth callback redirect.
// @Tags oauth
// @Accept json
// @Produce json
// @Param body body exchangeRequest true "Exchange token"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Router /oauth/exchange [post]
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
		// Mark email as confirmed if not already and provider verified it.
		if user.EmailConfirmedAt == nil && !isPlaceholderEmail(email) {
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

	var emailConfirmedAt *time.Time
	if !isPlaceholderEmail(email) {
		emailConfirmedAt = &now
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		user = model.User{
			Email:            email,
			Name:             name,
			EmailConfirmedAt: emailConfirmedAt,
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
	case "x":
		return fetchXProfile(ctx, token)
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

func fetchXProfile(ctx context.Context, token *oauth2.Token) (*oauthProfile, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	resp, err := client.Get("https://api.twitter.com/2/users/me?user.fields=id,name,username")
	if err != nil {
		return nil, fmt.Errorf("fetching x user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("x /users/me returned %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding x user: %w", err)
	}

	name := body.Data.Name
	if name == "" {
		name = body.Data.Username
	}

	return &oauthProfile{
		ProviderUserID: body.Data.ID,
		Email:          "", // X does not provide email via API v2
		Name:           name,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *OAuthHandler) redirectError(w http.ResponseWriter, r *http.Request, errCode string) {
	h.clearStateCookie(w)
	h.clearVerifierCookie(w)
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

func (h *OAuthHandler) clearVerifierCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
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
