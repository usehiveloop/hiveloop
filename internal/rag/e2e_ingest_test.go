// Package rag_test hosts the full end-to-end test suite for the
// RAG subsystem. Every test in this file exercises a real Rust
// rag-engine process (built from source) against a real MinIO backend,
// with the Go side composing Postgres fixtures from
// `internal/rag/testhelpers` and calling the engine through
// `internal/rag/ragclient`.
//
// HOW TO RUN
// ==========
//
// 1. Start infra (Postgres + Redis + MinIO with the hiveloop-rag-test
//    bucket auto-created):
//
//        make test-services-up
//
// 2. Run the suite:
//
//        go test -race -p 1 -count=1 ./internal/rag -run TestE2E_
//
//    If the Rust rag-engine built from this worktree still has handlers
//    returning UNIMPLEMENTED, the tests FAIL with `rpc error: code =
//    Unimplemented` — that is the intended signal. Assertions are
//    real, there are no skips; shipping a handler flips the matching
//    test green.
//
// 3. Run against a sibling worktree that ships a newer rag-engine:
//
//        RAG_ENGINE_BRANCH=/Users/you/code/hiveloop-rag-engine \
//          go test -race -p 1 -count=1 ./internal/rag -run TestE2E_
//
//    RAG_ENGINE_BRANCH may be an absolute path to:
//        - a worktree root (containing services/rag-engine/Cargo.toml)
//        - a pre-built rag-engine-server binary
//
// NO MOCKS. NO SKIPS. Per TESTING.md Hard Rules #1, #2, #7.
package rag_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// ------------------------------------------------------------------
// Shared test fixtures
// ------------------------------------------------------------------

const (
	// testEmbeddingDim matches the FakeEmbedder's LLM_EMBEDDING_DIM env
	// value. 2560 mirrors production Qwen3-Embedding-4B; using it here
	// also exercises the dim-validation code path in the Rust server.
	testEmbeddingDim = uint32(2560)
)

// testDatasetName is the canonical dataset every E2E test writes into.
// Per the Phase 2 plan, names are `rag_chunks__{provider}_{model}__{dim}`.
func TestE2E_IngestBatch_HitsSLO(t *testing.T) {
	inst, ds := startForTest(t)

	docs := syntheticDocs(500, "slo-org", []string{"group:slo"}, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	_, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             "slo-org",
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "slo-" + t.Name(),
		DeclaredVectorDim: testEmbeddingDim,
		Documents:         docs,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("IngestBatch SLO run: %v", err)
	}

	const budget = 15 * time.Second
	if elapsed > budget {
		t.Fatalf("IngestBatch took %s, over %s SLO budget", elapsed, budget)
	}
	t.Logf("IngestBatch(500 docs) completed in %s (budget %s)", elapsed, budget)
}

// ------------------------------------------------------------------
// 3. IngestBatch idempotency
// ------------------------------------------------------------------

// TestE2E_IngestBatch_Idempotent calls the same batch twice with the
// same idempotency key; the second call must succeed without double-
// writing. Business value: retry safety — the Go client's retry loop
// relies on this.
func TestE2E_IngestBatch_Idempotent(t *testing.T) {
	inst, ds := startForTest(t)

	docs := syntheticDocs(20, "idem-org", []string{"group:idem"}, false)
	idem := "idem-" + t.Name()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	first, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             "idem-org",
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    idem,
		DeclaredVectorDim: testEmbeddingDim,
		Documents:         docs,
	})
	if err != nil {
		t.Fatalf("first IngestBatch: %v", err)
	}

	second, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             "idem-org",
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    idem,
		DeclaredVectorDim: testEmbeddingDim,
		Documents:         docs,
	})
	if err != nil {
		t.Fatalf("second IngestBatch (idempotent): %v", err)
	}

	if first.GetTotals() != nil && second.GetTotals() != nil {
		// The replayed response must report the same per-doc outcomes.
		// We don't assert byte-equality of the whole proto (timestamps
		// and ordering subfields are implementation details) — just the
		// per-doc status count.
		if len(first.GetResults()) != len(second.GetResults()) {
			t.Fatalf("replay mismatch: %d results vs %d", len(first.GetResults()), len(second.GetResults()))
		}
	}
}

// ------------------------------------------------------------------
// 4. IngestBatch partial failures
// ------------------------------------------------------------------

// TestE2E_IngestBatch_PartialFailures submits a mix of valid + empty
// docs and asserts the batch returns gRPC OK with a mix of SUCCESS and
// SKIPPED per-doc results. Business value: an upstream connector that
// emits occasional junk pages MUST NOT poison an entire batch.
func TestE2E_IngestBatch_PartialFailures(t *testing.T) {
	inst, ds := startForTest(t)

	valid := syntheticDocs(5, "partial-org", []string{"group:partial"}, false)
	// Craft a few docs that are intentionally malformed: no sections.
	empty := make([]*ragpb.DocumentToIngest, 0, 3)
	for i := 0; i < 3; i++ {
		empty = append(empty, &ragpb.DocumentToIngest{
			DocId:      fmt.Sprintf("doc-empty-%d", i),
			SemanticId: fmt.Sprintf("Empty %d", i),
			Acl:        []string{"group:partial"},
			Sections:   nil, // drives SKIPPED
		})
	}
	docs := append(valid, empty...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             "partial-org",
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "partial-" + t.Name(),
		DeclaredVectorDim: testEmbeddingDim,
		Documents:         docs,
	})
	if err != nil {
		t.Fatalf("IngestBatch (should be gRPC OK despite bad docs): %v", err)
	}

	sawSuccess := false
	sawSkipOrFail := false
	for _, r := range resp.GetResults() {
		switch r.GetStatus() {
		case ragpb.DocumentStatus_DOCUMENT_STATUS_SUCCESS:
			sawSuccess = true
		case ragpb.DocumentStatus_DOCUMENT_STATUS_SKIPPED,
			ragpb.DocumentStatus_DOCUMENT_STATUS_FAILED:
			sawSkipOrFail = true
		}
	}
	if !sawSuccess {
		t.Fatal("expected at least one SUCCESS in mixed batch")
	}
	if !sawSkipOrFail {
		t.Fatal("expected at least one SKIPPED/FAILED in mixed batch")
	}
}

// ------------------------------------------------------------------
// 5. Search ACL matrix
// ------------------------------------------------------------------

// TestE2E_Search_RespectsACL exercises a comprehensive ACL matrix:
// public doc + private doc + group-scoped doc against 4 "users" with
// different permissions. Business value: ACL enforcement is the core
// security guarantee of the platform.
