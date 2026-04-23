package model_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
)

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
