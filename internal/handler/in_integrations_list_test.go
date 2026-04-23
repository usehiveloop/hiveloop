package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestInIntegrationHandler_List_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	providers := []string{"github", "slack", "notion"}
	for _, p := range providers {
		integ := model.InIntegration{
			ID:          uuid.New(),
			UniqueKey:   fmt.Sprintf("%s-%s", p, uuid.New().String()[:8]),
			Provider:    p,
			DisplayName: p + " test",
		}
		if err := h.db.Create(&integ).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations", nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) < 3 {
		t.Fatalf("expected at least 3 integrations, got %d", len(page.Data))
	}
}

func TestInIntegrationHandler_List_ExcludesDeleted(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	integ := createTestInIntegration(t, h.db, "github")
	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations", nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["id"] == integ.ID.String() {
			t.Fatal("deleted integration should not appear in list")
		}
	}
}

func TestInIntegrationHandler_List_Pagination(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	for i := 0; i < 5; i++ {
		provider := fmt.Sprintf("provider-%d-%s", i, uuid.New().String()[:8])
		integ := model.InIntegration{
			ID:          uuid.New(),
			UniqueKey:   fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
			Provider:    provider,
			DisplayName: provider,
		}
		if err := h.db.Create(&integ).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
		time.Sleep(time.Millisecond)
	}

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations?limit=2", nil, &user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page1 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	json.NewDecoder(rr.Body).Decode(&page1)
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("expected has_more=true")
	}
	if page1.NextCursor == nil {
		t.Fatal("expected next_cursor to be present")
	}

	rr2 := h.doRequest(t, http.MethodGet, "/v1/in/integrations?limit=2&cursor="+*page1.NextCursor, nil, &user)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}

	var page2 struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr2.Body).Decode(&page2)
	if len(page2.Data) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2.Data))
	}
}

func TestInIntegrationHandler_ListAvailable_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp []map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) < 1 {
		t.Fatal("expected at least 1 available integration")
	}

	for _, item := range resp {
		if _, exists := item["nango_config"]; exists {
			t.Fatal("nango_config should not be in available response")
		}
		if _, exists := item["unique_key"]; exists {
			t.Fatal("unique_key should not be in available response")
		}
	}
}

func TestInIntegrationHandler_ListAvailable_ExcludesDeleted(t *testing.T) {
	h := newInIntegHarness(t, nil)
	integ := createTestInIntegration(t, h.db, "github")

	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	for _, item := range resp {
		if item["id"] == integ.ID.String() {
			t.Fatal("deleted integration should not appear in available list")
		}
	}
}
