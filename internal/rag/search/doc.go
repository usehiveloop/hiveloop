// Package search exposes the Search() helper that composes hybrid
// (BM25 + vector) retrieval, ACL filtering, and reranking.
//
// Ports backend/onyx/context/search/ — specifically pipeline.py's
// SearchPipeline. Phase 2 scope stops at retrieval; chat/answer generation
// is explicitly out of scope per the plan's locked decisions.
package search
