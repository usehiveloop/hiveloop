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
	"fmt"
	"sort"
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
		DatasetName:        ds,
		VectorDim:          testEmbeddingDim,
		EmbeddingPrecision: "float32",
		IdempotencyKey:     "e2e-" + t.Name(),
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
