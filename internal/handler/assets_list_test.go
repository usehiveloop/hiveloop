package handler_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestListAssets_OrgScopeIsolatesOtherOrgs(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	convA := h.loadConv(t, h.convA1)
	convB := h.loadConv(t, h.convB1)
	seedAssetRow(t, h.db, convA, "videos", "a.mp4", now)
	seedAssetRow(t, h.db, convB, "videos", "b.mp4", now)

	rr := h.get(t, "", &h.orgA)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(page.Data))
	}
	if page.Data[0]["filename"] != "a.mp4" {
		t.Fatalf("wrong row leaked: %v", page.Data[0])
	}
}

func TestListAssets_FilterByConversation(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	convA1 := h.loadConv(t, h.convA1)
	convA2 := h.loadConv(t, h.convA2)
	seedAssetRow(t, h.db, convA1, "videos", "a1.mp4", now)
	seedAssetRow(t, h.db, convA2, "videos", "a2.mp4", now)

	rr := h.get(t, "?conversation_id="+h.convA1.String(), &h.orgA)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "a1.mp4" {
		t.Fatalf("filter conversation_id failed: %+v", page.Data)
	}
}

func TestListAssets_FilterByAgent(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	convA1 := h.loadConv(t, h.convA1)
	convA2 := h.loadConv(t, h.convA2)
	seedAssetRow(t, h.db, convA1, "videos", "a1.mp4", now)
	seedAssetRow(t, h.db, convA2, "videos", "a2.mp4", now)

	rr := h.get(t, "?agent_id="+h.agentA2.String(), &h.orgA)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "a2.mp4" {
		t.Fatalf("filter agent_id failed: %+v", page.Data)
	}
	if page.Data[0]["agent_id"] != h.agentA2.String() {
		t.Fatalf("agent_id field: %v", page.Data[0]["agent_id"])
	}
}

func TestListAssets_FilterByPath(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	convA1 := h.loadConv(t, h.convA1)
	seedAssetRow(t, h.db, convA1, "videos", "a.mp4", now)
	seedAssetRow(t, h.db, convA1, "images", "b.png", now)
	seedAssetRow(t, h.db, convA1, "", "root.txt", now)

	rr := h.get(t, "?path=images", &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "b.png" {
		t.Fatalf("filter path failed: %+v", page.Data)
	}

	rr = h.get(t, "?path=", &h.orgA)
	page = decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "root.txt" {
		t.Fatalf("filter path='' (root) failed: %+v", page.Data)
	}
}

func TestListAssets_CombinedFilters(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	convA1 := h.loadConv(t, h.convA1)
	convA2 := h.loadConv(t, h.convA2)
	seedAssetRow(t, h.db, convA1, "videos", "a1-vid.mp4", now)
	seedAssetRow(t, h.db, convA1, "exports", "a1-data.csv", now)
	seedAssetRow(t, h.db, convA2, "videos", "a2-vid.mp4", now)

	q := fmt.Sprintf("?agent_id=%s&path=videos", h.agentA1)
	rr := h.get(t, q, &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "a1-vid.mp4" {
		t.Fatalf("combined filter failed: %+v", page.Data)
	}
}

func TestListAssets_ForeignAgentReturnsEmpty(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	convB := h.loadConv(t, h.convB1)
	seedAssetRow(t, h.db, convB, "videos", "b.mp4", now)

	// Caller is org A but filters by an org-B agent — must not leak.
	rr := h.get(t, "?agent_id="+h.agentB1.String(), &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 0 {
		t.Fatalf("expected empty (foreign agent), got %d rows", len(page.Data))
	}
}

func TestListAssets_Pagination(t *testing.T) {
	h := newAssetsListHarness(t)
	convA1 := h.loadConv(t, h.convA1)
	base := time.Now().Add(-1 * time.Hour)
	for i := range 5 {
		seedAssetRow(t, h.db, convA1, "page", fmt.Sprintf("f%d.txt", i), base.Add(time.Duration(i)*time.Second))
	}

	rr := h.get(t, "?conversation_id="+h.convA1.String()+"&limit=2", &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 2 || !page.HasMore || page.NextCursor == nil {
		t.Fatalf("first page wrong: %+v", page)
	}

	rr = h.get(t,
		fmt.Sprintf("?conversation_id=%s&limit=2&cursor=%s", h.convA1, *page.NextCursor),
		&h.orgA,
	)
	page2 := decodeAssetList(t, rr)
	if len(page2.Data) != 2 || !page2.HasMore {
		t.Fatalf("second page wrong: %+v", page2)
	}
	if page.Data[0]["id"] == page2.Data[0]["id"] {
		t.Fatalf("pagination did not advance")
	}
}

func TestListAssets_RejectsBadFilters(t *testing.T) {
	h := newAssetsListHarness(t)
	for _, q := range []string{
		"?agent_id=not-a-uuid",
		"?conversation_id=not-a-uuid",
		"?limit=0",
		"?limit=abc",
		"?cursor=not-a-number",
	} {
		rr := h.get(t, q, &h.orgA)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("query %q expected 400, got %d", q, rr.Code)
		}
	}
}

func TestListAssets_MissingOrgContext(t *testing.T) {
	h := newAssetsListHarness(t)
	rr := h.get(t, "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
