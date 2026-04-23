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
	"strings"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

func TestE2E_CreateDatasetAndIngest_ThenSearch(t *testing.T) {
	inst, ds := startForTest(t)

	orgA := "org_A"
	orgB := "org_B"

	publicACL := []string{"PUBLIC"}
	privateAclA := []string{"user:alice@A.test", "group:A-engineers"}
	privateAclB := []string{"user:bob@B.test", "group:B-engineers"}

	// --- INGEST 100 docs for orgA (mix of public + private) --------
	docsA := syntheticDocs(50, orgA, privateAclA, false)
	docsA = append(docsA, syntheticDocs(50, orgA+"-public", publicACL, true)...)
	docsB := syntheticDocs(50, orgB, privateAclB, false)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ingestA, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             orgA,
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "ingest-" + t.Name() + "-A",
		DeclaredVectorDim: testEmbeddingDim,
		Documents:         docsA,
	})
	if err != nil {
		t.Fatalf("IngestBatch orgA: %v", err)
	}
	if got := len(ingestA.GetResults()); got != len(docsA) {
		t.Fatalf("orgA: expected %d results, got %d", len(docsA), got)
	}

	ingestB, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       ds,
		OrgId:             orgB,
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "ingest-" + t.Name() + "-B",
		DeclaredVectorDim: testEmbeddingDim,
		Documents:         docsB,
	})
	if err != nil {
		t.Fatalf("IngestBatch orgB: %v", err)
	}
	if got := len(ingestB.GetResults()); got != len(docsB) {
		t.Fatalf("orgB: expected %d results, got %d", len(docsB), got)
	}

	// --- SEARCH as orgA with their private ACL ---------------------
	queryVec := testhelpers.FakeVector("topic-3 rain", testhelpers.FakeEmbedKindQuery, testEmbeddingDim)
	searchA, err := inst.Client.Search(ctx, &ragpb.SearchRequest{
		DatasetName:   ds,
		OrgId:         orgA,
		QueryText:     "topic-3 rain",
		QueryVector:   queryVec,
		Mode:          ragpb.SearchMode_SEARCH_MODE_HYBRID,
		AclAnyOf:      privateAclA,
		IncludePublic: true,
		Limit:         20,
		CandidatePool: 100,
		HybridAlpha:   0.6,
		Rerank:        false,
	})
	if err != nil {
		t.Fatalf("Search orgA: %v", err)
	}
	if len(searchA.GetHits()) == 0 {
		t.Fatalf("orgA search returned zero hits — expected ≥1")
	}
	// Assert no orgB doc_ids leaked into orgA's result set.
	for _, h := range searchA.GetHits() {
		if strings.Contains(h.GetDocId(), "-"+orgB+"-") {
			t.Fatalf("cross-org leak: orgA search returned %s", h.GetDocId())
		}
	}

	// --- SEARCH as anonymous/public-only ---------------------------
	searchPub, err := inst.Client.Search(ctx, &ragpb.SearchRequest{
		DatasetName:   ds,
		OrgId:         orgA,
		QueryText:     "topic-3",
		QueryVector:   queryVec,
		Mode:          ragpb.SearchMode_SEARCH_MODE_HYBRID,
		AclAnyOf:      nil,
		IncludePublic: true,
		Limit:         20,
		CandidatePool: 100,
		HybridAlpha:   0.6,
	})
	if err != nil {
		t.Fatalf("Search public: %v", err)
	}
	for _, h := range searchPub.GetHits() {
		if !strings.Contains(h.GetDocId(), "public") {
			t.Fatalf("public-only search returned non-public doc: %s", h.GetDocId())
		}
	}

	// --- UPDATE ACL: revoke Alice from the first private doc -------
	revokedDoc := docsA[0].GetDocId()
	_, err = inst.Client.UpdateACL(ctx, &ragpb.UpdateACLRequest{
		DatasetName:    ds,
		OrgId:          orgA,
		IdempotencyKey: "acl-" + t.Name(),
		Entries: []*ragpb.UpdateACLEntry{
			{
				DocId:    revokedDoc,
				Acl:      []string{"group:A-engineers"}, // drops alice
				IsPublic: false,
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateACL: %v", err)
	}
	// Alice-only search: must NOT see the revoked doc.
	aliceSearch, err := inst.Client.Search(ctx, &ragpb.SearchRequest{
		DatasetName:   ds,
		OrgId:         orgA,
		QueryText:     docsA[0].GetSemanticId(),
		QueryVector:   testhelpers.FakeVector(docsA[0].GetSemanticId(), testhelpers.FakeEmbedKindQuery, testEmbeddingDim),
		Mode:          ragpb.SearchMode_SEARCH_MODE_HYBRID,
		AclAnyOf:      []string{"user:alice@A.test"},
		IncludePublic: false,
		Limit:         50,
		CandidatePool: 200,
		HybridAlpha:   0.5,
	})
	if err != nil {
		t.Fatalf("Search as alice: %v", err)
	}
	for _, h := range aliceSearch.GetHits() {
		if h.GetDocId() == revokedDoc {
			t.Fatalf("revoked doc %s still visible to alice", revokedDoc)
		}
	}

	// --- DELETE one doc by ID ---------------------------------------
	deletedDoc := docsA[1].GetDocId()
	_, err = inst.Client.DeleteByDocID(ctx, &ragpb.DeleteByDocIDRequest{
		DatasetName:    ds,
		OrgId:          orgA,
		DocIds:         []string{deletedDoc},
		IdempotencyKey: "del-" + t.Name(),
	})
	if err != nil {
		t.Fatalf("DeleteByDocID: %v", err)
	}
	postDelete, err := inst.Client.Search(ctx, &ragpb.SearchRequest{
		DatasetName:   ds,
		OrgId:         orgA,
		QueryText:     docsA[1].GetSemanticId(),
		QueryVector:   testhelpers.FakeVector(docsA[1].GetSemanticId(), testhelpers.FakeEmbedKindQuery, testEmbeddingDim),
		Mode:          ragpb.SearchMode_SEARCH_MODE_HYBRID,
		AclAnyOf:      privateAclA,
		IncludePublic: true,
		Limit:         50,
		CandidatePool: 200,
		HybridAlpha:   0.5,
	})
	if err != nil {
		t.Fatalf("Search post-delete: %v", err)
	}
	for _, h := range postDelete.GetHits() {
		if h.GetDocId() == deletedDoc {
			t.Fatalf("deleted doc %s still returned by search", deletedDoc)
		}
	}

	// --- DELETE all of orgA's content ------------------------------
	_, err = inst.Client.DeleteByOrg(ctx, &ragpb.DeleteByOrgRequest{
		OrgId:          orgA,
		DatasetNames:   []string{ds},
		Confirm:        true,
		IdempotencyKey: "del-org-" + t.Name(),
	})
	if err != nil {
		t.Fatalf("DeleteByOrg: %v", err)
	}
	afterWipe, err := inst.Client.Search(ctx, &ragpb.SearchRequest{
		DatasetName:   ds,
		OrgId:         orgA,
		QueryText:     "anything",
		QueryVector:   queryVec,
		Mode:          ragpb.SearchMode_SEARCH_MODE_HYBRID,
		AclAnyOf:      privateAclA,
		IncludePublic: true,
		Limit:         50,
		CandidatePool: 200,
		HybridAlpha:   0.5,
	})
	if err != nil {
		t.Fatalf("Search post-wipe: %v", err)
	}
	if len(afterWipe.GetHits()) != 0 {
		t.Fatalf("DeleteByOrg left %d hits behind", len(afterWipe.GetHits()))
	}
}

// ------------------------------------------------------------------
// 2. IngestBatch performance — hits SLO
// ------------------------------------------------------------------

// TestE2E_IngestBatch_HitsSLO pushes 500 docs through a single batch
// and asserts <15s wall clock (10s SLO target + 5s overhead budget).
// Business value: the product's SLA explicitly commits to 10s
// per-batch; this test blocks a regression.
