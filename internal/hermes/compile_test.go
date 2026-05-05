package hermes_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	yamlv3 "gopkg.in/yaml.v3"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/hermes"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

const testDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" //nolint:gosec // local-dev DSN, mirrors sibling integration tests

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("DB not available: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("DB ping failed: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func testEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 17)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	return sk
}

func testKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	kms, err := crypto.NewAEADWrapper(context.Background(), base64.StdEncoding.EncodeToString(key), "test-kms")
	require.NoError(t, err)
	return kms
}

func seedAgentFixture(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, encKey *crypto.SymmetricKey) (*model.Agent, uuid.UUID) {
	t.Helper()

	org := model.Org{ID: uuid.New(), Name: "compile-org-" + uuid.New().String()[:8], Active: true}
	require.NoError(t, db.Create(&org).Error)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encryptedKey, err := encKey.EncryptString("sk-openai-test")
	require.NoError(t, err)
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "openai", Label: "Test OpenAI",
		EncryptedKey: encryptedKey, WrappedDEK: []byte("dek-placeholder"),
		BaseURL: "https://api.openai.com", AuthScheme: "bearer",
	}
	require.NoError(t, db.Create(&cred).Error)

	encEnvBytes, err := encKey.EncryptString(`{"FOO":"bar","BRIDGE_SECRET":"shouldbeskipped"}`)
	require.NoError(t, err)

	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, Name: "Test Employee",
		IsEmployee:   true,
		CredentialID: &cred.ID, Model: "gpt-4o-mini",
		SystemPrompt: "You are a helpful test assistant.",
		Harness:      "hermes",
		EncryptedEnvVars: encEnvBytes,
		Resources: model.JSON{
			"github-conn-1": map[string]any{
				"repository": []any{
					map[string]any{"id": "octocat/hello", "name": "hello"},
				},
			},
		},
	}
	require.NoError(t, db.Create(&agent).Error)

	dek, err := crypto.GenerateDEK()
	require.NoError(t, err)
	wrappedDEK, err := kms.Wrap(context.Background(), dek)
	require.NoError(t, err)
	slackPlain, _ := json.Marshal(slackprov.Secrets{
		BotToken: "xoxb-real",
		AppToken: "xapp-real",
	})
	encSecrets, err := crypto.EncryptCredential(slackPlain, dek)
	require.NoError(t, err)
	profile := model.AgentProfile{
		ID: uuid.New(), OrgID: org.ID, AgentID: agent.ID,
		Provider: slackprov.Provider, Status: "active",
		EncryptedSecrets: encSecrets, WrappedDEK: wrappedDEK,
	}
	require.NoError(t, db.Create(&profile).Error)

	skill := model.Skill{
		ID: uuid.New(), Slug: "pr-review-" + uuid.New().String()[:6],
		Name: "PR Review", SourceType: model.SkillSourceInline,
		Status: model.SkillStatusPublished,
	}
	require.NoError(t, db.Create(&skill).Error)
	bundle := map[string]any{
		"id": skill.ID.String(), "title": "PR Review",
		"description": "Review pull requests",
		"content":     "# PR Review\n\nFollow these steps...\n",
		"files":       map[string]string{"reference/checklist.md": "- [ ] Tests"},
	}
	bundleBytes, _ := json.Marshal(bundle)
	version := model.SkillVersion{
		ID: uuid.New(), SkillID: skill.ID, Version: "v1",
		Bundle: bundleBytes,
	}
	require.NoError(t, db.Create(&version).Error)
	require.NoError(t, db.Model(&model.Skill{}).Where("id = ?", skill.ID).Update("latest_version_id", version.ID).Error)
	require.NoError(t, db.Create(&model.AgentSkill{AgentID: agent.ID, SkillID: skill.ID}).Error)

	return &agent, skill.ID
}

func decodeFile(t *testing.T, files []map[string]any, path string) []byte {
	t.Helper()
	for _, f := range files {
		if f["path"] == path {
			b, err := base64.StdEncoding.DecodeString(f["content_b64"].(string))
			require.NoError(t, err)
			return b
		}
	}
	t.Fatalf("file %s not in payload", path)
	return nil
}

func TestCompileAnthropicProviderDispatch(t *testing.T) {
	db := setupTestDB(t)
	encKey := testEncKey(t)
	kms := testKMS(t)

	org := model.Org{ID: uuid.New(), Name: "ant-org-" + uuid.New().String()[:8], Active: true}
	require.NoError(t, db.Create(&org).Error)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encryptedKey, err := encKey.EncryptString("sk-ant-test")
	require.NoError(t, err)
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID, ProviderID: "anthropic",
		Label: "Anthropic", EncryptedKey: encryptedKey, WrappedDEK: []byte("dek"),
		BaseURL: "https://api.anthropic.com", AuthScheme: "bearer",
	}
	require.NoError(t, db.Create(&cred).Error)

	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, Name: "AnthropicEmployee",
		IsEmployee:   true,
		CredentialID: &cred.ID, Model: "claude-3-5-sonnet-20241022",
		SystemPrompt: "You are anthropic-backed.", Harness: "hermes",
	}
	require.NoError(t, db.Create(&agent).Error)
	t.Cleanup(func() {
		db.Where("meta->>'agent_id' = ?", agent.ID.String()).Delete(&model.Token{})
	})

	req, err := hermes.Compile(context.Background(), hermes.CompileDeps{
		DB: db, Picker: credentials.NewPicker(db),
		KMS: kms, EncKey: encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        &config.Config{ProxyHost: "proxy.hiveloop.com"},
	}, &agent)
	require.NoError(t, err)

	raw, _ := json.Marshal(req.Files)
	var files []map[string]any
	require.NoError(t, json.Unmarshal(raw, &files))

	configRaw := decodeFile(t, files, "config.yaml")
	var cfgYAML map[string]any
	require.NoError(t, yamlv3.Unmarshal(configRaw, &cfgYAML))

	modelCfg := cfgYAML["model"].(map[string]any)
	require.Equal(t, "anthropic", modelCfg["provider"], "anthropic credential should drive provider=anthropic")
	require.Equal(t, "https://proxy.hiveloop.com", modelCfg["base_url"], "anthropic provider expects bare host (SDK appends /v1/messages)")

	customProviders, _ := cfgYAML["custom_providers"].([]any)
	require.Empty(t, customProviders, "no custom_providers entry for native anthropic provider")
}

func TestCompile(t *testing.T) {
	db := setupTestDB(t)
	encKey := testEncKey(t)
	kms := testKMS(t)
	agent, _ := seedAgentFixture(t, db, kms, encKey)

	cfg := &config.Config{
		ProxyHost:  "proxy.hiveloop.com",
		MCPBaseURL: "https://mcp.hiveloop.com",
	}
	deps := hermes.CompileDeps{
		DB: db, Picker: credentials.NewPicker(db),
		KMS: kms, EncKey: encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	t.Cleanup(func() {
		db.Where("agent_id = ?", agent.ID).Delete(&model.AgentSkill{})
		db.Where("meta->>'agent_id' = ?", agent.ID.String()).Delete(&model.Token{})
	})

	req, err := hermes.Compile(context.Background(), deps, agent)
	require.NoError(t, err)
	require.NotNil(t, req)
	require.Equal(t, agent.UpdatedAt.UnixNano(), req.Version)
	require.NotNil(t, req.FullSync)
	require.True(t, *req.FullSync)
	require.NotNil(t, req.Files)
	require.NotNil(t, req.Repos)

	raw, _ := json.Marshal(req.Files)
	var files []map[string]any
	require.NoError(t, json.Unmarshal(raw, &files))

	configRaw := decodeFile(t, files, "config.yaml")
	var cfgYAML map[string]any
	require.NoError(t, yamlv3.Unmarshal(configRaw, &cfgYAML))
	modelCfg := cfgYAML["model"].(map[string]any)
	require.Equal(t, "custom", modelCfg["provider"])
	require.Equal(t, "https://proxy.hiveloop.com/v1", modelCfg["base_url"])
	require.True(t, strings.HasPrefix(modelCfg["api_key"].(string), "ptok_"))
	memory := cfgYAML["memory"].(map[string]any)
	require.Equal(t, "hindsight", memory["provider"])
	hindsight := cfgYAML["hindsight"].(map[string]any)
	require.Equal(t, "bank-"+agent.OrgID.String(), hindsight["bank_id"])
	tools := cfgYAML["platform_toolsets"].(map[string]any)["api_server"].([]any)
	for _, name := range tools {
		require.NotEqual(t, "web", name)
	}
	mcpServers, ok := cfgYAML["mcp_servers"].([]any)
	require.True(t, ok)
	require.Len(t, mcpServers, 1)
	mcp := mcpServers[0].(map[string]any)
	require.Equal(t, "hiveloop", mcp["name"])

	envContent := decodeFile(t, files, ".env")
	envStr := string(envContent)
	require.Contains(t, envStr, "OPENAI_API_KEY=ptok_")
	require.Contains(t, envStr, "SLACK_BOT_TOKEN=xoxb-real\n")
	require.Contains(t, envStr, "SLACK_APP_TOKEN=xapp-real\n")
	require.Contains(t, envStr, "SLACK_ALLOWED_USERS=*\n")
	require.Contains(t, envStr, "FOO=bar\n")
	require.NotContains(t, envStr, "BRIDGE_SECRET")

	soul := decodeFile(t, files, "SOUL.md")
	require.Equal(t, "You are a helpful test assistant.", string(soul))

	hasSkillMD := false
	hasSkillRef := false
	for _, f := range files {
		path := f["path"].(string)
		if strings.HasPrefix(path, "skills/") && strings.HasSuffix(path, "/SKILL.md") {
			hasSkillMD = true
			body := decodeFile(t, files, path)
			require.Contains(t, string(body), "PR Review")
		}
		if strings.HasPrefix(path, "skills/") && strings.HasSuffix(path, "/reference/checklist.md") {
			hasSkillRef = true
		}
	}
	require.True(t, hasSkillMD, "expected SKILL.md to be present")
	require.True(t, hasSkillRef, "expected reference file to be present")
	require.Len(t, *req.Repos, 1)
	require.Equal(t, "hello", (*req.Repos)[0].Path)
	require.Equal(t, "https://github.com/octocat/hello.git", (*req.Repos)[0].Url)

	var tok model.Token
	require.NoError(t, db.Where("meta->>'agent_id' = ? AND meta->>'harness' = 'hermes'", agent.ID.String()).First(&tok).Error)
	require.Equal(t, *agent.OrgID, tok.OrgID)
}
