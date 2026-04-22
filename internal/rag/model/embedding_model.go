package model

import "time"

// RAGEmbeddingModel is the catalog of embedding models supported by
// Hiveloop's RAG pipeline. Each row is an admin-visible entry that
// RAGSearchSettings.EmbeddingModelID and RAGIndexAttempt.EmbeddingModelID
// FK into.
//
// Ports the `model_name`, `model_dim`, and `provider_type` columns of
// Onyx's `SearchSettings` at backend/onyx/db/models.py:2056-2069,
// together with the provider-level metadata from Onyx's
// `CloudEmbeddingProvider` at backend/onyx/db/models.py:3178-3197.
//
// DEVIATION: Onyx couples model identity into the SearchSettings row
// (per-tenant config) and the CloudEmbeddingProvider row (which also
// carries API keys). We split the catalog into a standalone read-only
// table so the admin UI can enumerate available models without
// compile-time coupling or exposing provider credentials. The Go-side
// registry at internal/rag/embedder/registry.go is the source of truth;
// SeedRegistry writes/updates rows on every boot.
type RAGEmbeddingModel struct {
	// ID is a provider-namespaced opaque identifier,
	// e.g. "siliconflow:qwen3-embedding-4b". Stable across deploys
	// because FKs from RAGSearchSettings and RAGIndexAttempt depend on it.
	ID string `gorm:"primaryKey;type:text"`

	// Provider is the embedding backend, e.g. "siliconflow" or "openai".
	// Mirrors Onyx SearchSettings.provider_type at models.py:2066.
	Provider string `gorm:"type:text;not null"`

	// ModelName is the provider-native model identifier,
	// e.g. "Qwen/Qwen3-Embedding-4B". Mirrors Onyx SearchSettings.model_name
	// at models.py:2056.
	ModelName string `gorm:"type:text;not null"`

	// Dimension is the embedding vector length. Mirrors Onyx
	// SearchSettings.model_dim at models.py:2057.
	Dimension int `gorm:"not null"`

	// MaxInputTokens is the provider's documented token ceiling per
	// embed call. Used by the chunker to size chunks safely.
	MaxInputTokens int `gorm:"not null"`

	// DatasetName is the LanceDB dataset name derived deterministically
	// from (Provider, ModelName, Dimension). Derivation lives in
	// embedder.deriveDatasetName; the column persists the derived value
	// so operators can read it directly.
	DatasetName string `gorm:"type:text;not null"`

	// QueryPrefix is the instruction token prepended to query embeds
	// (e.g. "query: " for Qwen3 E5-style models). Nil for providers
	// that do not require prefixes. Mirrors Onyx SearchSettings.query_prefix
	// at models.py:2059.
	QueryPrefix *string `gorm:"type:text"`

	// PassagePrefix is the instruction token prepended to passage embeds.
	// Mirrors Onyx SearchSettings.passage_prefix at models.py:2060.
	PassagePrefix *string `gorm:"type:text"`

	// PricingPer1MTokensUSD is the provider's published price for 1M
	// input tokens, in USD. Surfaced in the admin UI so operators can
	// pick a tier. Not a hard billing source — just a display hint.
	//
	// Column name pinned explicitly because gorm's default namer
	// produces "pricing_per1_m_tokens_usd", which is fine but brittle —
	// pin the column so a future namer change cannot cause a silent rename.
	PricingPer1MTokensUSD float64 `gorm:"column:pricing_per_1m_tokens_usd;not null"`

	// IsActive gates admin-UI visibility. Deprecated rows stay in the
	// table so historical RAGIndexAttempt FKs resolve, but do not show
	// up as selectable options for new configurations.
	IsActive bool `gorm:"not null;default:true"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (RAGEmbeddingModel) TableName() string { return "rag_embedding_models" }
