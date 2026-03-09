package middleware

import (
	"context"
	"net/http"
	"net/url"

	"gorm.io/gorm"

	"github.com/zitadel/zitadel-go/v3/pkg/authorization"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization/oauth"
	zmw "github.com/zitadel/zitadel-go/v3/pkg/http/middleware"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"

	"github.com/useportal/llmvault/internal/model"
)

// NewZitadelAuth initializes the ZITADEL authorization layer using OAuth2
// introspection authenticated with client credentials from an API application.
// The domain parameter is a full URL like "http://localhost:8085".
func NewZitadelAuth(ctx context.Context, domain, clientID, clientSecret string) (*zmw.Interceptor[*oauth.IntrospectionContext], error) {
	u, err := url.Parse(domain)
	if err != nil {
		return nil, err
	}

	hostname := u.Hostname()
	port := u.Port()
	var opts []zitadel.Option
	if u.Scheme == "http" {
		if port == "" {
			port = "80"
		}
		opts = append(opts, zitadel.WithInsecure(port))
	} else if port != "" && port != "443" {
		opts = append(opts, zitadel.WithPort(mustParseUint16(port)))
	}

	authZ, err := authorization.New(ctx,
		zitadel.New(hostname, opts...),
		oauth.WithIntrospection[*oauth.IntrospectionContext](
			oauth.ClientIDSecretIntrospectionAuthentication(clientID, clientSecret),
		),
	)
	if err != nil {
		return nil, err
	}
	return zmw.New(authZ), nil
}

func mustParseUint16(s string) uint16 {
	var n uint16
	for _, c := range s {
		n = n*10 + uint16(c-'0')
	}
	return n
}

// ResolveOrg returns middleware that reads the ZITADEL auth context,
// extracts the organization ID, loads the LLMVault Org from the database,
// and sets it on the request context.
//
// This must run AFTER the ZITADEL RequireAuthorization middleware.
func ResolveOrg(mw *zmw.Interceptor[*oauth.IntrospectionContext], db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := mw.Context(r.Context())
			zitadelOrgID := authCtx.OrganizationID()
			if zitadelOrgID == "" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
				return
			}

			var org model.Org
			if err := db.Where("zitadel_org_id = ?", zitadelOrgID).First(&org).Error; err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization not found"})
				return
			}

			if !org.Active {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization is inactive"})
				return
			}

			next.ServeHTTP(w, WithOrg(r, &org))
		})
	}
}

// RequireRole returns middleware that checks if the authenticated ZITADEL user
// has the specified project role.
//
// This must run AFTER the ZITADEL RequireAuthorization middleware.
func RequireRole(mw *zmw.Interceptor[*oauth.IntrospectionContext], role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := mw.Context(r.Context())
			if !authCtx.IsGrantedRole(role) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
