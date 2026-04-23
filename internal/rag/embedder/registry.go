package embedder

import (
	"strconv"
	"strings"
)

// The `Provider` column on each entry is a human-readable label used
// for surfacing in the admin UI and for deriving a stable LanceDB
// dataset name — it is NOT consulted at runtime to pick which embedder
// implementation to invoke. Runtime selection is driven entirely by
// the Rust rag-engine's `LLM_API_URL` / `LLM_API_KEY` / `LLM_MODEL`
// env vars. Any provider speaking the OpenAI `/v1/embeddings` surface —
// SiliconFlow, OpenRouter, Groq, OpenAI, Together — works with the
// same Rust code path; the label here is for display and dataset
// namespacing only.

// RegistryEntry is a single catalog row, in Go-side form. It mirrors the
// persistent columns of model.RAGEmbeddingModel excluding auto-managed
// timestamps. SeedRegistry translates these into DB rows and upserts
// them on every migration.
//
// The registry is the source of truth: editing an entry here and
// redeploying rewrites the corresponding DB row via upsert.
type RegistryEntry struct {
	ID                    string
	Provider              string
	ModelName             string
	Dimension             int
	MaxInputTokens        int
	DatasetName           string
	QueryPrefix           *string
	PassagePrefix         *string
	PricingPer1MTokensUSD float64
	IsActive              bool
}

// strPtr is a small helper to build *string literals for prefix fields.
func strPtr(s string) *string { return &s }

// Registry returns the full set of embedding models Hiveloop ships with.
// The default for new orgs is `siliconflow:qwen3-embedding-4b`
// (Qwen3-Embedding-4B) per the plan's "Locked stack decisions".
//
// Adding, removing, or mutating an entry and redeploying is the sole
// supported change workflow. Non-destructive: existing DB rows are
// upserted, never deleted by SeedRegistry — deprecated entries should
// flip IsActive=false so historical FKs still resolve.
func Registry() []RegistryEntry {
	qwenQueryPrefix := strPtr("query: ")
	qwenPassagePrefix := strPtr("passage: ")

	return []RegistryEntry{
		{
			ID:                    "siliconflow:qwen3-embedding-4b",
			Provider:              "siliconflow",
			ModelName:             "Qwen/Qwen3-Embedding-4B",
			Dimension:             2560,
			MaxInputTokens:        8192,
			DatasetName:           deriveDatasetName("siliconflow", "Qwen/Qwen3-Embedding-4B", 2560),
			QueryPrefix:           qwenQueryPrefix,
			PassagePrefix:         qwenPassagePrefix,
			PricingPer1MTokensUSD: 0.02,
			IsActive:              true,
		},
		{
			ID:                    "siliconflow:qwen3-embedding-0.6b",
			Provider:              "siliconflow",
			ModelName:             "Qwen/Qwen3-Embedding-0.6B",
			Dimension:             1024,
			MaxInputTokens:        8192,
			DatasetName:           deriveDatasetName("siliconflow", "Qwen/Qwen3-Embedding-0.6B", 1024),
			QueryPrefix:           qwenQueryPrefix,
			PassagePrefix:         qwenPassagePrefix,
			PricingPer1MTokensUSD: 0.007,
			IsActive:              true,
		},
		{
			ID:                    "siliconflow:qwen3-embedding-8b",
			Provider:              "siliconflow",
			ModelName:             "Qwen/Qwen3-Embedding-8B",
			Dimension:             4096,
			MaxInputTokens:        8192,
			DatasetName:           deriveDatasetName("siliconflow", "Qwen/Qwen3-Embedding-8B", 4096),
			QueryPrefix:           qwenQueryPrefix,
			PassagePrefix:         qwenPassagePrefix,
			PricingPer1MTokensUSD: 0.04,
			IsActive:              true,
		},
		{
			ID:                    "openai:text-embedding-3-small",
			Provider:              "openai",
			ModelName:             "text-embedding-3-small",
			Dimension:             1536,
			MaxInputTokens:        8191,
			DatasetName:           deriveDatasetName("openai", "text-embedding-3-small", 1536),
			QueryPrefix:           nil,
			PassagePrefix:         nil,
			PricingPer1MTokensUSD: 0.02,
			IsActive:              true,
		},
		{
			ID:                    "openai:text-embedding-3-large",
			Provider:              "openai",
			ModelName:             "text-embedding-3-large",
			Dimension:             3072,
			MaxInputTokens:        8191,
			DatasetName:           deriveDatasetName("openai", "text-embedding-3-large", 3072),
			QueryPrefix:           nil,
			PassagePrefix:         nil,
			PricingPer1MTokensUSD: 0.13,
			IsActive:              true,
		},
	}
}

// deriveDatasetName builds a deterministic LanceDB dataset name from the
// model's (provider, modelName, dim) triple. The format is:
//
//	rag_chunks__<provider>_<model-basename>__<dim>
//
// Normalization rules:
//
//  1. Strip the org/namespace prefix from modelName (everything up to
//     and including the last `/`): "Qwen/Qwen3-Embedding-4B" →
//     "Qwen3-Embedding-4B". HuggingFace-style org prefixes are
//     redundant once the provider is already in the name.
//  2. Lowercase.
//  3. Replace `-` with `_`.
//
// Dots and other characters are left alone because LanceDB tolerates
// them and collisions are bounded by the provider prefix.
//
// Example: ("siliconflow", "Qwen/Qwen3-Embedding-4B", 2560) →
// "rag_chunks__siliconflow_qwen3_embedding_4b__2560".
//
// Dim is part of the name to prevent a dimension-change silently reusing
// an existing dataset — LanceDB schemas are not automatically migrated.
func deriveDatasetName(provider, modelName string, dim int) string {
	// Basename: everything after the last `/`. If no `/`, the whole
	// string is the basename.
	if idx := strings.LastIndex(modelName, "/"); idx >= 0 {
		modelName = modelName[idx+1:]
	}

	normalize := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "-", "_")
		return s
	}
	return "rag_chunks__" + normalize(provider) + "_" + normalize(modelName) + "__" + strconv.Itoa(dim)
}
