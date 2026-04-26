package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/usehiveloop/hiveloop/internal/nango"
)

type proxyClient interface {
	Do(ctx context.Context, method, path string, query url.Values) (int, http.Header, []byte, error)
}

// nangoProxy targets RawProxyRequest, which (unlike ProxyRequest) does
// not parse JSON and does not raise on non-2xx — the connector must
// handle rate-limit headers on a 403 etc. itself.
type nangoProxy struct {
	client            *nango.Client
	providerConfigKey string
	connectionID      string
}

func newNangoProxy(c *nango.Client, providerConfigKey, connectionID string) proxyClient {
	return &nangoProxy{
		client:            c,
		providerConfigKey: providerConfigKey,
		connectionID:      connectionID,
	}
}

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
