package tasks_test

import (
	"context"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

// TestPrune_DeletesDocsMissingUpstream: pre-seed 10 docs, the slim
// connector reports only 7 — after pruning, rag_documents and
// rag_document_by_sources must each contain 7 rows (the 3 dropped IDs
// are gone), and the source's last_pruned column is advanced.
func TestPrune_DeletesDocsMissingUpstream(t *testing.T) {
	f := setupTask(t)

	// Pre-seed: 10 docs, all under one source.
	kind := nextStubKind()
	src := f.makeSource(t, kind)

	docs := genDocs("prune", 10)
	if err := preseedDocs(t, f, docs, []string{"user:any@x"}); err != nil {
		t.Fatalf("preseed: %v", err)
	}
	for _, d := range docs {
		if err := f.DB.Create(&ragmodel.RAGDocumentBySource{
			DocumentID:     d.DocID,
			RAGSourceID:    src.ID,
			HasBeenIndexed: true,
		}).Error; err != nil {
			t.Fatalf("create junction row: %v", err)
		}
	}

	// Slim connector returns the FIRST 7 of those doc IDs.
	keepIDs := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		keepIDs = append(keepIDs, docs[i].DocID)
	}
	stub := &stubConnector{slimIDs: keepIDs}
	registerStub(kind, stub)

	task, err := ragtasks.NewPruneTask(ragtasks.PrunePayload{RAGSourceID: src.ID})
	if err != nil {
		t.Fatalf("build prune task: %v", err)
	}
	if err := f.Deps.HandlePrune(context.Background(), task); err != nil {
		t.Fatalf("HandlePrune: %v", err)
	}

	// Junction rows: 7 left.
	var junctionCount int64
	f.DB.Model(&ragmodel.RAGDocumentBySource{}).
		Where("rag_source_id = ?", src.ID).
		Count(&junctionCount)
	if junctionCount != 7 {
		t.Fatalf("rag_document_by_sources count = %d, want 7", junctionCount)
	}
	// rag_documents: orphans removed (the 3 dropped ones had no other source).
	var docCount int64
	f.DB.Model(&ragmodel.RAGDocument{}).
		Where("org_id = ?", src.OrgIDValue).
		Count(&docCount)
	if docCount != 7 {
		t.Fatalf("rag_documents count = %d, want 7", docCount)
	}
	// Verify the dropped IDs are gone.
	for i := 7; i < 10; i++ {
		var row ragmodel.RAGDocument
		err := f.DB.First(&row, "id = ?", docs[i].DocID).Error
		if err == nil {
			t.Fatalf("doc %s still present after prune", docs[i].DocID)
		}
	}

	got := reloadSource(t, f.DB, src.ID)
	if got.LastPruned == nil {
		t.Fatalf("last_pruned not advanced after prune")
	}
	_ = interfaces.SlimDocument{} // keep import alive in case we expand
}
