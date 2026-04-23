package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/oauth2"

)

// oauthProfile holds the normalised user info fetched from an OAuth provider.

// provider (e.g. X/Twitter) does not return a user email.

// isPlaceholderEmail reports whether the email is a generated placeholder.
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
