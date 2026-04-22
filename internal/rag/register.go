// Package rag is the root of the Onyx-derived RAG subsystem. Everything
// else in the Retrieval-Augmented Generation stack lives under
// internal/rag/*.
//
// This file owns the schema-migration entry point that
// internal/model/org.go:AutoMigrate calls immediately after the core
// Hiveloop schema migrates. Tranche 1F wires every Phase 1 tranche's
// AutoMigrate<N> helper in FK-safe order.
package rag

import (
	"gorm.io/gorm"

	ragembedder "github.com/usehiveloop/hiveloop/internal/rag/embedder"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// AutoMigrate runs the full RAG schema migration.
//
// Call order is load-bearing: 1G (rag_embedding_models) must run first
// because 1B and 1C FK-reference it. After 1G comes 1A (documents +
// hierarchy — no cross-tranche FKs), then 1B/1C/1D/1E. See the 1F
// section of plans/onyx-port.md.
//
// Idempotent: every AutoMigrate<N> guards its DDL with IF NOT EXISTS
// or an information_schema lookup. CI + boot run this on every deploy.
func AutoMigrate(db *gorm.DB) error {
	// 1G — rag_embedding_models catalog + registry seed. FK target
	// for 1B (rag_index_attempts.embedding_model_id) and 1C
	// (rag_search_settings.embedding_model_id), so it must land
	// before they run.
	if err := ragembedder.AutoMigrate1G(db); err != nil {
		return err
	}

	// 1A — core document + hierarchy + their junctions.
	if err := ragmodel.AutoMigrate1A(db); err != nil {
		return err
	}

	// 1B — index attempts + errors + sync records.
	if err := ragmodel.AutoMigrate1B(db); err != nil {
		return err
	}

	// 1C — sync state + connection config + search settings
	// (the 1G FK on rag_search_settings.embedding_model_id is
	// added opportunistically inside AutoMigrate1C now that 1G has
	// already run above).
	if err := ragmodel.AutoMigrate1C(db); err != nil {
		return err
	}

	// 1D — external user groups + ACL junctions.
	if err := ragmodel.AutoMigrate1D(db); err != nil {
		return err
	}

	// 1E — external identity + OAuthAccount column adds.
	if err := ragmodel.AutoMigrate1E(db); err != nil {
		return err
	}

	// 3A — pivot every RAG table to key off the new top-level
	// RAGSource. Drops deprecated `in_connection_id` columns + the
	// `rag_connection_configs` table, renames the doc/hierarchy
	// junction tables, and installs the `rag_source_id` FKs that
	// depended on rag_sources existing.
	if err := ragmodel.AutoMigrate3A(db); err != nil {
		return err
	}

	return nil
}
