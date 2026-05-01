package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
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

func (o *Orchestrator) cloneAgentRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Agent) error {
	if agent.Resources == nil || len(agent.Resources) == 0 {
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

	if _, err := o.ExecuteCommand(ctx, sb, "mkdir -p /home/daytona/repos"); err != nil {
		return fmt.Errorf("creating repos directory: %w", err)
	}

	for _, repo := range repos {
		repoPath := "/home/daytona/repos/" + repo.Name
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
