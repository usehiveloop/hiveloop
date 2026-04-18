package subscriptions_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/subscriptions"
)

// The service test needs a real Postgres. When DATABASE_URL isn't set and the
// default test DB isn't reachable, we skip — same policy as other DB-dependent
// tests in this repo (e.g. internal/cache).

const testDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

func connectOrSkip(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Postgres not available, skipping: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("Postgres not reachable, skipping: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	return db
}

// seedAgentWithGitHub creates an org, user, in_integration (github-app),
// in_connection, agent pointing at that connection, and an active
// conversation. Returns the IDs the test needs.
func seedAgentWithGitHub(t *testing.T, db *gorm.DB) (orgID, agentID, convID uuid.UUID, cleanup func()) {
	t.Helper()

	orgID = uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "sub-test-" + orgID.String()[:8]}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	userID := uuid.New()
	if err := db.Create(&model.User{
		ID:    userID,
		Email: "subtest-" + userID.String()[:8] + "@example.com",
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	integrationID := uuid.New()
	if err := db.Create(&model.InIntegration{
		ID:          integrationID,
		Provider:    "github-app",
		UniqueKey:   "github-app-test-" + integrationID.String()[:8],
		DisplayName: "GitHub App",
	}).Error; err != nil {
		t.Fatalf("create in_integration: %v", err)
	}

	connectionID := uuid.New()
	if err := db.Create(&model.InConnection{
		ID:                connectionID,
		OrgID:             orgID,
		UserID:            userID,
		InIntegrationID:   integrationID,
		NangoConnectionID: "nango-" + connectionID.String()[:8],
	}).Error; err != nil {
		t.Fatalf("create in_connection: %v", err)
	}

	agentID = uuid.New()
	if err := db.Create(&model.Agent{
		ID:           agentID,
		OrgID:        &orgID,
		Name:         "test-agent-" + agentID.String()[:8],
		Model:        "anthropic/claude-haiku-4-5",
		SandboxType:  "shared",
		SystemPrompt: "test",
		Integrations: model.JSON{
			connectionID.String(): map[string]any{
				"actions": []any{"issues_create"},
			},
		},
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	sandboxID := uuid.New()
	if err := db.Create(&model.Sandbox{
		ID:                    sandboxID,
		OrgID:                 &orgID,
		SandboxType:           "shared",
		ExternalID:            "sb-ext-" + sandboxID.String()[:8],
		BridgeURL:             "https://test.invalid",
		EncryptedBridgeAPIKey: []byte("fake"),
		Status:                "running",
	}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	convID = uuid.New()
	if err := db.Create(&model.AgentConversation{
		ID:                   convID,
		OrgID:                orgID,
		AgentID:              agentID,
		SandboxID:            sandboxID,
		BridgeConversationID: "bridge-conv-" + convID.String()[:8],
		Status:               "active",
	}).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	cleanup = func() {
		db.Where("conversation_id = ?", convID).Delete(&model.ConversationSubscription{})
		db.Where("id = ?", convID).Delete(&model.AgentConversation{})
		db.Where("id = ?", sandboxID).Delete(&model.Sandbox{})
		db.Where("id = ?", agentID).Delete(&model.Agent{})
		db.Where("id = ?", connectionID).Delete(&model.InConnection{})
		db.Where("id = ?", integrationID).Delete(&model.InIntegration{})
		db.Where("id = ?", userID).Delete(&model.User{})
		db.Where("id = ?", orgID).Delete(&model.Org{})
	}
	return orgID, agentID, convID, cleanup
}

func TestSubscribe_HappyPath(t *testing.T) {
	db := connectOrSkip(t)
	orgID, agentID, convID, cleanup := seedAgentWithGitHub(t, db)
	t.Cleanup(cleanup)

	svc := subscriptions.NewService(db, catalog.Global())
	result, err := svc.Subscribe(context.Background(), subscriptions.SubscribeRequest{
		OrgID:          orgID,
		AgentID:        agentID,
		ConversationID: convID,
		ResourceType:   "github_pull_request",
		ResourceID:     "ziraloop/ziraloop#99",
	})
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if result.ResourceKey != "github/ziraloop/ziraloop/pull/99" {
		t.Errorf("canonical = %q, want github/ziraloop/ziraloop/pull/99", result.ResourceKey)
	}
	if result.Provider != "github-app" {
		t.Errorf("provider = %q, want github-app", result.Provider)
	}
	if result.Idempotent {
		t.Error("first call should not be idempotent")
	}
	if len(result.Events) == 0 {
		t.Error("Events list should be populated from catalog")
	}
}

func TestSubscribe_Idempotent(t *testing.T) {
	db := connectOrSkip(t)
	orgID, agentID, convID, cleanup := seedAgentWithGitHub(t, db)
	t.Cleanup(cleanup)

	svc := subscriptions.NewService(db, catalog.Global())
	req := subscriptions.SubscribeRequest{
		OrgID:          orgID,
		AgentID:        agentID,
		ConversationID: convID,
		ResourceType:   "github_issue",
		ResourceID:     "ziraloop/ziraloop#42",
	}

	first, err := svc.Subscribe(context.Background(), req)
	if err != nil {
		t.Fatalf("first subscribe failed: %v", err)
	}
	second, err := svc.Subscribe(context.Background(), req)
	if err != nil {
		t.Fatalf("second subscribe failed: %v", err)
	}
	if first.SubscriptionID != second.SubscriptionID {
		t.Errorf("subscription IDs differ across idempotent calls: %s vs %s", first.SubscriptionID, second.SubscriptionID)
	}
	if !second.Idempotent {
		t.Error("second call should be marked idempotent")
	}
}

func TestSubscribe_UnknownResourceType(t *testing.T) {
	db := connectOrSkip(t)
	orgID, agentID, convID, cleanup := seedAgentWithGitHub(t, db)
	t.Cleanup(cleanup)

	svc := subscriptions.NewService(db, catalog.Global())
	_, err := svc.Subscribe(context.Background(), subscriptions.SubscribeRequest{
		OrgID:          orgID,
		AgentID:        agentID,
		ConversationID: convID,
		ResourceType:   "github_banana",
		ResourceID:     "ziraloop/ziraloop#1",
	})
	if !errors.Is(err, subscriptions.ErrUnknownResourceType) {
		t.Fatalf("want ErrUnknownResourceType, got %v", err)
	}
}

func TestSubscribe_MissingIntegration(t *testing.T) {
	db := connectOrSkip(t)
	orgID, agentID, convID, cleanup := seedAgentWithGitHub(t, db)
	t.Cleanup(cleanup)

	// Remove the github-app integration from the agent by overwriting with empty.
	if err := db.Model(&model.Agent{}).Where("id = ?", agentID).Update("integrations", model.JSON{}).Error; err != nil {
		t.Fatalf("clearing agent integrations: %v", err)
	}

	svc := subscriptions.NewService(db, catalog.Global())
	_, err := svc.Subscribe(context.Background(), subscriptions.SubscribeRequest{
		OrgID:          orgID,
		AgentID:        agentID,
		ConversationID: convID,
		ResourceType:   "github_pull_request",
		ResourceID:     "ziraloop/ziraloop#99",
	})
	if !errors.Is(err, subscriptions.ErrIntegrationMissing) {
		t.Fatalf("want ErrIntegrationMissing, got %v", err)
	}
}

func TestSubscribe_InvalidResourceID(t *testing.T) {
	db := connectOrSkip(t)
	orgID, agentID, convID, cleanup := seedAgentWithGitHub(t, db)
	t.Cleanup(cleanup)

	svc := subscriptions.NewService(db, catalog.Global())
	_, err := svc.Subscribe(context.Background(), subscriptions.SubscribeRequest{
		OrgID:          orgID,
		AgentID:        agentID,
		ConversationID: convID,
		ResourceType:   "github_pull_request",
		ResourceID:     "not-a-valid-id",
	})
	if !errors.Is(err, subscriptions.ErrInvalidResourceID) {
		t.Fatalf("want ErrInvalidResourceID, got %v", err)
	}
}

func TestListActive(t *testing.T) {
	db := connectOrSkip(t)
	orgID, agentID, convID, cleanup := seedAgentWithGitHub(t, db)
	t.Cleanup(cleanup)

	svc := subscriptions.NewService(db, catalog.Global())
	ctx := context.Background()

	_, _ = svc.Subscribe(ctx, subscriptions.SubscribeRequest{
		OrgID: orgID, AgentID: agentID, ConversationID: convID,
		ResourceType: "github_pull_request", ResourceID: "ziraloop/ziraloop#99",
	})
	_, _ = svc.Subscribe(ctx, subscriptions.SubscribeRequest{
		OrgID: orgID, AgentID: agentID, ConversationID: convID,
		ResourceType: "github_issue", ResourceID: "ziraloop/ziraloop#42",
	})

	active, err := svc.ListActive(ctx, convID)
	if err != nil {
		t.Fatalf("ListActive error: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("active count = %d, want 2", len(active))
	}
}
