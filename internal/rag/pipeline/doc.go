// Package pipeline orchestrates the end-to-end indexing pipeline: fetch,
// chunk, embed, write to LanceDB + Postgres.
//
// Ports backend/onyx/indexing/indexing_pipeline.py — the build_indexing_pipeline
// composition and its stage functions.
package pipeline
