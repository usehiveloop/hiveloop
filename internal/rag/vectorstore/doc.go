// Package vectorstore wraps the LanceDB Go SDK and exposes the abstract
// vector-store operations the rest of the RAG system depends on.
//
// Ports backend/onyx/document_index/vespa/index.py — the analogous Vespa
// client — but swapped to LanceDB over S3-compatible storage (R2 in prod,
// MinIO in dev/test). The Phase 0 spike under spike/ verifies the Go
// bindings meet our requirements before we commit to this stack.
package vectorstore
