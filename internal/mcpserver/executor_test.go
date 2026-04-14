package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/nango"
)

// capturedBody records the JSON body sent to the Nango proxy.
type capturedBody struct {
	Query string `json:"query"`
}

func TestExecuteAction_GraphQL_BuildLogs(t *testing.T) {
	var captured capturedBody

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bodyBytes, _ := io.ReadAll(request.Body)
		json.Unmarshal(bodyBytes, &captured)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"buildLogs": []any{"line1"}}})
	}))
	defer server.Close()

	nangoClient := nango.NewClient(server.URL, "test")

	action := &catalog.ActionDef{
		DisplayName:    "Build Logs",
		Access:         "read",
		ResourceType:   "deployment",
		ResponseSchema: "log",
		Execution: &catalog.ExecutionConfig{
			Method:           "POST",
			Path:             "/graphql/v2",
			BodyMapping:      map[string]string{"deploymentId": "deploymentId", "limit": "limit"},
			GraphQLOperation: "query",
			GraphQLField:     "buildLogs",
		},
	}

	schemas := map[string]catalog.SchemaDefinition{
		"log": {
			Properties: map[string]catalog.SchemaPropertyDef{
				"message":   {Type: "string"},
				"severity":  {Type: "string"},
				"timestamp": {Type: "string"},
			},
		},
	}

	params := map[string]any{
		"deploymentId": "deploy-abc",
		"limit":        500,
	}

	_, err := ExecuteAction(
		context.Background(),
		nangoClient,
		"railway",
		"in_railway-test",
		"nango-conn",
		action,
		params,
		nil,
		schemas,
	)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	// Verify GraphQL query was built correctly.
	if captured.Query == "" {
		t.Fatal("expected GraphQL query in body, got empty")
	}
	if !strings.Contains(captured.Query, "query {") {
		t.Errorf("expected 'query {' in query, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, "buildLogs(") {
		t.Errorf("expected 'buildLogs(' in query, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, `deploymentId: "deploy-abc"`) {
		t.Errorf("expected deploymentId arg, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, "limit: 500") {
		t.Errorf("expected limit arg, got %q", captured.Query)
	}
	// Selection set should include schema fields.
	if !strings.Contains(captured.Query, "message") {
		t.Errorf("expected 'message' in selection set, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, "timestamp") {
		t.Errorf("expected 'timestamp' in selection set, got %q", captured.Query)
	}
}

func TestExecuteAction_GraphQL_Service(t *testing.T) {
	var captured capturedBody

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bodyBytes, _ := io.ReadAll(request.Body)
		json.Unmarshal(bodyBytes, &captured)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"service": map[string]any{"name": "web"}}})
	}))
	defer server.Close()

	nangoClient := nango.NewClient(server.URL, "test")

	action := &catalog.ActionDef{
		DisplayName:    "Service",
		Access:         "read",
		ResponseSchema: "service",
		Execution: &catalog.ExecutionConfig{
			Method:           "POST",
			Path:             "/graphql/v2",
			BodyMapping:      map[string]string{"id": "id"},
			GraphQLOperation: "query",
			GraphQLField:     "service",
		},
	}

	schemas := map[string]catalog.SchemaDefinition{
		"service": {
			Properties: map[string]catalog.SchemaPropertyDef{
				"id":   {Type: "string"},
				"name": {Type: "string"},
				"icon": {Type: "string"},
			},
		},
	}

	_, err := ExecuteAction(
		context.Background(),
		nangoClient,
		"railway",
		"in_railway-test",
		"nango-conn",
		action,
		map[string]any{"id": "svc-123"},
		nil,
		schemas,
	)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if !strings.Contains(captured.Query, "query {") {
		t.Errorf("expected query operation, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, `service(id: "svc-123")`) {
		t.Errorf("expected service(id: ...) in query, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, "name") {
		t.Errorf("expected 'name' in selection set, got %q", captured.Query)
	}
}

func TestExecuteAction_GraphQL_Deployments_NestedInput(t *testing.T) {
	var captured capturedBody

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bodyBytes, _ := io.ReadAll(request.Body)
		json.Unmarshal(bodyBytes, &captured)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"deployments": map[string]any{"edges": []any{}}}})
	}))
	defer server.Close()

	nangoClient := nango.NewClient(server.URL, "test")

	action := &catalog.ActionDef{
		DisplayName:    "Deployments",
		Access:         "read",
		ResponseSchema: "query_deployments_connection",
		Execution: &catalog.ExecutionConfig{
			Method:           "POST",
			Path:             "/graphql/v2",
			BodyMapping:      map[string]string{"first": "first", "input": "input"},
			GraphQLOperation: "query",
			GraphQLField:     "deployments",
		},
	}

	params := map[string]any{
		"first": 5,
		"input": map[string]any{
			"serviceId":     "svc-abc",
			"environmentId": "env-def",
		},
	}

	_, err := ExecuteAction(
		context.Background(),
		nangoClient,
		"railway",
		"in_railway-test",
		"nango-conn",
		action,
		params,
		nil,
	)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if !strings.Contains(captured.Query, "deployments(") {
		t.Errorf("expected 'deployments(' in query, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, "first: 5") {
		t.Errorf("expected 'first: 5' in query, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, `serviceId: "svc-abc"`) {
		t.Errorf("expected nested serviceId in query, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, `environmentId: "env-def"`) {
		t.Errorf("expected nested environmentId in query, got %q", captured.Query)
	}
}

func TestExecuteAction_GraphQL_Mutation(t *testing.T) {
	var captured capturedBody

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bodyBytes, _ := io.ReadAll(request.Body)
		json.Unmarshal(bodyBytes, &captured)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"deploymentRestart": true}})
	}))
	defer server.Close()

	nangoClient := nango.NewClient(server.URL, "test")

	action := &catalog.ActionDef{
		DisplayName: "Deployment Restart",
		Access:      "write",
		Execution: &catalog.ExecutionConfig{
			Method:           "POST",
			Path:             "/graphql/v2",
			BodyMapping:      map[string]string{"id": "id"},
			GraphQLOperation: "mutation",
			GraphQLField:     "deploymentRestart",
		},
	}

	_, err := ExecuteAction(
		context.Background(),
		nangoClient,
		"railway",
		"in_railway-test",
		"nango-conn",
		action,
		map[string]any{"id": "deploy-xyz"},
		nil,
	)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if !strings.Contains(captured.Query, "mutation {") {
		t.Errorf("expected 'mutation {' in query, got %q", captured.Query)
	}
	if !strings.Contains(captured.Query, `deploymentRestart(id: "deploy-xyz")`) {
		t.Errorf("expected deploymentRestart with id arg, got %q", captured.Query)
	}
}

func TestExecuteAction_REST_NoGraphQL(t *testing.T) {
	var capturedBodyMap map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bodyBytes, _ := io.ReadAll(request.Body)
		json.Unmarshal(bodyBytes, &capturedBodyMap)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	nangoClient := nango.NewClient(server.URL, "test")

	action := &catalog.ActionDef{
		DisplayName: "List Items",
		Access:      "read",
		Execution: &catalog.ExecutionConfig{
			Method:      "POST",
			Path:        "/api/items",
			BodyMapping: map[string]string{"limit": "limit"},
		},
	}

	_, err := ExecuteAction(
		context.Background(),
		nangoClient,
		"some-provider",
		"cfg-key",
		"conn-id",
		action,
		map[string]any{"limit": 10},
		nil,
	)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	// Should NOT have a "query" key — it's REST, not GraphQL.
	if _, hasQuery := capturedBodyMap["query"]; hasQuery {
		t.Errorf("REST action should not send GraphQL query, got %v", capturedBodyMap)
	}
	if capturedBodyMap["limit"] != float64(10) {
		t.Errorf("expected limit=10 in body, got %v", capturedBodyMap["limit"])
	}
}
