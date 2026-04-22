// Package rag is the root of the Onyx-derived RAG subsystem. Everything
// else in the Retrieval-Augmented Generation stack lives under
// internal/rag/*.
//
// This file owns the schema-migration entry point that
// internal/model/org.go:AutoMigrate calls immediately after the core
// Hiveloop schema migrates. Phase 1 tranches populate AutoMigrate with
// RAG-specific gorm.AutoMigrate calls and raw-SQL partial/GIN index
// statements. Phase 0 keeps it empty so the harness end-to-end pipeline
// (testhelpers.ConnectTestDB → model.AutoMigrate → rag.AutoMigrate) is
// wired and verified.
package rag

import "gorm.io/gorm"

// AutoMigrate runs RAG schema migrations. Phase 1 fills this in. Called
// from internal/model/org.go:AutoMigrate after the core schema migrates.
//
// Must remain idempotent: CI + boot run it on every deploy.
func AutoMigrate(db *gorm.DB) error {
	_ = db
	// Phase 1 tranches append model registrations and raw-SQL index
	// statements here. Keep calls grouped by tranche for review hygiene.
	return nil
}
