package sandbox

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type employeeGitIdentity struct {
	Username string
	Email    string
}

func (o *Orchestrator) loadAgentGitIdentity(ctx context.Context, agent *model.Agent) (*employeeGitIdentity, error) {
	identityAgent, err := o.resolveGitIdentityAgent(ctx, agent)
	if err != nil {
		return nil, err
	}
	if identityAgent == nil {
		return nil, nil
	}

	return gitIdentityFromProfile(identityAgent), nil
}

func (o *Orchestrator) loadEmployeeGitIdentity(ctx context.Context, agent *model.Agent) (*employeeGitIdentity, error) {
	return o.loadAgentGitIdentity(ctx, agent)
}

func (o *Orchestrator) resolveGitIdentityAgent(ctx context.Context, agent *model.Agent) (*model.Agent, error) {
	if agent == nil {
		return nil, nil
	}
	if agent.IsEmployee {
		return agent, nil
	}
	if agent.OrgID == nil {
		return agent, nil
	}

	var employee model.Agent
	query := o.db.WithContext(ctx).
		Where("org_id = ? AND id <> ? AND status <> ?", *agent.OrgID, agent.ID, "archived")
	err := query.Order("created_at ASC").First(&employee).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agent, nil
		}
		return nil, err
	}
	return &employee, nil
}

func gitIdentityFromProfile(agent *model.Agent) *employeeGitIdentity {
	username := fallbackGitUsername(agent)
	email := fallbackGitEmail(agent)
	return &employeeGitIdentity{
		Username: strings.TrimSpace(username),
		Email:    strings.TrimSpace(email),
	}
}

func setGitIdentityEnvVars(envVars map[string]string, agent *model.Agent, identity *employeeGitIdentity) {
	if agent == nil {
		return
	}
	envVars["HIVY_GIT_USERNAME"] = employeeGitUsername(agent, identity)
	envVars["HIVY_GIT_EMAIL"] = employeeGitEmail(agent, identity)
}

func employeeGitUsername(agent *model.Agent, identity *employeeGitIdentity) string {
	if identity != nil && strings.TrimSpace(identity.Username) != "" {
		return strings.TrimSpace(identity.Username)
	}
	return fallbackGitUsername(agent)
}

func employeeGitEmail(agent *model.Agent, identity *employeeGitIdentity) string {
	if identity != nil && strings.TrimSpace(identity.Email) != "" {
		return strings.TrimSpace(identity.Email)
	}
	return fallbackGitEmail(agent)
}

func fallbackGitUsername(agent *model.Agent) string {
	if agent == nil {
		return "agent"
	}
	if username := sanitizeName(agent.Name); username != "" {
		return username
	}
	return "hivy"
}

func fallbackGitEmail(agent *model.Agent) string {
	return fallbackGitUsername(agent) + "@users.noreply.github.com"
}
