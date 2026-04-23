package handler

import (
	"crypto/rsa"
	"strings"
	"time"

	"golang.org/x/oauth2"
	xEndpoints "golang.org/x/oauth2/endpoints"
	githubOAuth "golang.org/x/oauth2/github"
	googleOAuth "golang.org/x/oauth2/google"
	"gorm.io/gorm"

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
