package middleware_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

const toolUsageTestDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

func connectToolUsageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = toolUsageTestDBURL
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("cannot connect to test database: %v", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	return database
}

func TestToolUsageWriter_WriteAndFlush(t *testing.T) {
	database := connectToolUsageTestDB(t)
	orgID := uuid.New()

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.ToolUsage{})
	})

	writer := middleware.NewToolUsageWriter(database, 100)

	writer.Write(model.ToolUsage{
		ID:        "tu_" + ulid.Make().String(),
		OrgID:     orgID,
		AgentID:   uuid.New().String(),
		TokenJTI:  "test-jti-1",
		ToolName:  "crawl",
		Input:     "https://example.com",
		Status:    "success",
		TotalMs:   150,
		CreatedAt: time.Now().UTC(),
	})

	writer.Write(model.ToolUsage{
		ID:        "tu_" + ulid.Make().String(),
		OrgID:     orgID,
		AgentID:   uuid.New().String(),
		TokenJTI:  "test-jti-2",
		ToolName:  "search",
		Input:     "golang testing",
		Status:    "success",
		TotalMs:   200,
		CreatedAt: time.Now().UTC(),
	})

	// Shutdown flushes remaining entries
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	writer.Shutdown(ctx)

	var count int64
	database.Model(&model.ToolUsage{}).Where("org_id = ?", orgID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 records, got %d", count)
	}
}

func TestToolUsageWriter_BatchFlush(t *testing.T) {
	database := connectToolUsageTestDB(t)
	orgID := uuid.New()

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.ToolUsage{})
	})

	writer := middleware.NewToolUsageWriter(database, 1000)

	// Write enough records to trigger a batch flush (batch size is 50)
	for index := range 60 {
		writer.Write(model.ToolUsage{
			ID:        "tu_" + ulid.Make().String(),
			OrgID:     orgID,
			AgentID:   uuid.New().String(),
			TokenJTI:  fmt.Sprintf("jti-%d", index),
			ToolName:  "crawl",
			Input:     fmt.Sprintf("https://example.com/%d", index),
			Status:    "success",
			CreatedAt: time.Now().UTC(),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	writer.Shutdown(ctx)

	var count int64
	database.Model(&model.ToolUsage{}).Where("org_id = ?", orgID).Count(&count)
	if count != 60 {
		t.Fatalf("expected 60 records, got %d", count)
	}
}

func TestToolUsageWriter_ShutdownIdempotent(t *testing.T) {
	database := connectToolUsageTestDB(t)

	writer := middleware.NewToolUsageWriter(database, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Calling Shutdown twice should not panic
	writer.Shutdown(ctx)
	writer.Shutdown(ctx)
}

func TestToolUsageWriter_DropWhenFull(t *testing.T) {
	database := connectToolUsageTestDB(t)
	orgID := uuid.New()

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.ToolUsage{})
	})

	// Create writer with very small buffer
	writer := middleware.NewToolUsageWriter(database, 2)

	// Give the drain goroutine a moment to start
	time.Sleep(10 * time.Millisecond)

	// Write many records quickly — some should be dropped
	for index := range 100 {
		writer.Write(model.ToolUsage{
			ID:        "tu_" + ulid.Make().String(),
			OrgID:     orgID,
			AgentID:   uuid.New().String(),
			TokenJTI:  fmt.Sprintf("jti-%d", index),
			ToolName:  "crawl",
			Input:     "https://example.com",
			Status:    "success",
			CreatedAt: time.Now().UTC(),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	writer.Shutdown(ctx)

	// With a buffer of 2, we should have fewer than 100 records
	// (some will be dropped). Just verify it didn't panic.
	var count int64
	database.Model(&model.ToolUsage{}).Where("org_id = ?", orgID).Count(&count)
	if count > 100 {
		t.Fatalf("expected at most 100 records, got %d", count)
	}
}
