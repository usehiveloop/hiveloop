package tasks

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func memoryEvent(t *testing.T, orgID, agentID, sandboxID uuid.UUID, sessionID, eventType string, payload map[string]any) model.EmployeeMemoryEvent {
	t.Helper()
	payload["session_id"] = sessionID
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return model.EmployeeMemoryEvent{
		ID:         uuid.New(),
		OrgID:      orgID,
		EmployeeID: agentID,
		SandboxID:  sandboxID,
		SessionID:  sessionID,
		EventType:  eventType,
		Source:     "slack",
		Payload:    model.RawJSON(raw),
		EventAt:    time.Now().UTC(),
	}
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
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable" // #nosec G101 -- local test fixture
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
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
