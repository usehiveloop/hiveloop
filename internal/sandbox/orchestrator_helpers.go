package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	githubprofile "github.com/usehiveloop/hiveloop/internal/profiles/github"
	"gorm.io/gorm"
)

func disableProviderLifecycle(ctx context.Context, provider Provider, sb *model.Sandbox, externalID string) {
	if err := provider.SetAutoStop(ctx, externalID, 0); err != nil {
		logging.Capture(ctx, fmt.Errorf("disable provider auto-stop sandbox %s: %w", sb.ID, err))
	}
	if err := provider.SetAutoArchive(ctx, externalID, 0); err != nil {
		logging.Capture(ctx, fmt.Errorf("disable provider auto-archive sandbox %s: %w", sb.ID, err))
	}
}

func (o *Orchestrator) mergeUserEnvVars(ctx context.Context, envVars map[string]string, encrypted []byte) {
	if o.encKey == nil || len(encrypted) == 0 {
		return
	}
	decrypted, err := o.encKey.DecryptString(encrypted)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("decrypt user env vars: %w", err))
		return
	}
	var userVars map[string]string
	if err := json.Unmarshal([]byte(decrypted), &userVars); err != nil {
		logging.Capture(ctx, fmt.Errorf("parse user env vars: %w", err))
		return
	}
	for k, v := range userVars {
		if strings.HasPrefix(strings.ToUpper(k), "BRIDGE_") {
			continue
		}
		envVars[k] = v
	}
}

func (o *Orchestrator) loadOwningEmployee(ctx context.Context, agent *model.Agent) (*model.Agent, error) {
	if agent == nil || agent.IsEmployee || agent.OrgID == nil {
		return nil, nil
	}
	var employee model.Agent
	err := o.db.WithContext(ctx).
		Joins("JOIN agent_subagents ON agent_subagents.agent_id = agents.id").
		Where("agent_subagents.subagent_id = ? AND agents.org_id = ? AND agents.is_employee = ? AND agents.is_system = ?", agent.ID, *agent.OrgID, true, false).
		Order("agent_subagents.created_at ASC").
		First(&employee).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &employee, nil
}

func mergeJSONMaps(base model.JSON, override model.JSON) model.JSON {
	if len(base) == 0 && len(override) == 0 {
		return model.JSON{}
	}
	out := make(model.JSON, len(base)+len(override))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range override {
		out[key] = value
	}
	return out
}

func cloneAgentWithInheritedResources(agent *model.Agent, employee *model.Agent) *model.Agent {
	if agent == nil || employee == nil {
		return agent
	}
	clone := *agent
	clone.Resources = mergeJSONMaps(employee.Resources, agent.Resources)
	return &clone
}

func appendUniqueUUIDs(ids []uuid.UUID, seen map[uuid.UUID]bool, values ...uuid.UUID) []uuid.UUID {
	for _, id := range values {
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func (o *Orchestrator) cloneAgentRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Agent) error {
	if len(agent.Resources) == 0 {
		return nil
	}

	var repos []repoResource
	for _, resourceTypes := range agent.Resources {
		typesMap, ok := resourceTypes.(map[string]any)
		if !ok {
			continue
		}
		repoList, ok := typesMap["repository"]
		if !ok {
			continue
		}
		repoSlice, ok := repoList.([]any)
		if !ok {
			continue
		}
		for _, item := range repoSlice {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			repoID, _ := itemMap["id"].(string)
			repoName, _ := itemMap["name"].(string)
			if repoID != "" && repoName != "" {
				repos = append(repos, repoResource{ID: repoID, Name: repoName})
			}
		}
	}

	if len(repos) == 0 {
		return nil
	}

	return o.cloneRepositories(ctx, sb, repos, "/home/daytona/repos")
}

func (o *Orchestrator) cloneEmployeeSelectedRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Agent) error {
	repos, err := o.loadEmployeeSelectedGitHubRepositories(ctx, agent)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return nil
	}
	return o.cloneRepositories(ctx, sb, repos, "/workspace/repos")
}

func (o *Orchestrator) loadEmployeeSelectedGitHubRepositories(ctx context.Context, agent *model.Agent) ([]repoResource, error) {
	if agent == nil {
		return nil, nil
	}

	var profile model.AgentProfile
	err := o.db.WithContext(ctx).
		Where("agent_id = ? AND provider = ? AND status = ? AND deleted_at IS NULL AND revoked_at IS NULL",
			agent.ID, githubprofile.Provider, "active").
		First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee github profile: %w", err)
	}

	raw := profile.Config["selected_repositories"]
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal selected github repositories: %w", err)
	}
	var selected []struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	}
	if err := json.Unmarshal(data, &selected); err != nil {
		return nil, fmt.Errorf("decode selected github repositories: %w", err)
	}

	repos := make([]repoResource, 0, len(selected))
	for _, repo := range selected {
		name := strings.TrimSpace(repo.Name)
		fullName := strings.TrimSpace(repo.FullName)
		if name == "" || fullName == "" {
			continue
		}
		repos = append(repos, repoResource{ID: fullName, Name: name})
	}
	return repos, nil
}

func (o *Orchestrator) cloneRepositories(ctx context.Context, sb *model.Sandbox, repos []repoResource, baseDir string) error {
	if _, err := o.ExecuteCommand(ctx, sb, "mkdir -p "+baseDir); err != nil {
		return fmt.Errorf("creating repos directory: %w", err)
	}
	for _, repo := range repos {
		repoPath := baseDir + "/" + repo.Name
		cloneURL := "https://github.com/" + repo.ID + ".git"

		if _, err := o.ExecuteCommand(ctx, sb,
			fmt.Sprintf("git clone --depth=1 %s %s", cloneURL, repoPath)); err != nil {
			return fmt.Errorf("cloning %s: %w", repo.ID, err)
		}
	}

	return nil
}

func (o *Orchestrator) runSetupCommands(ctx context.Context, sb *model.Sandbox, commands []string) error {
	for _, cmd := range commands {
		if _, err := o.ExecuteCommand(ctx, sb, cmd); err != nil {
			return fmt.Errorf("setup command failed: %s: %w", cmd, err)
		}
	}
	return nil
}
