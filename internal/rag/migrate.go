// Package rag is the root of Hiveloop's Retrieval-Augmented Generation
// stack. Postgres schema lives under internal/rag/model; vector
// storage + search is a separate Rust service reached over gRPC.
package rag

import (
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag/embedder"
	"github.com/usehiveloop/hiveloop/internal/rag/model"
)

// AutoMigrate creates and reconciles every table, index, constraint, and
// foreign key owned by the RAG subsystem. It is called from the main
// Hiveloop AutoMigrate pipeline and from test setup. Idempotent.
//
// Ordering matters where FKs cross packages — the embedding_models
// catalog must exist before tables that FK-reference it.
func AutoMigrate(db *gorm.DB) error {
	if err := embedder.Migrate(db); err != nil {
		return err
	}
	if err := model.Migrate(db); err != nil {
		return err
	}
	return nil
}
