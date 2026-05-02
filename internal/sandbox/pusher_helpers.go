package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)


// harnessFor maps a (provider, model) pair to the ACP harness that should
// drive it. Claude harness handles Anthropic-compatible upstreams; OpenCode
// handles everything else (OpenAI/Google/Groq/etc.).
//
// The model-prefix check wins over the provider type so an OpenAI-shaped
// proxy serving Claude (e.g. Bedrock-via-OpenAI) still lands on the Claude
// harness.
func harnessFor(p bridgepkg.ProviderType, model string) bridgepkg.Harness {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "claude") {
		return bridgepkg.Claude
	}
	if p == bridgepkg.Anthropic {
		return bridgepkg.Claude
	}
	return bridgepkg.OpenCode
}

// resolveHarness returns the harness to stamp on this push. If the agent
// already has a non-empty Harness column we trust it (deterministic across
// pushes); otherwise we compute via harnessFor and the caller is responsible
// for persisting the choice back to the DB.
func resolveHarness(agentHarness string, p bridgepkg.ProviderType, model string) bridgepkg.Harness {
	if agentHarness != "" {
		return bridgepkg.Harness(agentHarness)
	}
	return harnessFor(p, model)
}

func buildHiveLoopMCPServer(mcpBaseURL, jti, token string) bridgepkg.McpServerDefinition {
	url := fmt.Sprintf("%s/%s", mcpBaseURL, jti)

	var transport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type: bridgepkg.StreamableHttp,
		Url:  url,
	}
	if token != "" {
		headers := map[string]string{"Authorization": "Bearer " + token}
		httpTransport.Headers = &headers
	}
	_ = transport.FromMcpTransport1(httpTransport)

	return bridgepkg.McpServerDefinition{
		Name:      "hiveloop",
		Transport: transport,
	}
}

func buildScopesFromIntegrations(integrations model.JSON) []map[string]any {
	if len(integrations) == 0 {
		return nil
	}

	var scopes []map[string]any
	for connectionID, config := range integrations {
		configMap, ok := config.(map[string]any)
		if !ok {
			continue
		}
		actionsRaw, ok := configMap["actions"]
		if !ok {
			continue
		}
		actionsSlice, ok := actionsRaw.([]any)
		if !ok {
			continue
		}
		actions := make([]string, 0, len(actionsSlice))
		for _, action := range actionsSlice {
			if actionStr, ok := action.(string); ok {
				actions = append(actions, actionStr)
			}
		}
		if len(actions) > 0 {
			scopes = append(scopes, map[string]any{
				"connection_id": connectionID,
				"actions":       actions,
			})
		}
	}

	return scopes
}

func decodeJSONAs[T any](j model.JSON) *T {
	if len(j) == 0 {
		return nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil
	}
	var result T
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	return &result
}

func (p *Pusher) loadBridgeSkills(ctx context.Context, agentID uuid.UUID) []bridgepkg.SkillDefinition {
	var links []model.AgentSkill
	if err := p.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&links).Error; err != nil || len(links) == 0 {
		return nil
	}

	skillIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		skillIDs[index] = link.SkillID
	}

	var skills []model.Skill
	if err := p.db.WithContext(ctx).Where("id IN ?", skillIDs).Find(&skills).Error; err != nil {
		return nil
	}

	var versionIDs []uuid.UUID
	for _, skill := range skills {
		if skill.LatestVersionID != nil {
			versionIDs = append(versionIDs, *skill.LatestVersionID)
		}
	}
	if len(versionIDs) == 0 {
		return nil
	}

	var versions []model.SkillVersion
	if err := p.db.WithContext(ctx).Where("id IN ?", versionIDs).Find(&versions).Error; err != nil {
		return nil
	}
	versionByID := make(map[uuid.UUID]model.SkillVersion, len(versions))
	for _, version := range versions {
		versionByID[version.ID] = version
	}

	var result []bridgepkg.SkillDefinition
	for _, skill := range skills {
		if skill.LatestVersionID == nil {
			continue
		}
		version, ok := versionByID[*skill.LatestVersionID]
		if !ok {
			continue
		}
		var def bridgepkg.SkillDefinition
		if err := json.Unmarshal(version.Bundle, &def); err != nil {
			logging.Capture(ctx, fmt.Errorf("unmarshal skill bundle %s: %w", skill.ID, err))
			continue
		}
		result = append(result, def)
	}

	return result
}
