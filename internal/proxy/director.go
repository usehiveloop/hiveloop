package proxy

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/useportal/llmvault/internal/cache"
	"github.com/useportal/llmvault/internal/middleware"
)

// NewDirector returns an httputil.ReverseProxy Director function.
// It resolves the credential from the cache, validates the upstream BaseURL,
// rewrites the URL to the upstream, and attaches the real API key.
//
// The request path is expected to be /v1/proxy/{credentialID}/... where ...
// is forwarded to the upstream.
func NewDirector(cacheManager *cache.Manager) func(req *http.Request) {
	return func(req *http.Request) {
		claims, ok := middleware.ClaimsFromContext(req.Context())
		if !ok {
			// TokenAuth middleware should have rejected this already.
			// Set a sentinel header so the error handler can detect it.
			req.Header.Set("X-Proxy-Error", "missing claims")
			return
		}

		orgID, err := uuid.Parse(claims.OrgID)
		if err != nil {
			req.Header.Set("X-Proxy-Error", "invalid org_id")
			return
		}

		cred, err := cacheManager.GetDecryptedCredential(req.Context(), claims.CredentialID, orgID)
		if err != nil {
			req.Header.Set("X-Proxy-Error", fmt.Sprintf("credential error: %v", err))
			return
		}

		// SSRF hardening: validate destination BaseURL and drop metadata-related headers
		if err := ValidateBaseURL(cred.BaseURL); err != nil {
			req.Header.Set("X-Proxy-Error", fmt.Sprintf("disallowed upstream: %v", err))
			return
		}
		for _, h := range []string{
			"Metadata-Flavor",
			"X-Aws-Ec2-Metadata-Token",
			"X-Aws-Ec2-Metadata-Token-Ttl-Seconds",
			"Metadata",
		} {
			req.Header.Del(h)
		}

		// Rewrite URL: strip /v1/proxy/{credentialID} prefix, append rest to base URL
		upstreamPath := stripProxyPrefix(req.URL.Path, claims.CredentialID)
		baseURL := strings.TrimRight(cred.BaseURL, "/")
		req.URL.Scheme = "https"
		if strings.HasPrefix(baseURL, "http://") {
			req.URL.Scheme = "http"
			baseURL = strings.TrimPrefix(baseURL, "http://")
		} else {
			baseURL = strings.TrimPrefix(baseURL, "https://")
		}

		// Split host from path in base URL
		hostAndPath := strings.SplitN(baseURL, "/", 2)
		req.URL.Host = hostAndPath[0]
		basePath := ""
		if len(hostAndPath) > 1 {
			basePath = "/" + hostAndPath[1]
		}
		req.URL.Path = basePath + upstreamPath
		req.Host = hostAndPath[0]

		// Strip the incoming Authorization header (sandbox token) and attach real API key
		req.Header.Del("Authorization")
		AttachAuth(req, cred.AuthScheme, cred.APIKey)

		// Zero the plaintext API key returned by the cache
		for i := range cred.APIKey {
			cred.APIKey[i] = 0
		}

		// Set tracing header
		req.Header.Set("X-Request-ID", uuid.New().String())
	}
}

// stripProxyPrefix removes the /v1/proxy/{credentialID} prefix from the path.
// Example: /v1/proxy/abc-123/v1/chat/completions → /v1/chat/completions
func stripProxyPrefix(path, credentialID string) string {
	prefix := "/v1/proxy/" + credentialID
	after := strings.TrimPrefix(path, prefix)
	if after == "" {
		return "/"
	}
	return after
}
