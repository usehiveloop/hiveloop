package employeeruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func buildEmployeeMCPServer(ctx context.Context, deps CompileDeps, agent *model.Employee) any {
	return buildHivyMCPServer(ctx, deps, agent, model.TokenRuntimeModeEmployee, "")
}

func buildEmployeeMCPServerWithToken(deps CompileDeps, token *ProxyTokenResult) any {
	return buildHivyMCPServerWithToken(deps, token)
}

func buildSpecialistMCPServerWithToken(deps CompileDeps, token *ProxyTokenResult) any {
	return buildHivyMCPServerWithToken(deps, token)
}

func buildHivyMCPServerWithToken(deps CompileDeps, token *ProxyTokenResult) any {
	if deps.Cfg == nil || deps.Cfg.MCPBaseURL == "" || token == nil || token.JTI == "" {
		return nil
	}
	return hivyMCPServer(deps.Cfg.MCPBaseURL, token.JTI)
}

func buildHivyMCPServer(ctx context.Context, deps CompileDeps, agent *model.Employee, runtimeMode string, specialistSlug string) any {
	if deps.DB == nil || deps.Cfg == nil || deps.Cfg.MCPBaseURL == "" || agent.OrgID == nil {
		return nil
	}
	var tok model.Token
	q := deps.DB.WithContext(ctx).
		Where("org_id = ? AND expires_at > ? AND revoked_at IS NULL", *agent.OrgID, time.Now()).
		Where("meta->>? = ? AND meta->>? = ? AND meta->>? = ?",
			model.TokenMetaEmployeeID, agent.ID.String(),
			model.TokenMetaType, model.TokenTypeEmployeeProxy,
			model.TokenMetaRuntimeMode, runtimeMode)
	if specialistSlug != "" {
		q = q.Where("meta->>? = ?", model.TokenMetaSpecialistSlug, specialistSlug)
	}
	if err := q.
		Order("created_at DESC").
		First(&tok).Error; err != nil {
		return nil
	}
	return hivyMCPServer(deps.Cfg.MCPBaseURL, tok.JTI)
}

func hivyMCPServer(baseURL, jti string) any {
	url := fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), jti)
	return map[string]any{
		"name":      "hivy",
		"transport": "streamable_http",
		"url":       url,
		"headers": map[string]string{
			"Authorization": employeeMCPAuthorizationHeader(),
		},
	}
}

func employeeMCPAuthorizationHeader() string {
	return "Bearer ${" + ProxyAPIKeyEnv + "}"
}

func upsertHivyMCPServer(servers []any, server any) []any {
	out := make([]any, 0, len(servers)+1)
	for _, existing := range servers {
		if m, ok := existing.(map[string]any); ok {
			if name, _ := m["name"].(string); name == "hivy" {
				continue
			}
		}
		out = append(out, existing)
	}
	return append(out, server)
}
