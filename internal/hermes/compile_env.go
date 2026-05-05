package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

func buildEnvFile(ctx context.Context, deps CompileDeps, agent *model.Agent, proxyToken string) ([]byte, error) {
	env := map[string]string{
		"OPENAI_API_KEY": proxyToken,
		"CUSTOM_API_KEY": proxyToken,
	}

	if err := mergeAgentEncryptedEnv(deps.EncKey, agent, env); err != nil {
		return nil, fmt.Errorf("decrypt agent env vars: %w", err)
	}

	profiles, err := loadAgentProfiles(ctx, deps.DB, agent.ID)
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	for _, profile := range profiles {
		if err := mergeProfileSecrets(ctx, deps.KMS, profile, env); err != nil {
			return nil, fmt.Errorf("decrypt profile %s: %w", profile.Provider, err)
		}
	}

	return formatEnvFile(env), nil
}

func loadAgentProfiles(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]model.AgentProfile, error) {
	var profiles []model.AgentProfile
	if err := db.WithContext(ctx).
		Where("agent_id = ? AND status = ? AND deleted_at IS NULL", agentID, "active").
		Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

func mergeAgentEncryptedEnv(encKey *crypto.SymmetricKey, agent *model.Agent, env map[string]string) error {
	if encKey == nil || len(agent.EncryptedEnvVars) == 0 {
		return nil
	}
	decrypted, err := encKey.DecryptString(agent.EncryptedEnvVars)
	if err != nil {
		return err
	}
	var userVars map[string]string
	if err := json.Unmarshal([]byte(decrypted), &userVars); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	for k, v := range userVars {
		if strings.HasPrefix(strings.ToUpper(k), "BRIDGE_") {
			continue
		}
		env[k] = v
	}
	return nil
}

func mergeProfileSecrets(ctx context.Context, kms *crypto.KeyWrapper, profile model.AgentProfile, env map[string]string) error {
	if kms == nil || len(profile.EncryptedSecrets) == 0 || len(profile.WrappedDEK) == 0 {
		return nil
	}
	dek, err := kms.Unwrap(ctx, profile.WrappedDEK)
	if err != nil {
		return fmt.Errorf("unwrap DEK: %w", err)
	}
	defer wipe(dek)

	plaintext, err := crypto.DecryptCredential(profile.EncryptedSecrets, dek)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	switch profile.Provider {
	case slackprov.Provider:
		return mergeSlackEnv(plaintext, env)
	default:
		return nil
	}
}

func mergeSlackEnv(plaintext []byte, env map[string]string) error {
	var s slackprov.Secrets
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return fmt.Errorf("parse slack secrets: %w", err)
	}
	if s.BotToken != "" {
		env["SLACK_BOT_TOKEN"] = s.BotToken
	}
	if s.AppToken != "" {
		env["SLACK_APP_TOKEN"] = s.AppToken
	}
	env["SLACK_ALLOWED_USERS"] = "*"
	return nil
}

func formatEnvFile(env map[string]string) []byte {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(env[k])
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
