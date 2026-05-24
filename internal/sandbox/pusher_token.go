package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/token"
)

func (p *Pusher) RotateEmployeeProxyToken(ctx context.Context, agent *model.Employee, sb *model.Sandbox) error {
	if agent.OrgID == nil {
		return fmt.Errorf("cannot rotate token for agent without org")
	}

	cred, err := credentials.Resolve(ctx, p.db, p.picker, agent)
	if err != nil {
		return fmt.Errorf("resolving agent credential: %w", err)
	}

	proxyToken, jti, err := p.mintEmployeeProxyToken(agent, cred)
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
		Meta:         model.JSON{"employee_id": agent.ID.String(), "type": "employee_proxy"},
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
		Where("meta->>'employee_id' = ? AND meta->>'type' = 'employee_proxy' AND jti != ?",
			agent.ID.String(), jti).
		Update("revoked_at", now)

	logging.FromContext(ctx).DebugContext(ctx, "agent token rotated", "employee_id", agent.ID, "jti", jti)

	return nil
}

func (p *Pusher) NeedsTokenRotation(agentID string) bool {
	var tok model.Token
	err := p.db.Where("meta->>'employee_id' = ? AND meta->>'type' = 'employee_proxy' AND revoked_at IS NULL",
		agentID).Order("created_at DESC").First(&tok).Error
	if err != nil {
		return true
	}
	return time.Until(tok.ExpiresAt) < tokenRotationWindow
}

func (p *Pusher) mintEmployeeProxyToken(agent *model.Employee, cred *model.Credential) (tokenStr, jti string, err error) {
	if agent.OrgID == nil {
		return "", "", fmt.Errorf("cannot mint token for agent without org_id")
	}
	tokenStr, jti, err = token.Mint(
		p.signingKey,
		(*agent.OrgID).String(),
		cred.ID.String(),
		agentTokenTTL,
		token.MintOptions{IsSystem: cred.IsSystem},
	)
	if err != nil {
		return "", "", err
	}
	tokenStr = "ptok_" + tokenStr
	return tokenStr, jti, nil
}
