package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/token"
)

func (p *Pusher) RotateAgentToken(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if agent.OrgID == nil {
		return fmt.Errorf("cannot rotate token for agent without org")
	}

	cred, err := credentials.Resolve(ctx, p.db, p.picker, agent)
	if err != nil {
		return fmt.Errorf("resolving agent credential: %w", err)
	}

	proxyToken, jti, err := p.mintAgentToken(agent, cred)
	if err != nil {
		return fmt.Errorf("minting new token: %w", err)
	}

	rotateScopes := buildScopesFromIntegrations(agent.Integrations)
	var rotateScopesJSON model.JSON
	if len(rotateScopes) > 0 {
		rotateScopesJSON = model.JSON{"scopes": rotateScopes}
	}

	now := time.Now()
	expiresAt := now.Add(agentTokenTTL)
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Scopes:       rotateScopesJSON,
		Meta:         model.JSON{"agent_id": agent.ID.String(), "type": "agent_proxy"},
	}
	if err := p.db.Create(&dbToken).Error; err != nil {
		return fmt.Errorf("storing new token: %w", err)
	}

	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}
	if err := client.RotateAPIKey(ctx, agent.ID.String(), proxyToken); err != nil {
		return fmt.Errorf("rotating key in bridge: %w", err)
	}

	p.db.Model(&model.Token{}).
		Where("meta->>'agent_id' = ? AND meta->>'type' = 'agent_proxy' AND jti != ?",
			agent.ID.String(), jti).
		Update("revoked_at", now)

	slog.Info("agent token rotated",
		"agent_id", agent.ID,
		"new_jti", jti,
		"expires_at", expiresAt.Format(time.RFC3339),
	)

	return nil
}

func (p *Pusher) NeedsTokenRotation(agentID string) bool {
	var tok model.Token
	err := p.db.Where("meta->>'agent_id' = ? AND meta->>'type' = 'agent_proxy' AND revoked_at IS NULL",
		agentID).Order("created_at DESC").First(&tok).Error
	if err != nil {
		return true
	}
	return time.Until(tok.ExpiresAt) < tokenRotationWindow
}

func (p *Pusher) mintAgentToken(agent *model.Agent, cred *model.Credential) (tokenStr, jti string, err error) {
	if agent.OrgID == nil {
		return "", "", fmt.Errorf("cannot mint token for agent without org_id")
	}
	tokenStr, jti, err = token.Mint(
		p.signingKey,
		(*agent.OrgID).String(),
		cred.ID.String(),
		agentTokenTTL,
	)
	if err != nil {
		return "", "", err
	}
	tokenStr = "ptok_" + tokenStr
	return tokenStr, jti, nil
}
