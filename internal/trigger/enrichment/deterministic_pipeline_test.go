package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/mcpserver"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

type capturedRequest struct {
	Method         string
	Path           string
	ProviderCfgKey string
	ConnectionID   string
	Body           map[string]any
}

func TestEnrichActions_RailwayDeploymentFailed(t *testing.T) {
	var captured []capturedRequest
	var mu sync.Mutex

	nangoServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bodyBytes, _ := io.ReadAll(request.Body)
		var bodyMap map[string]any
		json.Unmarshal(bodyBytes, &bodyMap)

		mu.Lock()
		captured = append(captured, capturedRequest{
			Method:         request.Method,
			Path:           request.URL.Path,
			ProviderCfgKey: request.Header.Get("Provider-Config-Key"),
			ConnectionID:   request.Header.Get("Connection-Id"),
			Body:           bodyMap,
		})
		mu.Unlock()

		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{"data": "mock-result"})
	}))
	defer nangoServer.Close()

	nangoClient := nango.NewClient(nangoServer.URL, "test-secret")
	actionsCatalog := catalog.Global()

	triggerDef, ok := actionsCatalog.GetTrigger("railway", "Deployment.failed")
	if !ok {
		t.Fatal("Deployment.failed trigger not found in catalog")
	}
	if len(triggerDef.Enrichment) == 0 {
		t.Fatal("Deployment.failed trigger has no enrichment actions")
	}

	refs := map[string]string{
		"deployment_id":    "deploy-0df056be",
		"project_id":       "proj-55776e03",
		"project_name":     "hiveloop.com",
		"service_id":       "svc-b6c22e03",
		"service_name":     "web.hiveloop.com",
		"environment_id":   "env-3c177170",
		"environment_name": "production",
		"workspace_id":     "ws-71ad85f8",
		"commit_hash":      "c7e74fa",
		"branch":           "main",
		"commit_author":    "kimiduck",
	}

	orgID := uuid.New()
	providerCfgKey := orgID.String() + "_railway-prod"
	nangoConnID := "nango-conn-abc"

	var results []enrichmentResult
	var waitGroup sync.WaitGroup

	for _, enrichAction := range triggerDef.Enrichment {
		waitGroup.Add(1)
		go func(action catalog.EnrichmentAction) {
			defer waitGroup.Done()

			params := substituteRefsInParams(action.Params, refs)
			actionDef, actionOK := actionsCatalog.GetAction("railway", action.Action)
			if !actionOK {
				mu.Lock()
				results = append(results, enrichmentResult{
					As: action.As, Action: action.Action,
					Err: fmt.Errorf("action %q not found", action.Action),
				})
				mu.Unlock()
				return
			}

			data, err := mcpserver.ExecuteAction(
				context.Background(),
				nangoClient,
				"railway",
				providerCfgKey,
				nangoConnID,
				actionDef,
				params,
				nil,
			)

			mu.Lock()
			results = append(results, enrichmentResult{
				As: action.As, Action: action.Action, Data: data, Err: err,
			})
			mu.Unlock()
		}(enrichAction)
	}

	waitGroup.Wait()

	if len(captured) != 4 {
		t.Fatalf("expected 4 Nango requests, got %d", len(captured))
	}

	for index, request := range captured {
		if request.ProviderCfgKey != providerCfgKey {
			t.Errorf("request %d: expected provider_cfg_key %q, got %q", index, providerCfgKey, request.ProviderCfgKey)
		}
		if request.ConnectionID != nangoConnID {
			t.Errorf("request %d: expected connection_id %q, got %q", index, nangoConnID, request.ConnectionID)
		}
		if request.Method != "POST" {
			t.Errorf("request %d: expected POST, got %s", index, request.Method)
		}
		if request.Path != "/proxy/graphql/v2" {
			t.Errorf("request %d: expected /proxy/graphql/v2, got %s", index, request.Path)
		}
		query, hasQuery := request.Body["query"].(string)
		if !hasQuery || query == "" {
			t.Errorf("request %d: expected GraphQL query in body, got %v", index, request.Body)
		}
	}

	for index, request := range captured {
		query, hasQuery := request.Body["query"].(string)
		if !hasQuery {
			t.Errorf("request %d: expected query in body", index)
			continue
		}
		if !strings.Contains(query, "query(") {
			t.Errorf("request %d: expected parameterized query, got %q", index, query)
		}
		variables, hasVars := request.Body["variables"].(map[string]any)
		if !hasVars {
			t.Errorf("request %d: expected variables in body", index)
			continue
		}
		if len(variables) == 0 {
			t.Errorf("request %d: variables map is empty", index)
		}
	}

	var allVariables []map[string]any
	for _, request := range captured {
		if vars, ok := request.Body["variables"].(map[string]any); ok {
			allVariables = append(allVariables, vars)
		}
	}

	foundDeploymentId := false
	foundProjectId := false
	foundEnvironmentId := false
	for _, vars := range allVariables {
		if vars["deploymentId"] == "deploy-0df056be" {
			foundDeploymentId = true
		}
		if vars["id"] == "proj-55776e03" {
			foundProjectId = true
		}
		if inputMap, ok := vars["input"].(map[string]any); ok {
			if inputMap["serviceId"] == "svc-b6c22e03" && inputMap["environmentId"] == "env-3c177170" {
				foundEnvironmentId = true
			}
		}
	}
	if !foundDeploymentId {
		t.Error("no request had variables.deploymentId = deploy-0df056be")
	}
	if !foundProjectId {
		t.Error("no request had variables.id = proj-55776e03")
	}
	if !foundEnvironmentId {
		t.Error("no request had variables.input.serviceId + environmentId")
	}

	for _, result := range results {
		if result.Err != nil {
			t.Errorf("action %q (%s) failed: %v", result.As, result.Action, result.Err)
		}
	}

	composedMessage := composeEnrichedMessage(DeterministicEnrichInput{
		Provider:  "railway",
		EventType: "Deployment.failed",
		Refs:      refs,
	}, results)

	assertContains(t, composedMessage, "## Deployment.failed", "event header")
	assertContains(t, composedMessage, "web.hiveloop.com", "service name in refs")
	assertContains(t, composedMessage, "mock-result", "API result data")

	for _, label := range []string{"build_logs", "runtime_logs", "project_topology", "recent_deployments"} {
		assertContains(t, composedMessage, "### "+label, label+" section")
	}
}

func TestEnrichActions_UnknownProvider(t *testing.T) {
	actionsCatalog := catalog.Global()

	triggerDef, ok := actionsCatalog.GetTrigger("nonexistent-provider", "some.event")
	if ok {
		t.Fatalf("expected trigger not found, got %v", triggerDef)
	}
}

func assertContains(t *testing.T, haystack, needle, description string) {
	t.Helper()
	for index := 0; index <= len(haystack)-len(needle); index++ {
		if haystack[index:index+len(needle)] == needle {
			return
		}
	}
	t.Errorf("missing %s: expected %q in output", description, needle)
}
