package employeeruntime

import (
	"context"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

const compileTestDBURL = "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable" // #nosec G101 -- test fixture, not a real secret

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func connectCompileTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = compileTestDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() {
		sqlDB.Close()
	})
	return db
}

type fakeMemoryRecall struct {
	bankID   string
	request  *hindsight.RecallRequest
	response *hindsight.RecallResponse
	err      error
}

func (f *fakeMemoryRecall) Recall(_ context.Context, bankID string, req *hindsight.RecallRequest) (*hindsight.RecallResponse, error) {
	f.bankID = bankID
	f.request = req
	if f.err != nil {
		return nil, f.err
	}
	if f.response == nil {
		return &hindsight.RecallResponse{}, nil
	}
	return f.response, nil
}
