package handler

import (
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"github.com/usehiveloop/hiveloop/internal/logging"
)

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

	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		h.redirectError(w, r, "invalid_state")
		return
	}
	h.clearStateCookie(w)

	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil || verifierCookie.Value == "" {
		h.redirectError(w, r, "missing_verifier")
		return
	}
	h.clearVerifierCookie(w)

	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		h.redirectError(w, r, errMsg)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectError(w, r, "missing_code")
		return
	}

	token, err := cfg.Exchange(r.Context(), code, oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "oauth code exchange failed", "provider", provider, "error", err)
		h.redirectError(w, r, "exchange_failed")
		return
	}

	profile, err := h.fetchProfile(r.Context(), provider, token)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "oauth profile fetch failed", "provider", provider, "error", err)
		h.redirectError(w, r, "profile_fetch_failed")
		return
	}

	if profile.Email == "" {
		name := profile.Name
		if name == "" {
			name = profile.ProviderUserID
		}
		profile.Email = strings.ToLower(name) + placeholderEmailDomain
	}

	user, err := h.findOrCreateUser(provider, profile)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "oauth user creation failed", "provider", provider, "error", err)
		h.redirectError(w, r, "account_creation_failed")
		return
	}

	h.issueExchangeTokenAndRedirect(w, r, provider, user)
}
