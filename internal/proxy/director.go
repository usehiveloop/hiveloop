package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/observe"
)

// credentialResolver abstracts the credential lookup for the director.
// Extracted from *cache.Manager so tests can inject stubs without DB/KMS.
type credentialResolver interface {
	GetDecryptedCredentialByID(ctx context.Context, credentialID string) (*cache.DecryptedCredential, error)
}

func NewDirector(resolver credentialResolver) func(req *http.Request) {
	return func(req *http.Request) {
		claims, ok := middleware.ClaimsFromContext(req.Context())
		if !ok {
			logging.Capture(req.Context(), fmt.Errorf("proxy director: missing claims on %s", req.URL.Path))
			req.Header.Set("X-Proxy-Error", "missing claims")
			return
		}

		cred, err := resolver.GetDecryptedCredentialByID(req.Context(), claims.CredentialID)
		if err != nil {
			logging.FromContext(req.Context()).Error("proxy director: credential lookup failed",
				"credential_id", claims.CredentialID,
				"jti", claims.JTI,
				"jwt_org_id", claims.OrgID,
				"path", req.URL.Path,
				"method", req.Method,
				"error", err.Error(),
			)
			logging.Capture(req.Context(), fmt.Errorf("proxy director: credential lookup %s: %w", claims.CredentialID, err))
			req.Header.Set("X-Proxy-Error", fmt.Sprintf("credential error: %v", err))
			return
		}

		if err := ValidateBaseURL(cred.BaseURL); err != nil {
			logging.Capture(req.Context(), fmt.Errorf("proxy director: disallowed upstream base URL: %w", err))
			req.Header.Set("X-Proxy-Error", fmt.Sprintf("disallowed upstream: %v", err))
			return
		}
		// SSRF hardening: drop cloud-metadata-related headers.
		// CF-* and X-Forwarded-*: the inbound is fronted by Cloudflare, so those
		// headers are added by CF on the way in. Forwarding them to a different
		// CF-protected upstream (e.g. crof.ai, openai via CF) trips Cloudflare
		// Error 1000 ("DNS points to prohibited IP") because CF treats incoming
		// CF-Connecting-IP from a non-CF source as a forged header.
		for _, h := range []string{
			"Metadata-Flavor",
			"X-Aws-Ec2-Metadata-Token",
			"X-Aws-Ec2-Metadata-Token-Ttl-Seconds",
			"Metadata",
			"CF-Connecting-IP",
			"CF-Connecting-IPv6",
			"CF-Ray",
			"CF-Visitor",
			"CF-IPCountry",
			"CF-Worker",
			"CDN-Loop",
			"True-Client-IP",
			"X-Forwarded-For",
			"X-Forwarded-Proto",
			"X-Forwarded-Host",
			"X-Real-IP",
		} {
			req.Header.Del(h)
		}

		upstreamPath := stripProxyPrefix(req.URL.Path)
		baseURL := strings.TrimRight(cred.BaseURL, "/")
		req.URL.Scheme = "https"
		if strings.HasPrefix(baseURL, "http://") {
			req.URL.Scheme = "http"
			baseURL = strings.TrimPrefix(baseURL, "http://")
		} else {
			baseURL = strings.TrimPrefix(baseURL, "https://")
		}

		hostAndPath := strings.SplitN(baseURL, "/", 2)
		req.URL.Host = hostAndPath[0]
		basePath := ""
		if len(hostAndPath) > 1 {
			basePath = "/" + hostAndPath[1]
		}
		req.URL.Path = basePath + upstreamPath
		req.Host = hostAndPath[0]

		modelName := ExtractModel(req)
		if captured, ok := observe.CapturedDataFromContext(req.Context()); ok {
			captured.Model = modelName
		}

		req.Header.Del("Authorization")
		AttachAuth(req, cred.AuthScheme, cred.APIKey)

		for i := range cred.APIKey {
			cred.APIKey[i] = 0
		}

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
