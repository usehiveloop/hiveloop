package execute

import (
	"context"
	"encoding/json"

	"github.com/ziraloop/ziraloop/internal/nango"
)

// NangoProxy is the narrow interface the executor depends on for outbound
// API calls. Keeping it as an interface (rather than *nango.Client directly)
// is the whole reason the test suite can run end-to-end without a real
// Nango connection — FakeNangoProxy implements it with scripted responses.
//
// The return type is `any`, not `map[string]any`, because many provider
// endpoints return arrays at the root (GitHub's list endpoints — pulls_list_files,
// issues_list_comments, issues_list_labels_for_repo, etc.) or even scalars.
// The existing nango.Client.ProxyRequestWithHeaders returns map[string]any
// and wraps non-object responses as {"_raw": "..."} — the production adapter
// here unwraps that so the executor sees the natural shape.
//
// In production, ProductionNangoProxy wraps the real nango.Client and
// delegates to ProxyRequestWithHeaders. The two implementations share no
// code; they just share this shape.
type NangoProxy interface {
	Proxy(ctx context.Context, req ProxyRequest) (any, error)
}

// ProxyRequest is the fully-resolved outbound call that the executor hands
// to the NangoProxy implementation. All the dispatcher's work (path
// substitution, query/body mapping, header selection) is already baked in.
// The proxy implementation only needs to deliver it.
type ProxyRequest struct {
	Method         string            // HTTP method (GET, POST, etc.)
	ProviderCfgKey string            // Nango provider config key ({orgID}_{integrationUniqueKey})
	NangoConnID    string            // Nango-assigned connection identifier
	Path           string            // Already-substituted API path (e.g., /repos/octocat/Hello-World/pulls/42)
	Query          map[string]string // Query parameters
	Body           map[string]any    // JSON body (may be empty for GET)
	Headers        map[string]string // Extra headers (Content-Type, provider-version headers, etc.)
}

// ProductionNangoProxy wraps a real *nango.Client and implements NangoProxy
// by delegating to ProxyRequestWithHeaders. This is the only adapter in the
// executor package — all production code uses this; all test code uses
// FakeNangoProxy.
type ProductionNangoProxy struct {
	Client *nango.Client
}

// NewProductionNangoProxy constructs a production proxy adapter. The
// executor only ever calls Proxy(); wrapping the client in this adapter is
// the one place the production code paths and the test code paths meet.
func NewProductionNangoProxy(client *nango.Client) *ProductionNangoProxy {
	return &ProductionNangoProxy{Client: client}
}

// Proxy delegates to nango.Client.ProxyRequestWithHeaders and unwraps the
// non-object-response escape hatch.
//
// The underlying nango.Client is typed to return map[string]any, which is
// fine for endpoints returning JSON objects. For endpoints returning arrays
// (GitHub's list endpoints, Slack's list-shaped results) or scalars, the
// client wraps the body as {"_raw": "<raw json string>"} so the type
// signature still compiles. This adapter reverses that: when the response
// looks like a wrapped raw value, re-parse the inner string as JSON and
// return the parsed value (which may be []any, string, number, etc.).
//
// For normal object responses, just pass the map through.
//
// A body of nil or empty map is passed through as nil so Nango doesn't
// serialize an empty object for GET requests (which some providers treat
// as a bad request).
func (p *ProductionNangoProxy) Proxy(ctx context.Context, req ProxyRequest) (any, error) {
	var body any
	if len(req.Body) > 0 {
		body = req.Body
	}
	result, err := p.Client.ProxyRequestWithHeaders(
		ctx,
		req.Method,
		req.ProviderCfgKey,
		req.NangoConnID,
		req.Path,
		req.Query,
		body,
		req.Headers,
	)
	if err != nil {
		return nil, err
	}
	return unwrapRawResponse(result), nil
}

// unwrapRawResponse detects the {"_raw": "..."} wrapper nango.Client
// produces for non-object responses and parses the inner string. Returns
// the parsed value when the wrapper is detected; otherwise returns the
// original map unchanged.
func unwrapRawResponse(result map[string]any) any {
	if result == nil {
		return nil
	}
	if len(result) != 1 {
		return result
	}
	rawValue, hasRaw := result["_raw"]
	if !hasRaw {
		return result
	}
	rawString, isString := rawValue.(string)
	if !isString {
		return result
	}
	var parsed any
	if err := json.Unmarshal([]byte(rawString), &parsed); err != nil {
		return result
	}
	return parsed
}
