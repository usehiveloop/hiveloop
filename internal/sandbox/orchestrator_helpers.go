package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
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

func (o *Orchestrator) providerID() string {
	if o == nil || o.provider == nil {
		return ""
	}
	return o.provider.ID()
}

func (o *Orchestrator) runtimeLayout() RuntimeLayout {
	layout := RuntimeLayout{
		AgentRepoDir:    "/work/repos",
		EmployeeRepoDir: "/workspace/repos",
	}
	if o != nil && o.provider != nil {
		providerLayout := o.provider.RuntimeLayout()
		if providerLayout.AgentRepoDir != "" {
			layout.AgentRepoDir = providerLayout.AgentRepoDir
		}
		if providerLayout.EmployeeRepoDir != "" {
			layout.EmployeeRepoDir = providerLayout.EmployeeRepoDir
		}
	}
	return layout
}

func (o *Orchestrator) ensureSandboxProvider(sb *model.Sandbox) error {
	if sb == nil {
		return fmt.Errorf("sandbox is nil")
	}
	expected := o.providerID()
	if expected == "" {
		return fmt.Errorf("sandbox provider not configured")
	}
	if sb.ProviderID == "" {
		sb.ProviderID = ProviderDaytona
	}
	if sb.ProviderID != expected {
		return fmt.Errorf("sandbox %s was created by provider %q; active provider is %q", sb.ID, sb.ProviderID, expected)
	}
	return nil
}

func (o *Orchestrator) ensureTemplateProvider(tmpl *model.SandboxTemplate) error {
	if tmpl == nil {
		return fmt.Errorf("sandbox template is nil")
	}
	expected := o.providerID()
	if expected == "" {
		return fmt.Errorf("sandbox provider not configured")
	}
	if tmpl.ProviderID == "" {
		tmpl.ProviderID = ProviderDaytona
	}
	if tmpl.ProviderID != expected {
		return fmt.Errorf("sandbox template %s was created by provider %q; active provider is %q", tmpl.ID, tmpl.ProviderID, expected)
	}
	return nil
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

func (o *Orchestrator) loadOwningEmployee(ctx context.Context, agent *model.Employee) (*model.Employee, error) {
	if agent == nil || agent.IsEmployee || agent.OrgID == nil {
		return nil, nil
	}
	var employee model.Employee
	err := o.db.WithContext(ctx).
		Where("org_id = ? AND id <> ? AND status <> ?", *agent.OrgID, agent.ID, "archived").
		Order("created_at ASC").
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

func cloneAgentWithInheritedResources(agent *model.Employee, employee *model.Employee) *model.Employee {
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

func (o *Orchestrator) cloneAgentRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Employee) error {
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

	return o.cloneRepositories(ctx, sb, repos, o.runtimeLayout().AgentRepoDir)
}

func (o *Orchestrator) cloneEmployeeSelectedRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Employee) error {
	repos, err := o.loadEmployeeSelectedGitHubRepositories(ctx, agent)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return nil
	}
	return o.cloneRepositories(ctx, sb, repos, o.runtimeLayout().EmployeeRepoDir)
}

func (o *Orchestrator) loadEmployeeSelectedGitHubRepositories(ctx context.Context, agent *model.Employee) ([]repoResource, error) {
	if agent == nil {
		return nil, nil
	}
	return loadSelectedGitHubRepositoriesForAgent(ctx, o.db, agent.ID)
}

func loadSelectedGitHubRepositoriesForAgent(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]repoResource, error) {
	if db == nil {
		return nil, nil
	}
	var agent model.Employee
	err := db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee github resources: %w", err)
	}

	raw := selectedRepositoriesFromResources(agent.Resources)
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

func selectedRepositoriesFromResources(resources model.JSON) any {
	for _, resourceTypes := range resources {
		typesMap, ok := resourceTypes.(map[string]any)
		if !ok {
			continue
		}
		if repos := typesMap["repository"]; repos != nil {
			return repos
		}
	}
	return nil
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
