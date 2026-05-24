package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/spider"
)

func TestSpiderCrawl_RecordsUsage(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "Page 1", URL: "https://example.com/1", StatusCode: 200},
			{Content: "Page 2", URL: "https://example.com/2", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	harness.usageWriter.Shutdown(t.Context())

	time.Sleep(100 * time.Millisecond)

	var usages []model.ToolUsage
	if err := harness.db.Where("org_id = ? AND tool_name = ?", harness.orgID, "crawl").Find(&usages).Error; err != nil {
		t.Fatalf("query tool_usages: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(usages))
	}

	usage := usages[0]
	if usage.OrgID != harness.orgID {
		t.Fatalf("expected org_id %s, got %s", harness.orgID, usage.OrgID)
	}
	if usage.ToolName != "crawl" {
		t.Fatalf("expected tool_name 'crawl', got %q", usage.ToolName)
	}
	if usage.Input != "https://example.com" {
		t.Fatalf("expected input 'https://example.com', got %q", usage.Input)
	}
	if usage.PagesReturned != 2 {
		t.Fatalf("expected pages_returned 2, got %d", usage.PagesReturned)
	}
	if usage.Status != "success" {
		t.Fatalf("expected status 'success', got %q", usage.Status)
	}
	if usage.TokenJTI != harness.tokenJTI {
		t.Fatalf("expected token_jti %q, got %q", harness.tokenJTI, usage.TokenJTI)
	}
	if usage.EmployeeID == "" {
		t.Fatal("expected employee_id to be set from token meta")
	}
}

func TestSpiderCrawl_RecordsErrorUsage(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", recorder.Code)
	}

	harness.usageWriter.Shutdown(t.Context())
	time.Sleep(100 * time.Millisecond)

	var usages []model.ToolUsage
	if err := harness.db.Where("org_id = ? AND tool_name = ?", harness.orgID, "crawl").Find(&usages).Error; err != nil {
		t.Fatalf("query tool_usages: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(usages))
	}

	if usages[0].Status != "error" {
		t.Fatalf("expected status 'error', got %q", usages[0].Status)
	}
	if usages[0].ErrorMessage == "" {
		t.Fatal("expected error_message to be set")
	}
}
