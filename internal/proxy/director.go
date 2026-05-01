package proxy

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/observe"
)

// NewDirector returns an httputil.ReverseProxy Director function.
// It resolves the credential from the cache, validates the upstream BaseURL,
// rewrites the URL to the upstream, and attaches the real API key.
//
// The request path is expected to be /v1/proxy/... where ...
// is forwarded to the upstream.
func NewDirector(cacheManager *cache.Manager) func(req *http.Request) {
	return func(req *http.Request) {
		claims, ok := middleware.ClaimsFromContext(req.Context())
		if !ok {
			// TokenAuth middleware should have rejected this already.
			// Set a sentinel header so the error handler can detect it.
			logging.Capture(req.Context(), fmt.Errorf("proxy director: missing claims on %s", req.URL.Path))
			req.Header.Set("X-Proxy-Error", "missing claims")
			return
		}

		orgID, err := uuid.Parse(claims.OrgID)
		if err != nil {
			logging.Capture(req.Context(), fmt.Errorf("proxy director: invalid org_id %q: %w", claims.OrgID, err))
			req.Header.Set("X-Proxy-Error", "invalid org_id")
			return
		}

		// System credentials FK to the platform org, not the customer org on
		// the JWT, so look them up under PlatformOrgID.
		lookupOrgID := orgID
		if claims.IsSystem {
			lookupOrgID = credentials.PlatformOrgID
		}

		cred, err := cacheManager.GetDecryptedCredential(req.Context(), claims.CredentialID, lookupOrgID)
		if err != nil {
			logging.FromContext(req.Context()).Error("proxy director: credential lookup failed",
				"credential_id", claims.CredentialID,
				"jti", claims.JTI,
				"jwt_org_id", claims.OrgID,
				"is_system", claims.IsSystem,
				"lookup_org_id", lookupOrgID.String(),
				"path", req.URL.Path,
				"method", req.Method,
				"error", err.Error(),
			)
			logging.Capture(req.Context(), fmt.Errorf("proxy director: credential lookup %s: %w", claims.CredentialID, err))
			req.Header.Set("X-Proxy-Error", fmt.Sprintf("credential error: %v", err))
			return
		}

		// SSRF hardening: validate destination BaseURL and drop metadata-related headers
		if err := ValidateBaseURL(cred.BaseURL); err != nil {
			logging.Capture(req.Context(), fmt.Errorf("proxy director: disallowed upstream base URL: %w", err))
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

		// Rewrite URL: strip /v1/proxy prefix, append rest to base URL
		upstreamPath := stripProxyPrefix(req.URL.Path)
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

		// Extract model from request body and store on captured data (if present)
		modelName := ExtractModel(req)
		if captured, ok := observe.CapturedDataFromContext(req.Context()); ok {
			captured.Model = modelName
		}

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

// stripProxyPrefix removes the /v1/proxy prefix from the path.
// Example: /v1/proxy/v1/chat/completions → /v1/chat/completions
func stripProxyPrefix(path string) string {
	after := strings.TrimPrefix(path, "/v1/proxy")
	if after == "" {
		return "/"
	}
	return after
}
