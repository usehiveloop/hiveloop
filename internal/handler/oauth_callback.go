package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

// oauthProfile holds the normalised user info fetched from an OAuth provider.

// provider (e.g. X/Twitter) does not return a user email.

// isPlaceholderEmail reports whether the email is a generated placeholder.
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
		if errors.Is(err, errOAuthLinkRequiresConfirmation) {
			slog.Warn("oauth auto-link blocked for confirmed account",
				"provider", provider, "email", profile.Email)
			h.redirectError(w, r, "account_link_required")
			return
		}
		slog.Error("oauth user creation failed", "provider", provider, "error", err)
		h.redirectError(w, r, "account_creation_failed")
		return
	}

	// 8. Issue exchange token and redirect to frontend.
	h.issueExchangeTokenAndRedirect(w, r, provider, user)
}

