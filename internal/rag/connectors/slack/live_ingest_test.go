package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	qdrantgo "github.com/qdrant/go-client/qdrant"
	slacksdk "github.com/slack-go/slack"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprofile "github.com/usehiveloop/hiveloop/internal/profiles/slack"
	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

func TestSlackBotProfileRAGIngestion_Live(t *testing.T) {
	if os.Getenv("HIVELOOP_E2E_SLACK_RAG") != "1" {
		t.Skip("set HIVELOOP_E2E_SLACK_RAG=1 to run the real Slack/Qdrant ingestion test")
	}
	botToken := requiredEnv(t, "HIVELOOP_E2E_SLACK_BOT_TOKEN")
	llmURL := requiredEnv(t, "LLM_API_URL")
	llmKey := requiredEnv(t, "LLM_API_KEY")
	llmModel := requiredEnv(t, "LLM_MODEL")
	qdrantHost := requiredEnv(t, "QDRANT_HOST")
	dim := uint32(requiredEnvInt(t, "LLM_EMBEDDING_DIM", 3072))
	collection := fmt.Sprintf("slack_rag_e2e_%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db := liveSlackRAGDB(t)
	kms := liveSlackRAGKMS(t)
	qd := liveSlackRAGQdrant(t, qdrantHost)
	t.Cleanup(func() { _ = qd.Close() })
	if err := qd.EnsureCollection(ctx, qdrant.CollectionConfig{Name: collection, VectorDim: dim, OnDisk: false}); err != nil {
		t.Fatalf("ensure qdrant collection: %v", err)
	}
	if os.Getenv("HIVELOOP_E2E_KEEP_SLACK_RAG") != "1" {
		t.Cleanup(func() { _ = qd.DeleteCollection(context.Background(), collection) })
	}

	identity, err := readonlySlackBotIdentity(botToken)
	if err != nil {
		t.Fatalf("verify slack bot token: %v", err)
	}

	org := model.Org{ID: uuid.New(), Name: "slack-rag-e2e-" + uuid.NewString()[:8]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &org.ID,
		Name:            "Slack RAG E2E " + uuid.NewString()[:8],
		SystemPrompt:    "test",
		Model:           "test",
		IsEmployee:      true,
		Tools:           model.JSON{},
		McpServers:      model.JSON{},
		Skills:          model.JSON{},
		Integrations:    model.JSON{},
		AgentConfig:     model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
		ProviderPrompts: model.ProviderPromptsMap{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := liveSlackRAGProfile(t, db, kms, org.ID, agent.ID, identity, slackprofile.Secrets{BotToken: botToken})
	historyDays := 90
	indexingStart := time.Now().UTC().AddDate(0, 0, -historyDays)
	refresh := 600
	src := ragmodel.RAGSource{
		ID:                 uuid.New(),
		OrgIDValue:         org.ID,
		KindValue:          ragmodel.RAGSourceKindSlackBotProfile,
		Name:               "Slack history e2e",
		Status:             ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:            true,
		AccessType:         ragmodel.AccessTypePublic,
		IndexingStart:      &indexingStart,
		RefreshFreqSeconds: &refresh,
		ConfigValue: model.JSON{
			"agent_profile_id":                profile.ID.String(),
			"agent_id":                        agent.ID.String(),
			"history_days":                    historyDays,
			"include_public_channels":         true,
			"include_joined_private_channels": true,
		},
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}
	if os.Getenv("HIVELOOP_E2E_KEEP_SLACK_RAG") != "1" {
		t.Cleanup(func() {
			db.Where("rag_source_id = ?", src.ID).Delete(&ragmodel.RAGIndexAttemptError{})
			db.Where("rag_source_id = ?", src.ID).Delete(&ragmodel.RAGIndexAttempt{})
			db.Delete(&ragmodel.RAGSource{}, "id = ?", src.ID)
			db.Delete(&model.AgentProfile{}, "id = ?", profile.ID)
			db.Delete(&model.Agent{}, "id = ?", agent.ID)
			db.Delete(&model.Org{}, "id = ?", org.ID)
		})
	}

	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: src.ID, FromBeginning: true})
	if err != nil {
		t.Fatalf("build ingest task: %v", err)
	}
	deps := &ragtasks.Deps{
		DB:         db,
		Qdrant:     qd,
		Embedder:   embedclient.NewEmbedder(embedclient.EmbedderConfig{BaseURL: llmURL, APIKey: llmKey, Model: llmModel, Dim: dim, Timeout: 120 * time.Second}),
		KMS:        kms,
		Collection: collection,
		BatchSize:  25,
	}
	if err := deps.HandleIngest(ctx, task); err != nil {
		t.Fatalf("handle ingest: %v", err)
	}

	points := scrollSlackRAGPoints(t, ctx, qd, collection, org.ID.String(), src.ID.String())
	if len(points) == 0 {
		t.Fatalf("expected Slack ingestion to write at least one qdrant point")
	}
	t.Logf("Slack RAG e2e wrote %d qdrant point(s) into collection=%s source_id=%s org_id=%s", len(points), collection, src.ID, org.ID)
	dump := compactPointDump(points, 20)
	t.Logf("Ingested Slack payload sample:\n%s", dump)

	for _, point := range points {
		docID, _ := point.Payload["doc_id"].(string)
		if !strings.HasPrefix(docID, "slack:") {
			t.Fatalf("unexpected doc_id %q payload=%#v", docID, point.Payload)
		}
		source, _ := point.Payload["source"].(map[string]any)
		if source["provider"] != "slack" {
			t.Fatalf("expected source.provider=slack, got %#v", source)
		}
	}
}

func requiredEnv(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s not set", key)
	}
	return value
}

func readonlySlackBotIdentity(botToken string) (slackprofile.Identity, error) {
	resp, err := slacksdk.New(botToken).AuthTest()
	if err != nil {
		return slackprofile.Identity{}, err
	}
	return slackprofile.Identity{
		TeamID:      resp.TeamID,
		TeamName:    resp.Team,
		TeamURL:     resp.URL,
		BotUserID:   resp.UserID,
		BotUsername: resp.User,
		BotID:       resp.BotID,
	}, nil
}

func requiredEnvInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%s must be an integer: %v", key, err)
	}
	return value
}

func liveSlackRAGDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("core automigrate: %v", err)
	}
	if err := db.AutoMigrate(&ragmodel.RAGSource{}, &ragmodel.RAGIndexAttempt{}, &ragmodel.RAGIndexAttemptError{}); err != nil {
		t.Fatalf("rag automigrate: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func liveSlackRAGKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	kms, err := crypto.NewAEADWrapper(context.Background(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "slack-rag-e2e")
	if err != nil {
		t.Fatalf("test kms: %v", err)
	}
	return kms
}

func liveSlackRAGQdrant(t *testing.T, host string) *qdrant.Client {
	t.Helper()
	port := requiredEnvInt(t, "QDRANT_PORT", 6334)
	useTLS, _ := strconv.ParseBool(os.Getenv("QDRANT_USE_TLS"))
	client, err := qdrant.New(qdrant.Config{
		Host:                   host,
		Port:                   port,
		UseTLS:                 useTLS,
		APIKey:                 os.Getenv("QDRANT_API_KEY"),
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		t.Fatalf("connect qdrant: %v", err)
	}
	return client
}

func liveSlackRAGProfile(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, orgID, agentID uuid.UUID, identity slackprofile.Identity, secrets slackprofile.Secrets) model.AgentProfile {
	t.Helper()
	plaintext, err := slackprofile.EncodeSecrets(secrets)
	if err != nil {
		t.Fatalf("encode slack secrets: %v", err)
	}
	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("generate dek: %v", err)
	}
	encrypted, err := crypto.EncryptCredential(plaintext, dek)
	if err != nil {
		t.Fatalf("encrypt slack secrets: %v", err)
	}
	wrapped, err := kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap dek: %v", err)
	}
	identityJSON := model.JSON{}
	rawIdentity, _ := json.Marshal(identity)
	_ = json.Unmarshal(rawIdentity, &identityJSON)
	profile := model.AgentProfile{
		ID:               uuid.New(),
		OrgID:            orgID,
		AgentID:          agentID,
		Provider:         slackprofile.Provider,
		ExternalID:       identity.TeamID,
		Label:            identity.TeamName,
		Identity:         identityJSON,
		Config:           model.JSON{},
		EncryptedSecrets: encrypted,
		WrappedDEK:       wrapped,
		Status:           "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return profile
}

func scrollSlackRAGPoints(t *testing.T, ctx context.Context, qd *qdrant.Client, collection, orgID, sourceID string) []qdrant.ScrolledPoint {
	t.Helper()
	var points []qdrant.ScrolledPoint
	var offset *qdrantgo.PointId
	for {
		page, err := qd.Scroll(ctx, qdrant.ScrollRequest{
			Collection:  collection,
			Filter:      qdrant.BuildSourceFilter(orgID, sourceID),
			Limit:       100,
			Offset:      offset,
			WithPayload: true,
		})
		if err != nil {
			t.Fatalf("scroll qdrant: %v", err)
		}
		points = append(points, page.Points...)
		if page.NextOffset == nil {
			return points
		}
		offset = page.NextOffset
	}
}

func compactPointDump(points []qdrant.ScrolledPoint, limit int) string {
	if len(points) < limit {
		limit = len(points)
	}
	rows := make([]map[string]any, 0, limit)
	for i := 0; i < limit; i++ {
		payload := points[i].Payload
		content, _ := payload["content"].(string)
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		rows = append(rows, map[string]any{
			"id":          points[i].ID,
			"doc_id":      payload["doc_id"],
			"semantic_id": payload["semantic_id"],
			"link":        payload["link"],
			"source":      payload["source"],
			"metadata":    payload["metadata"],
			"content":     content,
		})
	}
	raw, _ := json.MarshalIndent(rows, "", "  ")
	return string(raw)
}
