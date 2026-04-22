package embedder

import (
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// AutoMigrate1G migrates the embedding-model catalog table and seeds it
// from the Go-side registry. Called by Tranche 1F's finalizer in
// internal/rag/register.go:AutoMigrate.
//
// DEVIATION FROM 1G SPEC: the spec placed this helper at
// internal/rag/model/automigrate_1g.go. That path creates an import
// cycle because seed.go under internal/rag/embedder/ already imports
// internal/rag/model to construct RAGEmbeddingModel rows, and a model
// package that imports embedder to call SeedRegistry closes the loop.
// We moved the helper to the embedder package (the dependent side of
// the import arrow) so the call graph stays acyclic: model defines the
// row type, embedder owns the seed + the migrate-then-seed combo,
// register.go (1F) calls embedder.AutoMigrate1G. Semantically identical
// to the spec — the file sits in a different directory but does exactly
// what 1F needs.
//
// CRITICAL ORDERING: this must run before any tranche that FK-references
// rag_embedding_models — currently RAGSearchSettings.EmbeddingModelID
// (1C) and RAGIndexAttempt.EmbeddingModelID (1B). 1F's finalizer is
// responsible for calling AutoMigrate1G before migrating the FK-bearing
// tables; AutoMigrate1G does not chain to them.
//
// Safe to call multiple times: gorm.AutoMigrate is idempotent for the
// gorm-managed schema, and SeedRegistry upserts rather than inserts.
func AutoMigrate1G(db *gorm.DB) error {
	if err := db.AutoMigrate(&ragmodel.RAGEmbeddingModel{}); err != nil {
		return err
	}
	return SeedRegistry(db)
}
