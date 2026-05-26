package railway

import (
	"context"
	"fmt"
	"net/http"
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
