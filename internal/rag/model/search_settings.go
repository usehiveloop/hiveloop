package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGSearchSettings adapts Onyx's `SearchSettings` at
// backend/onyx/db/models.py:2052-2187.
//
// DEVIATIONS:
//
//  1. Per-org, not global. OrgID is the primary key; one settings row
//     per org. Onyx supports multiple SearchSettings rows coexisting
//     via IndexModelStatus (PAST / PRESENT / FUTURE), which powers
//     their model-switchover workflow. Hiveloop's "one model per org
//     for the lifetime of their index" invariant means we don't need
//     the IndexModelStatus/SwitchoverType machinery. If an org wants a
//     new embedding model, ops deletes their chunks and re-ingests;
//     the settings row is mutated in place.
//
//  2. No IndexModelStatus column, no SwitchoverType column.
//     Consequence of (1).
//
//  3. FK EmbeddingModelID → rag_embedding_models(id). The catalog
//     table is created by embedder.Migrate before rag model.Migrate
//     installs this FK (see rag.AutoMigrate).
//
//  4. Hiveloop additions: RerankerModelID (per-org reranker choice —
//     Qwen3-Reranker-0.6B default), HybridAlpha (BM25/vector weight
//     for hybrid search, default 0.7), and the three ContextualRAG
//     fields reserved for a future port of Onyx models.py:2094-2101.
type RAGSearchSettings struct {
	// OrgID is the primary key — one settings row per org. FK CASCADE
	// so org deletion wipes the settings row along with everything
	// else.
	OrgID uuid.UUID `gorm:"type:uuid;primaryKey"`

	// EmbeddingModelID references rag_embedding_models.id. The FK
	// constraint is installed by model.Migrate after embedder.Migrate
	// has created the catalog table.
	EmbeddingModelID string `gorm:"type:varchar(128);not null;index"`

	// EmbeddingDim — port of Onyx `SearchSettings.model_dim` at
	// models.py:2057. Denormalized from the model catalog so downstream
	// queries don't need a join to know the vector width.
	EmbeddingDim int `gorm:"not null"`

	// Normalize — port of Onyx models.py:2058. Whether to L2-normalize
	// embedding outputs before storage.
	Normalize bool `gorm:"not null;default:true"`

	// QueryPrefix / PassagePrefix — port of Onyx models.py:2059-2060.
	// Some models (e.g. E5, Qwen3-Embedding) expect a text prefix to
	// disambiguate query-vs-document embeddings.
	QueryPrefix   *string `gorm:"type:text"`
	PassagePrefix *string `gorm:"type:text"`

	// EmbeddingPrecision — port of Onyx models.py:2079-2081.
	EmbeddingPrecision EmbeddingPrecision `gorm:"type:varchar(16);not null;default:'float'"`

	// ReducedDimension — port of Onyx models.py:2089. Optional
	// Matryoshka-style truncation (OpenAI models support this).
	ReducedDimension *int `gorm:"type:integer"`

	// MultipassIndexing — port of Onyx models.py:2092. Enables the
	// "mini + large chunks" strategy.
	MultipassIndexing bool `gorm:"not null;default:true"`

	// RerankerModelID — Hiveloop addition. Points at a reranker
	// registered in the same rag_embedding_models catalog (or a
	// separate reranker catalog in future). Nullable = no reranking.
	RerankerModelID *string `gorm:"type:varchar(128)"`

	// HybridAlpha — Hiveloop addition. Weight on the vector score in
	// hybrid search (0 = pure BM25, 1 = pure vector). Default 0.7 per
	// plan's "locked stack decisions" table.
	HybridAlpha float64 `gorm:"type:double precision;not null;default:0.7"`

	// IndexName — port of Onyx models.py:2065. Names the underlying
	// vector-store dataset (LanceDB dataset name). For Hiveloop this
	// is derived from `rag_embedding_models.DatasetName` at ingest
	// time; persisted here so ops can pin an org to a specific index
	// during ops work.
	IndexName string `gorm:"type:varchar(256);not null"`

	// EnableContextualRAG — port of Onyx models.py:2095. Reserved.
	EnableContextualRAG bool `gorm:"not null;default:false"`

	// ContextualRAGLLMName / ContextualRAGLLMProvider — port of Onyx
	// models.py:2098-2101. Reserved.
	ContextualRAGLLMName     *string `gorm:"type:varchar(128)"`
	ContextualRAGLLMProvider *string `gorm:"type:varchar(64)"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName — Hiveloop `rag_*` convention.
func (RAGSearchSettings) TableName() string { return "rag_search_settings" }
