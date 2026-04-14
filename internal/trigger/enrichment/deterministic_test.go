package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/mcpserver"
	"github.com/ziraloop/ziraloop/internal/nango"
)

// ---------------------------------------------------------------------------
// substituteRefsInParams
// ---------------------------------------------------------------------------

func TestSubstituteRefsInParams_FlatParams(t *testing.T) {
	refs := map[string]string{
		"deployment_id": "deploy-abc",
		"service_id":    "svc-123",
	}
	params := map[string]any{
		"deploymentId": "$refs.deployment_id",
		"limit":        500,
	}

	result := substituteRefsInParams(params, refs)

	if result["deploymentId"] != "deploy-abc" {
		t.Errorf("expected deploy-abc, got %v", result["deploymentId"])
	}
	if result["limit"] != 500 {
		t.Errorf("expected 500, got %v", result["limit"])
	}
}

func TestSubstituteRefsInParams_NestedMap(t *testing.T) {
	refs := map[string]string{
		"service_id":     "svc-123",
		"environment_id": "env-456",
	}
	params := map[string]any{
		"input": map[string]any{
			"serviceId":     "$refs.service_id",
			"environmentId": "$refs.environment_id",
		},
		"first": 5,
	}

	result := substituteRefsInParams(params, refs)

	input, ok := result["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected input to be map[string]any, got %T", result["input"])
	}
	if input["serviceId"] != "svc-123" {
		t.Errorf("expected svc-123, got %v", input["serviceId"])
	}
	if input["environmentId"] != "env-456" {
		t.Errorf("expected env-456, got %v", input["environmentId"])
	}
	if result["first"] != 5 {
		t.Errorf("expected 5, got %v", result["first"])
	}
}

func TestSubstituteRefsInParams_Slice(t *testing.T) {
	refs := map[string]string{"id": "abc"}
	params := map[string]any{
		"ids": []any{"$refs.id", "literal", 42},
	}

	result := substituteRefsInParams(params, refs)

	ids, ok := result["ids"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result["ids"])
	}
	if ids[0] != "abc" {
		t.Errorf("expected abc, got %v", ids[0])
	}
	if ids[1] != "literal" {
		t.Errorf("expected literal, got %v", ids[1])
	}
	if ids[2] != 42 {
		t.Errorf("expected 42, got %v", ids[2])
	}
}

func TestSubstituteRefsInParams_MissingRef(t *testing.T) {
	refs := map[string]string{}
	params := map[string]any{
		"deploymentId": "$refs.nonexistent",
		"literal":      "stays",
	}

	result := substituteRefsInParams(params, refs)

	if result["deploymentId"] != "$refs.nonexistent" {
		t.Errorf("expected $refs.nonexistent, got %v", result["deploymentId"])
	}
	if result["literal"] != "stays" {
		t.Errorf("expected stays, got %v", result["literal"])
	}
}

func TestSubstituteRefsInParams_NilParams(t *testing.T) {
	result := substituteRefsInParams(nil, map[string]string{"a": "b"})
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSubstituteRefsInParams_DoesNotMutateOriginal(t *testing.T) {
	refs := map[string]string{"id": "replaced"}
	original := map[string]any{
		"nested": map[string]any{"val": "$refs.id"},
	}

	result := substituteRefsInParams(original, refs)

	nested := original["nested"].(map[string]any)
	if nested["val"] != "$refs.id" {
		t.Errorf("original was mutated: %v", nested["val"])
	}

	resultNested := result["nested"].(map[string]any)
	if resultNested["val"] != "replaced" {
		t.Errorf("expected replaced, got %v", resultNested["val"])
	}
}

// ---------------------------------------------------------------------------
// composeEnrichedMessage
// ---------------------------------------------------------------------------

func TestComposeEnrichedMessage_AllSuccessful(t *testing.T) {
	input := DeterministicEnrichInput{
		Provider:  "railway",
		EventType: "Deployment.failed",
		Refs: map[string]string{
			"service_name": "web.ziraloop.com",
			"branch":       "main",
		},
	}
	results := []enrichmentResult{
		{As: "build_logs", Action: "build_logs", Data: map[string]any{"data": []any{"line1", "line2"}}},
		{As: "service_details", Action: "service", Data: map[string]any{"name": "web", "status": "FAILED"}},
	}

	msg := composeEnrichedMessage(input, results)

	assertContains(t, msg, "## Deployment.failed", "event header")
	assertContains(t, msg, "service_name", "refs key")
	assertContains(t, msg, "web.ziraloop.com", "refs value")
	assertContains(t, msg, "### build_logs", "build_logs section")
	assertContains(t, msg, "### service_details", "service_details section")
	assertContains(t, msg, "```json", "JSON code block")
}

func TestComposeEnrichedMessage_PartialFailure(t *testing.T) {
	input := DeterministicEnrichInput{
		Provider:  "railway",
		EventType: "Deployment.failed",
		Refs:      map[string]string{"branch": "main"},
	}
	results := []enrichmentResult{
		{As: "build_logs", Action: "build_logs", Data: map[string]any{"log": "ok"}},
		{As: "runtime_logs", Action: "deployment_logs", Err: fmt.Errorf("nango proxy: 404 not found")},
	}

	msg := composeEnrichedMessage(input, results)

	assertContains(t, msg, "### build_logs", "successful section")
	assertContains(t, msg, "### runtime_logs", "failed section header")
	assertContains(t, msg, "> **Error:**", "error annotation")
	assertContains(t, msg, "404 not found", "error detail")
}

// ---------------------------------------------------------------------------
// Full enrichment pipeline test with mock Nango
// ---------------------------------------------------------------------------

// capturedRequest records what was sent to the Nango proxy.
type capturedRequest struct {
	Method         string
	Path           string
	ProviderCfgKey string
	ConnectionID   string
	Body           map[string]any
}

// TestEnrichActions_RailwayDeploymentFailed tests the full enrichment pipeline
// by calling ExecuteAction for each enrichment action (same code path as
// DeterministicEnricher.Enrich) against a mock Nango server.
//
// This tests: trigger catalog lookup → ref substitution → action resolution →
// Nango proxy call with correct payload shape.
func TestEnrichActions_RailwayDeploymentFailed(t *testing.T) {
	// Capture all Nango proxy requests.
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

	// Look up the trigger definition from the real catalog.
	triggerDef, ok := actionsCatalog.GetTrigger("railway", "Deployment.failed")
	if !ok {
		t.Fatal("Deployment.failed trigger not found in catalog")
	}
	if len(triggerDef.Enrichment) == 0 {
		t.Fatal("Deployment.failed trigger has no enrichment actions")
	}

	// These are the refs extracted from the real Railway webhook.
	refs := map[string]string{
		"deployment_id":    "deploy-0df056be",
		"project_id":       "proj-55776e03",
		"project_name":     "ziraloop.com",
		"service_id":       "svc-b6c22e03",
		"service_name":     "web.ziraloop.com",
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

	// Execute each enrichment action (same logic as DeterministicEnricher.Enrich).
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

	// --- Verify: all 4 enrichment actions were executed ---

	if len(captured) != 4 {
		t.Fatalf("expected 4 Nango requests, got %d", len(captured))
	}

	// --- Verify: all requests use correct Nango credentials ---

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
		if request.Path != "/proxy/graphql" {
			t.Errorf("request %d: expected /proxy/graphql, got %s", index, request.Path)
		}
	}

	// --- Verify: payload shapes have substituted refs ---

	// Build action→body map. Actions run in parallel so order isn't guaranteed.
	actionBodies := identifyActionBodies(captured)

	// build_logs: deploymentId from $refs.deployment_id, limit=500
	assertActionBody(t, actionBodies, "build_logs", map[string]any{
		"deploymentId": "deploy-0df056be",
		"limit":        float64(500),
	})

	// deployment_logs: deploymentId from $refs.deployment_id, limit=200
	assertActionBody(t, actionBodies, "deployment_logs", map[string]any{
		"deploymentId": "deploy-0df056be",
		"limit":        float64(200),
	})

	// service: id from $refs.service_id
	assertActionBody(t, actionBodies, "service", map[string]any{
		"id": "svc-b6c22e03",
	})

	// deployments: nested input with refs, first=5
	deploymentsBody, ok := actionBodies["deployments"]
	if !ok {
		t.Fatal("deployments action was not captured")
	}
	inputMap, isMap := deploymentsBody["input"].(map[string]any)
	if !isMap {
		t.Fatalf("deployments: expected input to be map, got %T", deploymentsBody["input"])
	}
	if inputMap["serviceId"] != "svc-b6c22e03" {
		t.Errorf("deployments: expected input.serviceId=svc-b6c22e03, got %v", inputMap["serviceId"])
	}
	if inputMap["environmentId"] != "env-3c177170" {
		t.Errorf("deployments: expected input.environmentId=env-3c177170, got %v", inputMap["environmentId"])
	}
	if deploymentsBody["first"] != float64(5) {
		t.Errorf("deployments: expected first=5, got %v", deploymentsBody["first"])
	}

	// --- Verify: all actions succeeded ---

	for _, result := range results {
		if result.Err != nil {
			t.Errorf("action %q (%s) failed: %v", result.As, result.Action, result.Err)
		}
	}

	// --- Verify: composed message ---

	composedMessage := composeEnrichedMessage(DeterministicEnrichInput{
		Provider:  "railway",
		EventType: "Deployment.failed",
		Refs:      refs,
	}, results)

	assertContains(t, composedMessage, "## Deployment.failed", "event header")
	assertContains(t, composedMessage, "web.ziraloop.com", "service name in refs")
	assertContains(t, composedMessage, "mock-result", "API result data")

	// Verify all 4 sections are present.
	for _, label := range []string{"build_logs", "runtime_logs", "service_details", "recent_deployments"} {
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// identifyActionBodies maps captured Nango requests to action names by
// inspecting the body shape. This handles the fact that parallel execution
// means captured order is nondeterministic.
func identifyActionBodies(captured []capturedRequest) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for _, request := range captured {
		if _, hasDeplID := request.Body["deploymentId"]; hasDeplID {
			limit, hasLimit := request.Body["limit"]
			if hasLimit {
				limitNum, isFloat := limit.(float64)
				if isFloat && limitNum == 500 {
					result["build_logs"] = request.Body
				} else if isFloat && limitNum == 200 {
					result["deployment_logs"] = request.Body
				}
			}
		}
		if _, hasID := request.Body["id"]; hasID {
			result["service"] = request.Body
		}
		if _, hasInput := request.Body["input"]; hasInput {
			result["deployments"] = request.Body
		}
	}
	return result
}

func assertActionBody(t *testing.T, actionBodies map[string]map[string]any, actionName string, expected map[string]any) {
	t.Helper()
	body, ok := actionBodies[actionName]
	if !ok {
		t.Errorf("%s action was not captured", actionName)
		return
	}
	for key, expectedValue := range expected {
		if body[key] != expectedValue {
			t.Errorf("%s: expected %s=%v, got %v", actionName, key, expectedValue, body[key])
		}
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
