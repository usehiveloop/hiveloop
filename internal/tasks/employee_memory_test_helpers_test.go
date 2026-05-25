package tasks

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func memoryEvent(t *testing.T, orgID, agentID, sandboxID uuid.UUID, sessionID, eventType string, payload map[string]any) model.EmployeeSessionEvent {
	t.Helper()
	payload["session_id"] = sessionID
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return model.EmployeeSessionEvent{
		ID:                uuid.New(),
		OrgID:             orgID,
		EmployeeID:        agentID,
		SandboxID:         sandboxID,
		EmployeeSessionID: memorySessionID(orgID, agentID, sandboxID, sessionID),
		SessionID:         sessionID,
		EventType:         eventType,
		Source:            "slack",
		Payload:           model.RawJSON(raw),
		EventAt:           time.Now().UTC(),
	}
}

func createMemorySession(t *testing.T, db *gorm.DB, orgID, agentID, sandboxID uuid.UUID, sessionID string) uuid.UUID {
	t.Helper()
	id := memorySessionID(orgID, agentID, sandboxID, sessionID)
	session := model.EmployeeSession{
		ID:                    id,
		OrgID:                 orgID,
		EmployeeID:            agentID,
		SandboxID:             sandboxID,
		RuntimeConversationID: sessionID,
		Source:                "slack",
		SourceResourceKey:     sessionID,
		Status:                "active",
		IntegrationScopes:     model.JSON{},
	}
	if err := db.Where("id = ?", id).FirstOrCreate(&session).Error; err != nil {
		t.Fatalf("create memory session: %v", err)
	}
	return id
}

func memorySessionID(orgID, agentID, sandboxID uuid.UUID, sessionID string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(orgID.String()+":"+agentID.String()+":"+sandboxID.String()+":"+sessionID))
}

func hasTaskString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func openTasksMemoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	return db
}

func testTasksEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 31)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("new symmetric key: %v", err)
	}
	return sk
}
