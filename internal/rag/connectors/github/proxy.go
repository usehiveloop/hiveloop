// Nango proxy boundary.
//
// Production wires *nango.Client (which exposes RawProxyRequest with the
// signature the connector needs); tests wire a fake replayer. The
// connector code never sees, stores, or logs a GitHub token: every call
// is a `proxyClient.Do(ctx, method, path, query) → (status, headers, body)`
// and Nango injects the bearer header on its side.
//
// The seam lives at this package boundary on purpose — the alternative
// (mock the GitHub HTTP API directly via httptest) requires teaching
// nango.Client about a custom endpoint per test, which is more wiring
// than just swapping the proxyClient interface implementation.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/usehiveloop/hiveloop/internal/nango"
)

// proxyClient is the small shape the connector consumes. Both
// *nango.Client (via nangoProxy below) and the test fake satisfy it.
//
// The signature deliberately uses url.Values rather than a freeform
// map so callers don't have to think about URL-encoding when shipping
// queries like state=open&page=2&per_page=100.
type proxyClient interface {
	Do(ctx context.Context, method, path string, query url.Values) (int, http.Header, []byte, error)
}

// nangoProxy adapts *nango.Client to proxyClient. It targets
// RawProxyRequest, which (unlike ProxyRequest) does not parse JSON and
// does not raise on non-2xx — both behaviors the connector must handle
// itself (rate-limit headers on a 403 etc.).
type nangoProxy struct {
	client            *nango.Client
	providerConfigKey string
	connectionID      string
}

// newNangoProxy is the production-side constructor. The connector binds
// providerConfigKey + connectionID once at construction time; every call
// then carries the same context-of-trust.
func newNangoProxy(c *nango.Client, providerConfigKey, connectionID string) proxyClient {
	return &nangoProxy{
		client:            c,
		providerConfigKey: providerConfigKey,
		connectionID:      connectionID,
	}
}

// Do issues the proxy call. We use Encode() rather than handing the map
// to nango — nango.RawProxyRequest takes a raw query string and we want
// proper RFC 3986 escaping (e.g. PR titles in `since=` if we ever push
// that filter to GitHub).
func (p *nangoProxy) Do(
	ctx context.Context, method, path string, query url.Values,
) (int, http.Header, []byte, error) {
	rawQuery := ""
	if len(query) > 0 {
		rawQuery = query.Encode()
	}
	resp, err := p.client.RawProxyRequest(
		ctx, method, p.providerConfigKey, p.connectionID, path, rawQuery, nil, "",
	)
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, resp.Body, nil
}

// getJSON is the typed-decode helper layered on top of proxyClient.
//
// Behaviour:
//   - Issues the request via p.Do.
//   - On rate-limit (403/429 + headers), returns a *rateLimitError so
//     withRateLimitRetry can sleep + retry.
//   - On any other 4xx/5xx, returns a plain error including the body
//     prefix for grep-ability in logs.
//   - On 2xx, json.Unmarshal into T and return T + the response headers
//     (callers need them for pagination's Link parsing).
func getJSON[T any](
	ctx context.Context, p proxyClient, method, path string, query url.Values,
) (T, http.Header, error) {
	var zero T
	status, headers, body, err := p.Do(ctx, method, path, query)
	if err != nil {
		return zero, nil, err
	}
	if resetAt, ok := detectRateLimit(status, headers); ok {
		return zero, headers, &rateLimitError{resetAt: resetAt}
	}
	if status >= 400 {
		preview := body
		if len(preview) > 256 {
			preview = preview[:256]
		}
		return zero, headers, fmt.Errorf("github %s %s: status %d: %s", method, path, status, string(preview))
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, headers, fmt.Errorf("github %s %s: decode: %w", method, path, err)
	}
	return out, headers, nil
}
