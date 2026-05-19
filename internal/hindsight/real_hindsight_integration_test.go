package hindsight

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestRealHindsightRetainRecallOrgTeam(t *testing.T) {
	if os.Getenv("HINDSIGHT_INTEGRATION") != "1" {
		t.Skip("set HINDSIGHT_INTEGRATION=1 and HINDSIGHT_API_URL to run against a real Hindsight service")
	}
	baseURL := os.Getenv("HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := NewClient(baseURL)
	orgID := uuid.New()
	bankID := OrgBankID(orgID)
	if err := client.ConfigureBank(ctx, bankID, DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("ConfigureBank: %v", err)
	}

	fact := "The Platform team requires rollback notes in every deployment plan. marker=" + uuid.NewString()
	_, err := client.Retain(ctx, bankID, &RetainRequest{
		Items: []RetainItem{{
			Content:    fact,
			Context:    "Integration test durable team policy",
			DocumentID: "integration-test:" + uuid.NewString(),
			Tags: []string{
				"company:" + orgID.String(),
				"source:manual",
				"visibility:company",
				"memory_type:policy",
			},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Metadata:  map[string]string{"test": "real-hindsight"},
			ObservationScopes: [][]string{
				{"company:" + orgID.String()},
			},
		}},
		Async: false,
	})
	if err != nil {
		t.Fatalf("Retain: %v", err)
	}

	resp, err := client.Recall(ctx, bankID, &RecallRequest{
		Query:  "What deployment policy does the Platform team follow?",
		Budget: "mid",
		TagGroups: []any{map[string]any{
			"tags":  []string{"company:" + orgID.String()},
			"match": "all_strict",
		}},
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatalf("expected at least one recalled memory")
	}
	if !strings.Contains(strings.ToLower(toJSONForTest(resp.Results)), "rollback") {
		t.Fatalf("recall results did not include retained policy: %#v", resp.Results)
	}
}

func TestRealHindsightForgetDocument(t *testing.T) {
	if os.Getenv("HINDSIGHT_INTEGRATION") != "1" {
		t.Skip("set HINDSIGHT_INTEGRATION=1 and HINDSIGHT_API_URL to run against a real Hindsight service")
	}
	baseURL := os.Getenv("HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := NewClient(baseURL)
	orgID := uuid.New()
	bankID := OrgBankID(orgID)
	if err := client.ConfigureBank(ctx, bankID, DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("ConfigureBank: %v", err)
	}

	marker := "forget-marker-" + uuid.NewString()
	documentID := "integration-forget:" + uuid.NewString()
	_, err := client.Retain(ctx, bankID, &RetainRequest{
		Items: []RetainItem{{
			Content:    "The Platform team requires blue envelopes before deploys. " + marker,
			Context:    "Integration test memory that should be forgotten",
			DocumentID: documentID,
			Tags: []string{
				"company:" + orgID.String(),
				"source:manual",
				"visibility:company",
				"memory_type:policy",
			},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}},
		Async: false,
	})
	if err != nil {
		t.Fatalf("Retain: %v", err)
	}
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID}
	result := callRealMemoryTool(t, ctx, agent, client, "memory_forget", map[string]any{
		"document_id": documentID,
		"reason":      "real Hindsight integration test cleanup",
	})
	if result.IsError {
		t.Fatalf("memory_forget returned error: %s", realToolText(t, result))
	}
	var payload memoryForgetToolResponse
	decodeRealToolJSON(t, result, &payload)
	if !payload.Deleted || payload.DocumentID != documentID {
		t.Fatalf("unexpected memory_forget payload: %#v", payload)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Recall(ctx, bankID, &RecallRequest{
			Query:  "What does the Platform team require before deploys? " + marker,
			Budget: "mid",
			TagGroups: []any{map[string]any{
				"tags":  []string{"company:" + orgID.String()},
				"match": "all_strict",
			}},
		})
		if err != nil {
			t.Fatalf("Recall after delete: %v", err)
		}
		if !strings.Contains(strings.ToLower(toJSONForTest(resp.Results)), strings.ToLower(marker)) {
			return
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("deleted document marker still appeared in recall")
}

func TestRealHindsightMemoryRetainToolReturnsDocumentID(t *testing.T) {
	if os.Getenv("HINDSIGHT_INTEGRATION") != "1" {
		t.Skip("set HINDSIGHT_INTEGRATION=1 and HINDSIGHT_API_URL to run against a real Hindsight service")
	}
	baseURL := os.Getenv("HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := NewClient(baseURL)
	orgID := uuid.New()
	agentID := uuid.New()
	bankID := OrgBankID(orgID)
	if err := client.ConfigureBank(ctx, bankID, DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("ConfigureBank: %v", err)
	}

	result := callRealMemoryTool(t, ctx, &model.Agent{ID: agentID, OrgID: &orgID}, client, "memory_retain", map[string]any{
		"content":     "Integration test memory_retain document ID marker " + uuid.NewString(),
		"context":     "Real Hindsight integration test",
		"memory_type": "preference",
	})
	if result.IsError {
		t.Fatalf("memory_retain returned error: %s", realToolText(t, result))
	}
	var payload memoryRetainToolResponse
	decodeRealToolJSON(t, result, &payload)
	if !strings.HasPrefix(payload.DocumentID, "manual:"+agentID.String()+":") {
		t.Fatalf("document_id = %q", payload.DocumentID)
	}
	if payload.BankID != bankID {
		t.Fatalf("bank_id = %q, want %q", payload.BankID, bankID)
	}
}

func toJSONForTest(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func callRealMemoryTool(t *testing.T, ctx context.Context, agent *model.Agent, client *Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "real-memory-test", Version: "v1"}, nil)
	AddMemoryTools(server, agent, client, nil, nil)
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "real-memory-test-client", Version: "v1"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call tool %s: %v", name, err)
	}
	return result
}

func decodeRealToolJSON(t *testing.T, result *mcp.CallToolResult, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(realToolText(t, result)), out); err != nil {
		t.Fatalf("decode tool json: %v; text=%s", err, realToolText(t, result))
	}
}

func realToolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatalf("tool result has no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("tool result content type = %T", result.Content[0])
	}
	return text.Text
}
