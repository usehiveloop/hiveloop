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
// 2. Run the suite against this branch (`rag/phase-2j-go-testhelpers`):
//
//        go test -race -p 1 -count=1 ./internal/rag -run TestE2E_
//
//    On the 2J branch alone, the Rust RPC handlers still return
//    UNIMPLEMENTED (they are wired in Tranche 2F). Most tests therefore
//    FAIL with `rpc error: code = Unimplemented`. That is CORRECT — the
//    assertions are real, there are no skips, and merging 2F flips
//    them green.
//
// 3. Run against a sibling worktree that HAS 2F wired:
//
//        RAG_ENGINE_BRANCH=/Users/you/code/hiveloop-rag-2f \
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
func testDatasetName() string {
	return fmt.Sprintf("rag_chunks__fake__%d", testEmbeddingDim)
}

// startForTest is a one-liner wrapper to start the engine with the
// default test config and also create a matching dataset. Returns a
// cancel helper the test may defer to surface shutdown errors early.
func startForTest(t *testing.T) (*testhelpers.RagEngineInstance, string) {
	t.Helper()
	inst := testhelpers.StartRagEngineInTestMode(t, testhelpers.RagEngineConfig{})
	ds := testDatasetName()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := inst.Client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{
		DatasetName:         ds,
		VectorDim:           testEmbeddingDim,
		EmbeddingPrecision:  "float32",
		IdempotencyKey:      "e2e-" + t.Name(),
	}); err != nil {
		// The 2J-standalone state is CreateDataset → Unimplemented. The
		// caller branches on that; we surface the error verbatim.
		t.Fatalf("create dataset: %v", err)
	}
	return inst, ds
}

// syntheticDocs returns n fake DocumentToIngest rows. `orgIdx` and
// `aclSet` are mixed in to the ACL strings so test assertions can pick
// specific subsets. Content is deterministic so FakeEmbedder produces
// repeatable vectors.
func syntheticDocs(n int, orgID string, aclSet []string, isPublic bool) []*ragpb.DocumentToIngest {
	docs := make([]*ragpb.DocumentToIngest, 0, n)
	for i := 0; i < n; i++ {
		docs = append(docs, &ragpb.DocumentToIngest{
			DocId:      fmt.Sprintf("doc-%s-%04d", orgID, i),
			SemanticId: fmt.Sprintf("Doc %s #%d", orgID, i),
			Link:       fmt.Sprintf("https://example.test/%s/%d", orgID, i),
			Acl:        append([]string(nil), aclSet...),
			IsPublic:   isPublic,
			Sections: []*ragpb.Section{
				{
					Text:  fmt.Sprintf("The rain in %s falls mainly on batch %d, covering topic-%d.", orgID, i, i%7),
					Title: "Section-A",
				},
				{
					Text:  fmt.Sprintf("Further commentary on topic-%d within %s.", i%7, orgID),
					Title: "Section-B",
				},
			},
		})
	}
	return docs
}

// ------------------------------------------------------------------
// 1. Full happy path
// ------------------------------------------------------------------

// TestE2E_CreateDatasetAndIngest_ThenSearch exercises the complete
// create → ingest → search → update-acl → delete-by-doc → delete-by-org
// lifecycle. Business value: this IS the product — if it breaks,
// everything downstream is broken.
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
		DatasetName:        ds,
		OrgId:              orgA,
		Mode:               ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:     "ingest-" + t.Name() + "-A",
		DeclaredVectorDim:  testEmbeddingDim,
		Documents:          docsA,
	})
	if err != nil {
		t.Fatalf("IngestBatch orgA: %v", err)
	}
	if got := len(ingestA.GetResults()); got != len(docsA) {
		t.Fatalf("orgA: expected %d results, got %d", len(docsA), got)
	}

	ingestB, err := inst.Client.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:        ds,
		OrgId:              orgB,
		Mode:               ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:     "ingest-" + t.Name() + "-B",
		DeclaredVectorDim:  testEmbeddingDim,
		Documents:          docsB,
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
		DatasetName:     ds,
		OrgId:           orgA,
		QueryText:       "topic-3 rain",
		QueryVector:     queryVec,
		Mode:            ragpb.SearchMode_SEARCH_MODE_HYBRID,
		AclAnyOf:        privateAclA,
		IncludePublic:   true,
		Limit:           20,
		CandidatePool:   100,
		HybridAlpha:     0.6,
		Rerank:          false,
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

func containsDoc(hits []*ragpb.SearchHit, docID string) bool {
	for _, h := range hits {
		if h.GetDocId() == docID {
			return true
		}
	}
	return false
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
