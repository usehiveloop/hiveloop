package sandbox

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	githubprofile "github.com/usehiveloop/hiveloop/internal/profiles/github"
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

	identity, err := o.loadGitHubProfileIdentity(ctx, identityAgent.ID)
	if err != nil {
		return nil, err
	}
	return gitIdentityFromProfile(identityAgent, identity), nil
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

	var employee model.Agent
	query := o.db.WithContext(ctx).
		Joins("JOIN agent_subagents ON agent_subagents.agent_id = agents.id").
		Where("agent_subagents.subagent_id = ? AND agents.is_employee = ?", agent.ID, true)
	if agent.OrgID != nil {
		query = query.Where("agents.org_id = ?", *agent.OrgID)
	}
	err := query.Order("agent_subagents.created_at ASC").First(&employee).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agent, nil
		}
		return nil, err
	}
	return &employee, nil
}

func (o *Orchestrator) loadGitHubProfileIdentity(ctx context.Context, agentID uuid.UUID) (model.JSON, error) {
	var profile model.AgentProfile
	err := o.db.WithContext(ctx).
		Where("agent_id = ? AND provider = ? AND status = ? AND deleted_at IS NULL AND revoked_at IS NULL", agentID, githubprofile.Provider, "active").
		First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.JSON{}, nil
		}
		return nil, err
	}
	return githubprofile.DecryptIdentity(o.encKey, profile)
}

func gitIdentityFromProfile(agent *model.Agent, identity model.JSON) *employeeGitIdentity {
	username, email := githubprofile.GitAuthor(identity, "")
	if strings.TrimSpace(username) == "" {
		username = fallbackGitUsername(agent)
	}
	if strings.TrimSpace(email) == "" {
		email = fallbackGitEmail(agent)
	}
	return &employeeGitIdentity{
		Username: strings.TrimSpace(username),
		Email:    strings.TrimSpace(email),
	}
}

func setGitIdentityEnvVars(envVars map[string]string, agent *model.Agent, identity *employeeGitIdentity) {
	if agent == nil {
		return
	}
	envVars["HIVELOOP_GIT_USERNAME"] = employeeGitUsername(agent, identity)
	envVars["HIVELOOP_GIT_EMAIL"] = employeeGitEmail(agent, identity)
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
	return "agent-" + shortID(agent.ID)
}

func fallbackGitEmail(agent *model.Agent) string {
	return fallbackGitUsername(agent) + "@users.noreply.github.com"
}
