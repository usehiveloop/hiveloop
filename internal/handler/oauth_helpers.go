package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/oauth2"
)

// isPlaceholderEmail reports whether the email is a generated placeholder.
func fetchGitHubProfile(ctx context.Context, token *oauth2.Token) (*oauthProfile, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("building github user request: %w", err)
	}
	resp, err := client.Do(req)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", fmt.Errorf("building github emails request: %w", err)
	}
	resp, err := client.Do(req)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("building google userinfo request: %w", err)
	}
	resp, err := client.Do(req)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitter.com/2/users/me?user.fields=id,name,username", nil)
	if err != nil {
		return nil, fmt.Errorf("building x user request: %w", err)
	}
	resp, err := client.Do(req)
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
		Email:          "",
		Name:           name,
	}, nil
}

func (h *OAuthHandler) redirectError(w http.ResponseWriter, r *http.Request, errCode string) {
	next := h.oauthNextFromCookie(r)
	h.clearStateCookie(w)
	h.clearVerifierCookie(w)
	h.clearNextCookie(w)

	redirectURL := fmt.Sprintf("%s/auth/signin?error=%s", h.frontendURL, url.QueryEscape(errCode))
	if next != "" {
		redirectURL += "&next=" + url.QueryEscape(next)
	}
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
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

func sanitizeOAuthNext(raw string) string {
	next := strings.TrimSpace(raw)
	if next == "" || len(next) > 2048 {
		return ""
	}
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\r\n") {
		return ""
	}
	return next
}

func (h *OAuthHandler) setNextCookie(w http.ResponseWriter, next string) {
	if next == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_next",
		Value:    url.QueryEscape(next),
		Path:     "/oauth/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *OAuthHandler) oauthNextFromCookie(r *http.Request) string {
	cookie, err := r.Cookie("oauth_next")
	if err != nil || cookie.Value == "" {
		return ""
	}
	next, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return ""
	}
	return sanitizeOAuthNext(next)
}

func (h *OAuthHandler) clearNextCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_next",
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
