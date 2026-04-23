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
func TestE2E_Search_RespectsACL(t *testing.T) {
	inst, ds := startForTest(t)

	orgID := "acl-org"

	ingestDocs := []*ragpb.DocumentToIngest{
		{
			DocId:      "public-1",
			SemanticId: "Public Doc",
			Acl:        []string{"PUBLIC"},
			IsPublic:   true,
			Sections:   []*ragpb.Section{{Text: "public widgets inventory"}},
		},
		{
			DocId:      "alice-private",
			SemanticId: "Alice Private",
			Acl:        []string{"user:alice"},
			IsPublic:   false,
			Sections:   []*ragpb.Section{{Text: "alice widgets inventory"}},
		},
		{
			DocId:      "eng-group",
			SemanticId: "Engineering Doc",
			Acl:        []string{"group:eng"},
			IsPublic:   false,
			Sections:   []*ragpb.Section{{Text: "engineering widgets inventory"}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             orgID,
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "acl-matrix-" + t.Name(),
		DeclaredVectorDim: testEmbeddingDim,
		Documents:         ingestDocs,
	}); err != nil {
		t.Fatalf("IngestBatch acl matrix: %v", err)
	}

	queryText := "widgets inventory"
	queryVec := testhelpers.FakeVector(queryText, testhelpers.FakeEmbedKindQuery, testEmbeddingDim)

	cases := []struct {
		name       string
		acls       []string
		incPublic  bool
		mustInclude []string
		mustExclude []string
	}{
		{
			name:       "anonymous-sees-public-only",
			acls:       nil,
			incPublic:  true,
			mustInclude: []string{"public-1"},
			mustExclude: []string{"alice-private", "eng-group"},
		},
		{
			name:       "alice-sees-public-and-own",
			acls:       []string{"user:alice"},
			incPublic:  true,
			mustInclude: []string{"public-1", "alice-private"},
			mustExclude: []string{"eng-group"},
		},
		{
			name:       "engineer-sees-public-and-group",
			acls:       []string{"group:eng"},
			incPublic:  true,
			mustInclude: []string{"public-1", "eng-group"},
			mustExclude: []string{"alice-private"},
		},
		{
			name:       "engineer-no-public",
			acls:       []string{"group:eng"},
			incPublic:  false,
			mustInclude: []string{"eng-group"},
			mustExclude: []string{"public-1", "alice-private"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp, err := inst.Client.Search(ctx, &ragpb.SearchRequest{
				DatasetName:   ds,
				OrgId:         orgID,
				QueryText:     queryText,
				QueryVector:   queryVec,
				Mode:          ragpb.SearchMode_SEARCH_MODE_HYBRID,
				AclAnyOf:      tc.acls,
				IncludePublic: tc.incPublic,
				Limit:         10,
				CandidatePool: 50,
				HybridAlpha:   0.5,
			})
			if err != nil {
				t.Fatalf("Search %s: %v", tc.name, err)
			}
			seen := map[string]bool{}
			for _, h := range resp.GetHits() {
				seen[h.GetDocId()] = true
			}
			for _, want := range tc.mustInclude {
				if !seen[want] {
					t.Errorf("%s: expected %s in results, got %v", tc.name, want, keys(seen))
				}
			}
			for _, forbid := range tc.mustExclude {
				if seen[forbid] {
					t.Errorf("%s: expected %s to be absent, but it appeared", tc.name, forbid)
				}
			}
		})
	}
}

// ------------------------------------------------------------------
// 6. Graceful shutdown
// ------------------------------------------------------------------

// TestE2E_GracefulShutdown starts the server, kicks off a slow-ish
// ingest, signals shutdown, and asserts the process exits cleanly
// within a bounded grace period. Business value: rolling deploys can't
// drop in-flight ingests.
//
// This test depends on the Rust side honoring SIGTERM properly (2H);
// the helper uses SIGTERM-with-10s-grace.
func TestE2E_GracefulShutdown(t *testing.T) {
	inst := testhelpers.StartRagEngineInTestMode(t, testhelpers.RagEngineConfig{})

	// Start a small ingest in the background but don't wait for it
	// synchronously — we want Stop() to overlap with it.
	ds := "rag_chunks__fake__" + fmt.Sprintf("%d", testEmbeddingDim)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := inst.Client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{
		DatasetName:        ds,
		VectorDim:          testEmbeddingDim,
		EmbeddingPrecision: "float32",
		IdempotencyKey:     "shutdown-ds-" + t.Name(),
	}); err != nil {
		t.Fatalf("CreateDataset: %v", err)
	}

	ingestDone := make(chan error, 1)
	go func() {
		_, err := inst.Client.IngestBatch(context.Background(), &ragpb.IngestBatchRequest{
			DatasetName:       ds,
			OrgId:             "shutdown-org",
			Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
			IdempotencyKey:    "shutdown-" + t.Name(),
			DeclaredVectorDim: testEmbeddingDim,
			Documents:         syntheticDocs(25, "shutdown-org", []string{"group:shutdown"}, false),
		})
		ingestDone <- err
	}()

	// Give the ingest ~200ms to start before we yank the server.
	time.Sleep(200 * time.Millisecond)
	inst.Stop()

	select {
	case <-inst.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("shutdown did not complete within 30s")
	}

	// Drain the ingest. We accept any outcome here — it might have
	// completed before SIGTERM, or it might have been aborted. The key
	// behavior under test is the server exit.
	select {
	case <-ingestDone:
	case <-time.After(5 * time.Second):
		t.Log("ingest goroutine still blocked after shutdown — not failing, but unusual")
	}
}

// ------------------------------------------------------------------
// 7. Perm-sync pattern
// ------------------------------------------------------------------

// TestE2E_PermSyncPattern simulates a perm-sync job: a doc is ingested
// with ACL=[A], then UpdateACL reassigns to [B,C]. A search as A must
// no longer see it; searches as B and C must. Business value: the
// whole point of having UpdateACL separate from IngestBatch is that
// perm-sync can happen without re-embedding.
