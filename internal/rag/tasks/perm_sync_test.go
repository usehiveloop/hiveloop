package tasks_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

// Verifies that perm-sync updates ACLs without re-running IngestBatch
// (no re-embed, no chunk rewrite).
func TestPermSync_PushesAclWithoutReembed(t *testing.T) {
	f := setupTask(t)

	origACL := []string{"user:original@example.com"}
	docs := genDocs("perm", 3)
	if err := preseedDocs(t, f, docs, origACL); err != nil {
		t.Fatalf("preseed docs: %v", err)
	}

	kind := nextStubKind()
	newACL := []string{"user:new@example.com", "user:second@example.com"}
	access := make([]interfaces.DocExternalAccess, 0, len(docs))
	for i := range docs {
		access = append(access, interfaces.DocExternalAccess{
			DocID: docs[i].DocID,
			ExternalAccess: &interfaces.ExternalAccess{
				ExternalUserEmails: newACL,
			},
		})
	}
	stub := &stubConnector{permSet: access}
	registerStub(kind, stub)

	src := f.makeSource(t, kind)
	task, err := ragtasks.NewPermSyncTask(ragtasks.PermSyncPayload{RAGSourceID: src.ID})
	if err != nil {
		t.Fatalf("build perm-sync task: %v", err)
	}
	if err := f.Deps.HandlePermSync(context.Background(), task); err != nil {
		t.Fatalf("HandlePermSync: %v", err)
	}

	for _, d := range docs {
		var row ragmodel.RAGDocument
		if err := f.DB.First(&row, "id = ?", d.DocID).Error; err != nil {
			t.Fatalf("reload %s: %v", d.DocID, err)
		}
		if !equalSlices(row.ExternalUserEmails, newACL) {
			t.Fatalf("doc %s acl = %v, want %v", d.DocID, []string(row.ExternalUserEmails), newACL)
		}
	}

	got := reloadSource(t, f.DB, src.ID)
	if got.LastTimePermSync == nil {
		t.Fatalf("last_time_perm_sync not advanced after perm-sync")
	}
}

func preseedDocs(t *testing.T, f *taskFixture, docs []interfaces.Document, acl []string) error {
	t.Helper()
	pbDocs := make([]*ragpb.DocumentToIngest, 0, len(docs))
	for i := range docs {
		pbDocs = append(pbDocs, &ragpb.DocumentToIngest{
			DocId:      docs[i].DocID,
			SemanticId: docs[i].SemanticID,
			Acl:        acl,
			Sections:   []*ragpb.Section{{Text: docs[i].Sections[0].Text}},
		})
	}
	ctx := context.Background()
	if _, err := f.Deps.RagClient.IngestBatch(ctx, &ragpb.IngestBatchRequest{
		DatasetName:       f.Deps.DatasetName,
		OrgId:             f.Org.ID.String(),
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    "preseed-" + t.Name(),
		DeclaredVectorDim: f.Deps.DeclaredVectorDim,
		Documents:         pbDocs,
	}); err != nil {
		return err
	}
	for i := range docs {
		row := ragmodel.RAGDocument{
			ID:                 docs[i].DocID,
			OrgID:              f.Org.ID,
			SemanticID:         docs[i].SemanticID,
			ExternalUserEmails: pq.StringArray(acl),
		}
		if err := f.DB.Save(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func equalSlices[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var _ = json.Marshal
