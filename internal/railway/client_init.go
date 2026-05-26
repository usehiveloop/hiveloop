package railway

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Khan/genqlient/graphql"
)

func NewClient(cfg Config) (*Client, error) {
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("railway token is required")
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultGraphQLEndpoint
	}
	httpClient := cfg.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	minInterval := cfg.MinRequestInterval
	if minInterval <= 0 {
		minInterval = defaultMinRequestInterval
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	initialDelay := cfg.InitialBackoff
	if initialDelay <= 0 {
		initialDelay = defaultInitialBackoff
	}
	maxDelay := cfg.MaxBackoff
	if maxDelay <= 0 {
		maxDelay = defaultMaxBackoff
	}
	baseTransport := httpClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	return &Client{
		gql:          graphql.NewClient(endpoint, railwayHTTPClient(httpClient, token, baseTransport, minInterval)),
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
	}, nil
}
