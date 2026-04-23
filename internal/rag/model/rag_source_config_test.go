package model_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	coremodel "github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/rag"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// isCheckViolation pins on Postgres error 23514 so we don't
// false-positive on unique / FK failures.
func TestRAGSource_PersistsConfigJSON(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	connID := conn.ID
	cfg := coremodel.JSON{
		"repo_allowlist":     []any{"hiveloop/api", "hiveloop/web"},
		"include_issues":     true,
		"include_prs":        false,
		"batch_size":         float64(250), // JSON numbers round-trip as float64
		"nested":             map[string]any{"depth": float64(2), "enabled": true},
	}

	src := &ragmodel.RAGSource{
		OrgIDValue:     org.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "config round-trip",
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: &connID,
		AccessType:     ragmodel.AccessTypePrivate,
		ConfigValue:    cfg,
	}
	if err := db.Create(src).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
	})

	var reloaded ragmodel.RAGSource
	if err := db.Where("id = ?", src.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Allowlist
	al, ok := reloaded.ConfigValue["repo_allowlist"].([]any)
	if !ok || len(al) != 2 || al[0] != "hiveloop/api" || al[1] != "hiveloop/web" {
		t.Fatalf("repo_allowlist round-trip failed: %#v", reloaded.ConfigValue["repo_allowlist"])
	}
	// Booleans
	if reloaded.ConfigValue["include_issues"] != true {
		t.Fatalf("include_issues round-trip: %#v", reloaded.ConfigValue["include_issues"])
	}
	if reloaded.ConfigValue["include_prs"] != false {
		t.Fatalf("include_prs round-trip: %#v", reloaded.ConfigValue["include_prs"])
	}
	// Number
	if reloaded.ConfigValue["batch_size"] != float64(250) {
		t.Fatalf("batch_size round-trip: %#v", reloaded.ConfigValue["batch_size"])
	}
	// Nested object
	nested, ok := reloaded.ConfigValue["nested"].(map[string]any)
	if !ok || nested["depth"] != float64(2) || nested["enabled"] != true {
		t.Fatalf("nested round-trip: %#v", reloaded.ConfigValue["nested"])
	}
}

// Business value: migrations must be idempotent. Running
// rag.AutoMigrate twice against a live DB must not error and must not
// multiply indexes.
func TestAutoMigrate_IsIdempotent(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	countIndexes := func() int64 {
		var n int64
		if err := db.Raw(`
			SELECT COUNT(*) FROM pg_indexes
			WHERE schemaname = 'public' AND tablename LIKE 'rag_%'
		`).Scan(&n).Error; err != nil {
			t.Fatalf("count indexes: %v", err)
		}
		return n
	}
	before := countIndexes()

	if err := rag.AutoMigrate(db); err != nil {
		t.Fatalf("second rag.AutoMigrate: %v", err)
	}
	after := countIndexes()
	if before != after {
		t.Fatalf("rag.AutoMigrate not idempotent: index count %d -> %d", before, after)
	}
}

