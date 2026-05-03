package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type assetsListHarness struct {
	db     *gorm.DB
	router *chi.Mux

	orgA model.Org
	orgB model.Org

	agentA1 uuid.UUID
	agentA2 uuid.UUID
	agentB1 uuid.UUID

	convA1 uuid.UUID // agent A1
	convA2 uuid.UUID // agent A2
	convB1 uuid.UUID // org B
}

// seedAssetRow inserts a ConversationAsset directly (no S3) so the listing
// tests don't depend on MinIO being up. The list endpoint never touches S3.
func seedAssetRow(t *testing.T, db *gorm.DB, conv model.AgentConversation, folder, filename string, createdAt time.Time) model.ConversationAsset {
	t.Helper()
	key := fmt.Sprintf("pub/c/%s/%s", conv.ID, filename)
	if folder != "" {
		key = fmt.Sprintf("pub/c/%s/%s/%s", conv.ID, folder, filename)
	}
	a := model.ConversationAsset{
		ID:             uuid.New(),
		ConversationID: conv.ID,
		OrgID:          conv.OrgID,
		SandboxID:      conv.SandboxID,
		Path:           folder,
		Filename:       filename,
		Key:            key,
		PublicURL:      "https://cdn.example.com/" + key,
		ContentType:    "text/plain",
		Bytes:          7,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", a.ID).Delete(&model.ConversationAsset{}) })
	return a
}

func newAssetsListHarness(t *testing.T) *assetsListHarness {
	t.Helper()
	db := connectTestDB(t)

	h := handler.NewUploadsHandler(db, nil)
	r := chi.NewRouter()
	r.Get("/v1/assets", h.ListAssets)

	mkOrg := func(prefix string) model.Org {
		o := model.Org{
			ID:        uuid.New(),
			Name:      fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8]),
			RateLimit: 1000,
			Active:    true,
		}
		if err := db.Create(&o).Error; err != nil {
			t.Fatalf("create org: %v", err)
		}
		t.Cleanup(func() { db.Where("id = ?", o.ID).Delete(&model.Org{}) })
		return o
	}
	mkAgent := func(orgID uuid.UUID, name string) uuid.UUID {
		id := uuid.New()
		if err := db.Create(&model.Agent{ID: id, OrgID: &orgID, Name: name, Status: "active"}).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
		return id
	}
	mkSandbox := func(orgID, agentID uuid.UUID) uuid.UUID {
		id := uuid.New()
		if err := db.Create(&model.Sandbox{
			ID:                    id,
			OrgID:                 &orgID,
			AgentID:               &agentID,
			EncryptedBridgeAPIKey: []byte("placeholder"),
			Status:                "running",
			ExternalID:            "x",
			BridgeURL:             "http://localhost:1",
		}).Error; err != nil {
			t.Fatalf("create sandbox: %v", err)
		}
		return id
	}
	mkConv := func(orgID, agentID, sandboxID uuid.UUID) uuid.UUID {
		id := uuid.New()
		if err := db.Create(&model.AgentConversation{
			ID:                   id,
			OrgID:                orgID,
			AgentID:              agentID,
			SandboxID:            sandboxID,
			BridgeConversationID: "bridge-" + uuid.New().String()[:8],
			Status:               "active",
		}).Error; err != nil {
			t.Fatalf("create conversation: %v", err)
		}
		return id
	}

	orgA := mkOrg("assets-a")
	orgB := mkOrg("assets-b")

	agentA1 := mkAgent(orgA.ID, "agent-a1")
	agentA2 := mkAgent(orgA.ID, "agent-a2")
	agentB1 := mkAgent(orgB.ID, "agent-b1")

	sbA1 := mkSandbox(orgA.ID, agentA1)
	sbA2 := mkSandbox(orgA.ID, agentA2)
	sbB1 := mkSandbox(orgB.ID, agentB1)

	convA1 := mkConv(orgA.ID, agentA1, sbA1)
	convA2 := mkConv(orgA.ID, agentA2, sbA2)
	convB1 := mkConv(orgB.ID, agentB1, sbB1)

	return &assetsListHarness{
		db: db, router: r,
		orgA: orgA, orgB: orgB,
		agentA1: agentA1, agentA2: agentA2, agentB1: agentB1,
		convA1: convA1, convA2: convA2, convB1: convB1,
	}
}

func (h *assetsListHarness) get(t *testing.T, query string, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/assets"+query, nil)
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *assetsListHarness) loadConv(t *testing.T, id uuid.UUID) model.AgentConversation {
	t.Helper()
	var c model.AgentConversation
	if err := h.db.Where("id = ?", id).First(&c).Error; err != nil {
		t.Fatalf("load conv: %v", err)
	}
	return c
}

type assetListPage struct {
	Data       []map[string]any `json:"data"`
	HasMore    bool             `json:"has_more"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}

func decodeAssetList(t *testing.T, rr *httptest.ResponseRecorder) assetListPage {
	t.Helper()
	var p assetListPage
	if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode list: %v body=%s", err, rr.Body.String())
	}
	return p
}
