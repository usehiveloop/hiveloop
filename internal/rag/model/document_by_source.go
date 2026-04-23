package model

import (
	"github.com/google/uuid"
)

// RAGDocumentBySource is the junction table recording that a particular
// RAGDocument was indexed by a particular RAGSource. Port of Onyx's
// DocumentByConnectorCredentialPair at
// backend/onyx/db/models.py:2512-2558.
//
// DEVIATION: Onyx keys by (document_id, connector_id, credential_id).
// Hiveloop keys by (document_id, rag_source_id) — every RAG table
// uniformly references the top-level RAGSource, which itself carries
// the InConnection reference for integration-backed sources.
//
// The same document can be indexed by multiple sources (imagine a shared
// Google Drive file visible under two org-member sources); this table is
// the many-to-many edge that makes per-source counts and per-source
// pruning possible.
//
// FKs use bare column fields — no association struct — because gorm's
// auto-FK inference on composite PKs is finicky and we want the
// migration to be deterministic.
type RAGDocumentBySource struct {
	// DocumentID — part of the composite PK, FK to rag_documents.id.
	// Port of `id` at models.py:2517.
	DocumentID string `gorm:"type:text;primaryKey"`

	// RAGSourceID — part of the composite PK, FK to rag_sources.id with
	// ON DELETE CASCADE (removing a source must sweep its edges, but
	// NOT the shared document — see the cascade test in document_test.go).
	// Adapts Onyx's composite connector_id + credential_id at
	// models.py:2519-2525.
	RAGSourceID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_rag_doc_source_source;index:idx_rag_doc_source_counts,priority:1"`

	// HasBeenIndexed distinguishes edges that were created purely by
	// permission-syncing from edges that represent a completed content
	// indexing. Port of `has_been_indexed` at models.py:2530-2531 (see
	// the Onyx comment at 2527-2530 for the semantics). Part of the
	// counts index so the scheduler can efficiently compute "how many
	// docs has this source actually finished indexing".
	HasBeenIndexed bool `gorm:"not null;index:idx_rag_doc_source_counts,priority:2"`
}

func (RAGDocumentBySource) TableName() string { return "rag_document_by_sources" }
