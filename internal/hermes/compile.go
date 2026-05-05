// Package hermes also provides the control-plane compile step that turns a
// model.Agent into a *sdk.SyncRequest the sidecar can apply directly.
package hermes

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/token"

	hsdk "github.com/usehiveloop/hermes/pkg/sdk"
)

const (
	agentTokenTTL          = 24 * time.Hour
	tokenPrefix            = "ptok_"
	hermesProxyPathOpenAI  = "/v1"
	hermesProxyPathDefault = ""
	hindsightBankFmt       = "bank-%s"
)

// hermesAnthropicProviders names the credential providers whose upstream
// natively speaks Anthropic Messages. For these we drive Hermes with
// provider=anthropic so the Anthropic SDK appends /v1/messages itself.
var hermesAnthropicProviders = map[string]bool{
	"anthropic": true,
}

func hermesProviderForCredential(cred *model.Credential, cfg *config.Config) (provider, baseURL string) {
	host := cfg.ProxyHost
	if hermesAnthropicProviders[cred.ProviderID] {
		return "anthropic", fmt.Sprintf("https://%s%s", host, hermesProxyPathDefault)
	}
	return "custom", fmt.Sprintf("https://%s%s", host, hermesProxyPathOpenAI)
}

var hermesAPIServerToolsets = []string{
	"terminal", "file", "skills", "todo",
	"memory", "session_search", "vision", "cronjob",
}

type CompileDeps struct {
	DB         *gorm.DB
	Picker     credentials.Picker
	KMS        *crypto.KeyWrapper
	EncKey     *crypto.SymmetricKey
	SigningKey []byte
	Cfg        *config.Config
}

func Compile(ctx context.Context, deps CompileDeps, agent *model.Agent) (*hsdk.SyncRequest, error) {
	if agent == nil {
		return nil, fmt.Errorf("hermes.Compile: nil agent")
	}
	if agent.OrgID == nil {
		return nil, fmt.Errorf("hermes.Compile: agent %s has no org_id", agent.ID)
	}

	cred, err := credentials.Resolve(ctx, deps.DB, deps.Picker, agent)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	proxyToken, jti, err := mintAgentToken(deps.SigningKey, agent, cred)
	if err != nil {
		return nil, fmt.Errorf("mint token: %w", err)
	}

	configYAML, err := buildConfigYAML(agent, cred, proxyToken, jti, deps.Cfg)
	if err != nil {
		return nil, fmt.Errorf("build config.yaml: %w", err)
	}

	envFile, err := buildEnvFile(ctx, deps, agent, proxyToken)
	if err != nil {
		return nil, fmt.Errorf("build .env: %w", err)
	}

	skillFiles, err := buildSkillFiles(ctx, deps.DB, agent.ID)
	if err != nil {
		return nil, fmt.Errorf("build skills: %w", err)
	}

	files := []hsdk.SyncFile{
		fileEntry("config.yaml", configYAML, "0640"),
		fileEntry(".env", envFile, "0600"),
		fileEntry("SOUL.md", []byte(agent.SystemPrompt), "0644"),
	}
	files = append(files, skillFiles...)

	repos := buildRepoSpecs(agent.Resources)

	if err := persistToken(deps.DB, agent, cred, jti); err != nil {
		return nil, fmt.Errorf("persist token: %w", err)
	}

	full := true
	return &hsdk.SyncRequest{
		Version:  agent.UpdatedAt.UnixNano(),
		FullSync: &full,
		Files:    &files,
		Repos:    &repos,
	}, nil
}

func mintAgentToken(signingKey []byte, agent *model.Agent, cred *model.Credential) (string, string, error) {
	tokenStr, jti, err := token.Mint(
		signingKey,
		agent.OrgID.String(),
		cred.ID.String(),
		agentTokenTTL,
		token.MintOptions{IsSystem: cred.IsSystem},
	)
	if err != nil {
		return "", "", err
	}
	return tokenPrefix + tokenStr, jti, nil
}

func persistToken(db *gorm.DB, agent *model.Agent, cred *model.Credential, jti string) error {
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(agentTokenTTL),
		Scopes:       scopesFromIntegrations(agent.Integrations),
		Meta:         model.JSON{"agent_id": agent.ID.String(), "type": "agent_proxy", "harness": "hermes"},
	}
	return db.Create(&dbToken).Error
}

func scopesFromIntegrations(integrations model.JSON) model.JSON {
	if len(integrations) == 0 {
		return nil
	}
	var scopes []map[string]any
	for connID, raw := range integrations {
		cfg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		actionsRaw, ok := cfg["actions"].([]any)
		if !ok {
			continue
		}
		actions := make([]string, 0, len(actionsRaw))
		for _, a := range actionsRaw {
			if s, ok := a.(string); ok {
				actions = append(actions, s)
			}
		}
		if len(actions) > 0 {
			scopes = append(scopes, map[string]any{"connection_id": connID, "actions": actions})
		}
	}
	if len(scopes) == 0 {
		return nil
	}
	return model.JSON{"scopes": scopes}
}

func buildConfigYAML(agent *model.Agent, cred *model.Credential, proxyToken, jti string, cfg *config.Config) ([]byte, error) {
	provider, proxyURL := hermesProviderForCredential(cred, cfg)

	modelBlock := map[string]any{
		"default":  agent.Model,
		"provider": provider,
		"base_url": proxyURL,
		"api_key":  proxyToken,
	}
	customProviders := []map[string]any{}
	if provider == "custom" {
		customProviders = append(customProviders, map[string]any{
			"name":     "hiveloop-proxy",
			"base_url": proxyURL,
			"api_key":  proxyToken,
		})
	}

	root := map[string]any{
		"model":            modelBlock,
		"custom_providers": customProviders,
		"agent": map[string]any{
			"max_turns": 180,
		},
		"terminal": map[string]any{
			"backend": "local",
		},
		"memory": map[string]any{
			"memory_enabled":       true,
			"user_profile_enabled": true,
			"memory_char_limit":    2200,
			"user_char_limit":      1375,
			"nudge_interval":       0,
			"provider":             "hindsight",
		},
		"hindsight": map[string]any{
			"bank_id": fmt.Sprintf(hindsightBankFmt, agent.OrgID.String()),
		},
		"platform_toolsets": map[string]any{
			"api_server": hermesAPIServerToolsets,
		},
	}

	if mcp := buildHiveloopMCP(cfg, jti, proxyToken); mcp != nil {
		root["mcp_servers"] = []any{mcp}
	}

	return yaml.Marshal(root)
}

func buildHiveloopMCP(cfg *config.Config, jti, tok string) map[string]any {
	if cfg.MCPBaseURL == "" || jti == "" {
		return nil
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(cfg.MCPBaseURL, "/"), jti)
	server := map[string]any{
		"name":      "hiveloop",
		"transport": "streamable_http",
		"url":       url,
	}
	if tok != "" {
		server["headers"] = map[string]any{"Authorization": "Bearer " + tok}
	}
	return server
}

func buildRepoSpecs(resources model.JSON) []hsdk.RepoSpec {
	if len(resources) == 0 {
		return nil
	}
	var out []hsdk.RepoSpec
	for _, byType := range resources {
		typesMap, ok := byType.(map[string]any)
		if !ok {
			continue
		}
		repoList, ok := typesMap["repository"].([]any)
		if !ok {
			continue
		}
		for _, item := range repoList {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := itemMap["id"].(string)
			name, _ := itemMap["name"].(string)
			if id == "" || name == "" {
				continue
			}
			out = append(out, hsdk.RepoSpec{
				Url:    fmt.Sprintf("https://github.com/%s.git", id),
				Path:   name,
				Branch: stringPtr(stringFromMap(itemMap, "branch")),
			})
		}
	}
	return out
}

func stringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fileEntry(path string, content []byte, mode string) hsdk.SyncFile {
	m := mode
	return hsdk.SyncFile{
		Path:       path,
		ContentB64: base64.StdEncoding.EncodeToString(content),
		Mode:       &m,
	}
}
