package railway

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Khan/genqlient/graphql"
)

const DefaultGraphQLEndpoint = "https://backboard.railway.app/graphql/v2"
const (
	defaultHTTPTimeout        = 2 * time.Minute
	defaultMaxAttempts        = 4
	defaultInitialBackoff     = 750 * time.Millisecond
	defaultMaxBackoff         = 8 * time.Second
	defaultMinRequestInterval = 250 * time.Millisecond
)

type Config struct {
	Endpoint           string
	Token              string
	HTTP               *http.Client
	MaxAttempts        int
	InitialBackoff     time.Duration
	MaxBackoff         time.Duration
	MinRequestInterval time.Duration
}

type Client struct {
	gql          graphql.Client
	maxAttempts  int
	initialDelay time.Duration
	maxDelay     time.Duration
}

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
	limiter := &requestLimiter{minInterval: minInterval}
	baseTransport := httpClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	return &Client{
		gql: graphql.NewClient(endpoint, rateLimitedTransport{
			base: authTransport{
				base:  baseTransport,
				token: token,
			},
			limiter: limiter,
		}.client(httpClient)),
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
	}, nil
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t authTransport) client(template *http.Client) *http.Client {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	next := *template
	next.Transport = authTransport{base: base, token: t.token}
	return &next
}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := req.Clone(req.Context())
	next.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(next)
}

type requestLimiter struct {
	mu          sync.Mutex
	next        time.Time
	minInterval time.Duration
}

func (l *requestLimiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if now.Before(l.next) {
		wait = l.next.Sub(now)
		l.next = l.next.Add(l.minInterval)
	} else {
		l.next = now.Add(l.minInterval)
	}
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type rateLimitedTransport struct {
	base    http.RoundTripper
	limiter *requestLimiter
}

func (t rateLimitedTransport) client(template *http.Client) *http.Client {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	next := *template
	next.Transport = rateLimitedTransport{base: base, limiter: t.limiter}
	return &next
}

func (t rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.limiter != nil {
		if err := t.limiter.wait(req.Context()); err != nil {
			return nil, err
		}
	}
	return t.base.RoundTrip(req)
}

type Service struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ServiceDomain struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

type Deployment struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (c *Client) CreateServiceFromImage(ctx context.Context, input CreateServiceInput) (*Service, error) {
	var out struct {
		ServiceCreate Service `json:"serviceCreate"`
	}
	err := c.request(ctx, "CreateRailwaySandboxService", `
mutation CreateRailwaySandboxService($name: String, $projectId: String!, $environmentId: String!, $source: ServiceSourceInput, $variables: EnvironmentVariables) {
  serviceCreate(input: { name: $name, projectId: $projectId, environmentId: $environmentId, source: $source, variables: $variables }) {
    id
    name
  }
}`, map[string]any{
		"name":          input.Name,
		"projectId":     input.ProjectID,
		"environmentId": input.EnvironmentID,
		"source":        map[string]any{"image": input.Image},
		"variables":     input.Variables,
	}, &out)
	if err != nil {
		if isDuplicateServiceNameError(err) {
			return c.ServiceByName(ctx, input.ProjectID, input.Name)
		}
		return nil, err
	}
	return &out.ServiceCreate, nil
}

type CreateServiceInput struct {
	Name          string
	ProjectID     string
	EnvironmentID string
	Image         string
	Variables     map[string]string
}

func (c *Client) CreateServiceDomain(ctx context.Context, projectID, environmentID, serviceID string) (*ServiceDomain, error) {
	var out struct {
		ServiceDomainCreate ServiceDomain `json:"serviceDomainCreate"`
	}
	err := c.request(ctx, "CreateRailwaySandboxServiceDomain", `
mutation CreateRailwaySandboxServiceDomain($environmentId: String!, $serviceId: String!) {
  serviceDomainCreate(input: { environmentId: $environmentId, serviceId: $serviceId }) {
    id
    domain
  }
}`, map[string]any{
		"environmentId": environmentID,
		"serviceId":     serviceID,
	}, &out)
	if err != nil {
		domains, domainErr := c.Domains(ctx, projectID, environmentID, serviceID)
		if domainErr == nil && len(domains) > 0 {
			return &domains[0], nil
		}
		return nil, err
	}
	return &out.ServiceDomainCreate, nil
}

func (c *Client) ServiceByName(ctx context.Context, projectID, name string) (*Service, error) {
	services, err := c.Services(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, service := range services {
		if service.Name == name {
			return &service, nil
		}
	}
	return nil, fmt.Errorf("railway service %q exists but could not be loaded", name)
}

func (c *Client) Services(ctx context.Context, projectID string) ([]Service, error) {
	var out struct {
		Project struct {
			Services struct {
				Edges []struct {
					Node Service `json:"node"`
				} `json:"edges"`
			} `json:"services"`
		} `json:"project"`
	}
	err := c.request(ctx, "RailwaySandboxServices", `
query RailwaySandboxServices($projectId: String!) {
  project(id: $projectId) {
    services {
      edges {
        node {
          id
          name
        }
      }
    }
  }
}`, map[string]any{
		"projectId": projectID,
	}, &out)
	if err != nil {
		return nil, err
	}
	items := make([]Service, 0, len(out.Project.Services.Edges))
	for _, edge := range out.Project.Services.Edges {
		items = append(items, edge.Node)
	}
	return items, nil
}

func (c *Client) DeleteService(ctx context.Context, environmentID, serviceID string) error {
	var out struct {
		ServiceDelete bool `json:"serviceDelete"`
	}
	return c.request(ctx, "DeleteRailwaySandboxService", `
mutation DeleteRailwaySandboxService($environmentId: String!, $serviceId: String!) {
  serviceDelete(environmentId: $environmentId, id: $serviceId)
}`, map[string]any{
		"environmentId": environmentID,
		"serviceId":     serviceID,
	}, &out)
}

func (c *Client) Deployments(ctx context.Context, input DeploymentListInput) ([]Deployment, error) {
	var out struct {
		Deployments struct {
			Edges []struct {
				Node Deployment `json:"node"`
			} `json:"edges"`
		} `json:"deployments"`
	}
	err := c.request(ctx, "RailwaySandboxDeployments", `
query RailwaySandboxDeployments($input: DeploymentListInput!, $first: Int) {
  deployments(input: $input, first: $first) {
    edges {
      node {
        id
        status
        createdAt
        updatedAt
      }
    }
  }
}`, map[string]any{
		"input": map[string]any{
			"projectId":     input.ProjectID,
			"environmentId": input.EnvironmentID,
			"serviceId":     input.ServiceID,
		},
		"first": input.First,
	}, &out)
	if err != nil {
		return nil, err
	}
	items := make([]Deployment, 0, len(out.Deployments.Edges))
	for _, edge := range out.Deployments.Edges {
		items = append(items, edge.Node)
	}
	return items, nil
}

type DeploymentListInput struct {
	ProjectID     string
	EnvironmentID string
	ServiceID     string
	First         int
}

func (c *Client) Domains(ctx context.Context, projectID, environmentID, serviceID string) ([]ServiceDomain, error) {
	var out struct {
		Domains struct {
			ServiceDomains []ServiceDomain `json:"serviceDomains"`
		} `json:"domains"`
	}
	err := c.request(ctx, "RailwaySandboxDomains", `
query RailwaySandboxDomains($environmentId: String!, $projectId: String!, $serviceId: String!) {
  domains(environmentId: $environmentId, projectId: $projectId, serviceId: $serviceId) {
    serviceDomains {
      id
      domain
    }
  }
}`, map[string]any{
		"environmentId": environmentID,
		"projectId":     projectID,
		"serviceId":     serviceID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return out.Domains.ServiceDomains, nil
}

func (c *Client) request(ctx context.Context, opName, query string, variables map[string]any, data any) error {
	resp := graphql.Response{Data: data}
	req := &graphql.Request{
		Query:     query,
		Variables: variables,
		OpName:    opName,
	}
	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		lastErr = c.gql.MakeRequest(ctx, req, &resp)
		if lastErr == nil || !isRetryableGraphQLError(lastErr) || attempt == c.maxAttempts {
			return lastErr
		}
		if err := sleepWithBackoff(ctx, c.initialDelay, c.maxDelay, attempt); err != nil {
			return err
		}
	}
	return lastErr
}

func isRetryableGraphQLError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"429",
		"500",
		"502",
		"503",
		"504",
		"deadline exceeded",
		"connection reset",
		"connection refused",
		"temporary",
		"timeout",
		"too many requests",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func isDuplicateServiceNameError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "service named") && strings.Contains(msg, "already exists")
}

func sleepWithBackoff(ctx context.Context, initial, maxDelay time.Duration, attempt int) error {
	delay := initial
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= maxDelay {
			delay = maxDelay
			break
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
