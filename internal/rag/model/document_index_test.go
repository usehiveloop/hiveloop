package model_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// bootstrapDocs opens a test DB with the full RAG schema migrated.
// testhelpers.ConnectTestDB calls rag.AutoMigrate internally, which
// creates every rag_* table, index, constraint, and FK this package
// needs.
func explainJSON(t *testing.T, db *gorm.DB, q string, args ...any) string {
	t.Helper()
	var plan string
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL enable_seqscan = off`).Error; err != nil {
			return fmt.Errorf("disable seqscan: %w", err)
		}
		rows, err := tx.Raw(`EXPLAIN (FORMAT JSON) `+q, args...).Rows()
		if err != nil {
			return fmt.Errorf("EXPLAIN: %w", err)
		}
		defer func() { _ = rows.Close() }()

		var builder strings.Builder
		for rows.Next() {
			var chunk []byte
			if err := rows.Scan(&chunk); err != nil {
				return fmt.Errorf("scan plan: %w", err)
			}
			builder.Write(chunk)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows.Err: %w", err)
		}
		plan = builder.String()
		return nil
	})
	if err != nil {
		t.Fatalf("explain tx: %v", err)
	}
	// Sanity-check the result is valid JSON.
	var parsed any
	if jerr := json.Unmarshal([]byte(plan), &parsed); jerr != nil {
		t.Logf("warn: plan was not valid JSON: %v (text=%q)", jerr, plan)
	}
	return plan
}

func planMentions(plan, needle string) bool {
	return strings.Contains(plan, needle)
}

