package model_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// bootstrapDocs opens a test DB with the full RAG schema migrated.
// testhelpers.ConnectTestDB calls rag.AutoMigrate internally, which
// creates every rag_* table, index, constraint, and FK this package
// needs.
func TestRAGHierarchyNode_UniqueRawIDSource(t *testing.T) {
	db := bootstrapDocs(t)
	org := testhelpers.NewTestOrg(t, db)
	cleanupDocsForOrg(t, db, org.ID)

	raw := "collision-raw-" + uuid.NewString()
	first := &ragmodel.RAGHierarchyNode{
		OrgID:       org.ID,
		RawNodeID:   raw,
		DisplayName: "A",
		Source:      ragmodel.DocumentSourceConfluence,
		NodeType:    ragmodel.HierarchyNodeTypeSpace,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := &ragmodel.RAGHierarchyNode{
		OrgID:       org.ID,
		RawNodeID:   raw,
		DisplayName: "B",
		Source:      ragmodel.DocumentSourceConfluence,
		NodeType:    ragmodel.HierarchyNodeTypeSpace,
	}
	err := db.Create(second).Error
	if err == nil {
		t.Fatalf("expected unique-violation on duplicate (raw_node_id, source); got nil")
	}
	if !strings.Contains(err.Error(), "uq_rag_hierarchy_node_raw_id_source") {
		t.Fatalf("error did not mention the unique index: %v", err)
	}
}

func TestRAGDocumentBySource_SourceCascade(t *testing.T) {
	db := bootstrapDocs(t)
	org := testhelpers.NewTestOrg(t, db)
	cleanupDocsForOrg(t, db, org.ID)

	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	doc := &ragmodel.RAGDocument{
		ID:           docID(t),
		OrgID:        org.ID,
		SemanticID:   "Shared Doc",
		LastModified: time.Now(),
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	edge := &ragmodel.RAGDocumentBySource{
		DocumentID:     doc.ID,
		RAGSourceID:    src.ID,
		HasBeenIndexed: true,
	}
	if err := db.Create(edge).Error; err != nil {
		t.Fatalf("create edge: %v", err)
	}

	if err := db.Exec(`DELETE FROM rag_sources WHERE id = ?`, src.ID).Error; err != nil {
		t.Fatalf("delete rag_source: %v", err)
	}

	// Edge gone.
	var edgeCount int64
	if err := db.Model(&ragmodel.RAGDocumentBySource{}).
		Where("document_id = ? AND rag_source_id = ?", doc.ID, src.ID).
		Count(&edgeCount).Error; err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 0 {
		t.Fatalf("expected 0 edge rows after rag_source delete, got %d", edgeCount)
	}

	// Doc survives.
	var docCount int64
	if err := db.Model(&ragmodel.RAGDocument{}).
		Where("id = ?", doc.ID).
		Count(&docCount).Error; err != nil {
		t.Fatalf("count docs: %v", err)
	}
	if docCount != 1 {
		t.Fatalf("doc should survive rag_source delete, got count=%d", docCount)
	}
}

func TestRAGHierarchyNodeType_IsValid(t *testing.T) {
	cases := []struct {
		in   ragmodel.HierarchyNodeType
		want bool
	}{
		{ragmodel.HierarchyNodeTypeFolder, true},
		{ragmodel.HierarchyNodeTypeSource, true},
		{ragmodel.HierarchyNodeTypeSharedDrive, true},
		{ragmodel.HierarchyNodeTypeMyDrive, true},
		{ragmodel.HierarchyNodeTypeSpace, true},
		{ragmodel.HierarchyNodeTypePage, true},
		{ragmodel.HierarchyNodeTypeProject, true},
		{ragmodel.HierarchyNodeTypeDatabase, true},
		{ragmodel.HierarchyNodeTypeWorkspace, true},
		{ragmodel.HierarchyNodeTypeSite, true},
		{ragmodel.HierarchyNodeTypeDrive, true},
		{ragmodel.HierarchyNodeTypeChannel, true},
		{ragmodel.HierarchyNodeType(""), false},
		{ragmodel.HierarchyNodeType("random_string"), false},
		{ragmodel.HierarchyNodeType("Folder"), false}, // case-sensitive per Onyx
		{ragmodel.HierarchyNodeType(" folder"), false},
	}
	for _, c := range cases {
		got := c.in.IsValid()
		if got != c.want {
			t.Errorf("HierarchyNodeType(%q).IsValid() = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDocumentSource_IsValid(t *testing.T) {
	// Spot-check across the enum: first, last-from-Onyx-range, special
	// cases, and a couple of mid-range ones.
	positives := []ragmodel.DocumentSource{
		ragmodel.DocumentSourceIngestionAPI,
		ragmodel.DocumentSourceSlack,
		ragmodel.DocumentSourceGithub,
		ragmodel.DocumentSourceConfluence,
		ragmodel.DocumentSourceGoogleDrive,
		ragmodel.DocumentSourceNotion,
		ragmodel.DocumentSourceS3,
		ragmodel.DocumentSourceGoogleCloudStorage,
		ragmodel.DocumentSourceNotApplicable,
		ragmodel.DocumentSourceMockConnector,
		ragmodel.DocumentSourceCraftFile,
		ragmodel.DocumentSourceBitbucket,
		ragmodel.DocumentSourceTestrail,
	}
	for _, p := range positives {
		if !p.IsValid() {
			t.Errorf("expected %q to be valid", p)
		}
	}
	negatives := []ragmodel.DocumentSource{
		"",
		"random_source",
		"Slack",
		"google-drive",
		" slack",
		"jira ",
	}
	for _, n := range negatives {
		if n.IsValid() {
			t.Errorf("expected %q to be invalid", n)
		}
	}
}

