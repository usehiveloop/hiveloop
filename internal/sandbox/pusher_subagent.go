package sandbox

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/token"
)

func (p *Pusher) buildSubagentDefinitions(parent *model.Agent, parentCred *model.Credential) ([]bridgepkg.AgentDefinition, error) {
	var links []model.AgentSubagent
	if err := p.db.Where("agent_id = ?", parent.ID).Find(&links).Error; err != nil {
		return nil, fmt.Errorf("querying agent_subagents: %w", err)
	}
	if len(links) == 0 {
		return nil, nil
	}

	subagentIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		subagentIDs[index] = link.SubagentID
	}

	var subagents []model.Agent
	if err := p.db.Where("id IN ?", subagentIDs).Find(&subagents).Error; err != nil {
		return nil, fmt.Errorf("loading subagents: %w", err)
	}

	defs := make([]bridgepkg.AgentDefinition, 0, len(subagents))
	for _, sub := range subagents {
		sub.Model = parent.Model

		proxyTok, err := token.MintAndPersist(p.db, p.signingKey, *parent.OrgID, parentCred.ID, agentTokenTTL, map[string]any{
			"agent_id":        sub.ID.String(),
			"parent_agent_id": parent.ID.String(),
			"type":            "subagent_proxy",
		})
		if err != nil {
			return nil, fmt.Errorf("minting proxy token for subagent %s: %w", sub.ID, err)
		}

		defs = append(defs, p.buildAgentDefinition(&sub, parentCred, proxyTok.TokenString, proxyTok.JTI))
	}

	return defs, nil
}
