package embedder

import (
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// Migrate creates the rag_embedding_models catalog table and upserts
// the Go-side registry into it.
//
// This helper lives in the embedder package (rather than the model
// package) because the seed reads RegistryEntry values that the
// embedder package defines; putting it alongside the other RAG model
// migrations would create an import cycle (model would need to import
// embedder to seed, while embedder already imports model to construct
// RAGEmbeddingModel rows).
//
// Safe to call multiple times: gorm.AutoMigrate is idempotent and the
// seed uses ON CONFLICT DO UPDATE.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&ragmodel.RAGEmbeddingModel{}); err != nil {
		return err
	}
	return SeedRegistry(db)
}
