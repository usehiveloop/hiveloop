package handler

import (
	"log/slog"
	"net/http"

	"golang.org/x/oauth2"

)

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

