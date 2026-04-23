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
//  1. Start infra (Postgres + Redis + MinIO with the hiveloop-rag-test
//     bucket auto-created):
//
//     make test-services-up
//
// 2. Run the suite:
//
//	    go test -race -p 1 -count=1 ./internal/rag -run TestE2E_
//
//	If the Rust rag-engine built from this worktree still has handlers
//	returning UNIMPLEMENTED, the tests FAIL with `rpc error: code =
//	Unimplemented` — that is the intended signal. Assertions are
//	real, there are no skips; shipping a handler flips the matching
//	test green.
//
// 3. Run against a sibling worktree that ships a newer rag-engine:
//
//	    RAG_ENGINE_BRANCH=/Users/you/code/hiveloop-rag-engine \
//	      go test -race -p 1 -count=1 ./internal/rag -run TestE2E_
//
//	RAG_ENGINE_BRANCH may be an absolute path to:
//	    - a worktree root (containing services/rag-engine/Cargo.toml)
//	    - a pre-built rag-engine-server binary
//
// NO MOCKS. NO SKIPS. Per TESTING.md Hard Rules #1, #2, #7.
package rag_test

import (
	"context"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

func TestE2E_PermSyncPattern(t *testing.T) {
	inst, ds := startForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	orgID := "perm-org"
	docID := "perm-doc-1"
	qtext := "unique perm-sync phrase marker-alpha"

	if _, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             orgID,
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "perm-init-" + t.Name(),
		DeclaredVectorDim: testEmbeddingDim,
		Documents: []*ragpb.DocumentToIngest{
			{
				DocId:      docID,
				SemanticId: "Perm Doc",
				Acl:        []string{"user:A"},
				Sections:   []*ragpb.Section{{Text: qtext}},
			},
		},
	}); err != nil {
		t.Fatalf("IngestBatch: %v", err)
	}

	queryVec := testhelpers.FakeVector(qtext, testhelpers.FakeEmbedKindQuery, testEmbeddingDim)
	searchForUser := func(user string) []*ragpb.SearchHit {
		resp, err := inst.Client.Search(ctx, &ragpb.SearchRequest{
			DatasetName:   ds,
			OrgId:         orgID,
			QueryText:     qtext,
			QueryVector:   queryVec,
			Mode:          ragpb.SearchMode_SEARCH_MODE_HYBRID,
			AclAnyOf:      []string{user},
			IncludePublic: false,
			Limit:         10,
			CandidatePool: 50,
			HybridAlpha:   0.6,
		})
		if err != nil {
			t.Fatalf("Search as %s: %v", user, err)
		}
		return resp.GetHits()
	}

	// A can see it pre-update.
	if !containsDoc(searchForUser("user:A"), docID) {
		t.Fatalf("pre-update: user:A should see %s", docID)
	}

	// Reassign.
	if _, err := inst.Client.UpdateACL(ctx, &ragpb.UpdateACLRequest{
		DatasetName:    ds,
		OrgId:          orgID,
		IdempotencyKey: "perm-reassign-" + t.Name(),
		Entries: []*ragpb.UpdateACLEntry{
			{DocId: docID, Acl: []string{"user:B", "user:C"}, IsPublic: false},
		},
	}); err != nil {
		t.Fatalf("UpdateACL: %v", err)
	}

	if containsDoc(searchForUser("user:A"), docID) {
		t.Fatalf("post-update: user:A must NOT see %s", docID)
	}
	if !containsDoc(searchForUser("user:B"), docID) {
		t.Fatalf("post-update: user:B must see %s", docID)
	}
	if !containsDoc(searchForUser("user:C"), docID) {
		t.Fatalf("post-update: user:C must see %s", docID)
	}
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------
