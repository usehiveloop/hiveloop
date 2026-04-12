package catalog

import (
	"strings"
	"testing"
)

func TestBuildGraphQLRequest_SimpleQuery(t *testing.T) {
	action := ActionDef{
		ResponseSchema: "issue",
	}
	execConfig := ExecutionConfig{
		GraphQLOperation: "query",
		GraphQLField:     "issue",
		BodyMapping:      map[string]string{"id": "id"},
	}
	params := map[string]any{"id": "abc-123"}
	schemas := map[string]SchemaDefinition{
		"issue": {
			Type: "object",
			Properties: map[string]SchemaPropertyDef{
				"id":          {Type: "string"},
				"title":       {Type: "string"},
				"description": {Type: "string"},
				"priority":    {Type: "number"},
				"assignee":    {Type: "object"},
				"project":     {Type: "object"},
			},
		},
	}

	result := BuildGraphQLRequest(action, execConfig, params, schemas)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	query, ok := result["query"].(string)
	if !ok {
		t.Fatal("expected query string in result")
	}

	if !strings.HasPrefix(query, "query") {
		t.Errorf("query should start with 'query': %s", query)
	}
	if !strings.Contains(query, "issue(") {
		t.Errorf("query should contain field name 'issue(': %s", query)
	}
	if !strings.Contains(query, `id: "abc-123"`) {
		t.Errorf("query should contain argument: %s", query)
	}
	if !strings.Contains(query, "title") {
		t.Errorf("query should select 'title': %s", query)
	}
	if !strings.Contains(query, "assignee { id name }") {
		t.Errorf("query should select nested object with id+name: %s", query)
	}
	t.Logf("Generated query: %s", query)
}

func TestBuildGraphQLRequest_Mutation(t *testing.T) {
	action := ActionDef{
		ResponseSchema: "issue_payload",
	}
	execConfig := ExecutionConfig{
		GraphQLOperation: "mutation",
		GraphQLField:     "issueCreate",
		BodyMapping:      map[string]string{"input": "input"},
	}
	params := map[string]any{
		"input": map[string]any{
			"title":       "New issue",
			"description": "Bug report",
			"teamId":      "team-123",
		},
	}
	schemas := map[string]SchemaDefinition{
		"issue_payload": {
			Type: "object",
			Properties: map[string]SchemaPropertyDef{
				"success": {Type: "boolean"},
				"issue":   {Type: "object"},
			},
		},
	}

	result := BuildGraphQLRequest(action, execConfig, params, schemas)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	query := result["query"].(string)

	if !strings.HasPrefix(query, "mutation") {
		t.Errorf("should be a mutation: %s", query)
	}
	if !strings.Contains(query, "issueCreate(") {
		t.Errorf("should contain mutation field: %s", query)
	}
	if !strings.Contains(query, "success") {
		t.Errorf("should select success field: %s", query)
	}
	t.Logf("Generated mutation: %s", query)
}

func TestBuildGraphQLRequest_NoArgs(t *testing.T) {
	action := ActionDef{
		ResponseSchema: "org",
	}
	execConfig := ExecutionConfig{
		GraphQLOperation: "query",
		GraphQLField:     "organization",
	}
	schemas := map[string]SchemaDefinition{
		"org": {
			Type: "object",
			Properties: map[string]SchemaPropertyDef{
				"id":   {Type: "string"},
				"name": {Type: "string"},
			},
		},
	}

	result := BuildGraphQLRequest(action, execConfig, nil, schemas)
	query := result["query"].(string)

	if strings.Contains(query, "(") && strings.Contains(query, ")") && !strings.Contains(query, "()") {
		// Check no argument parentheses — "organization { id name }" not "organization() { id name }"
		if strings.Contains(query, "organization(") {
			t.Errorf("no-arg query should not have parentheses: %s", query)
		}
	}
	t.Logf("Generated query: %s", query)
}

func TestBuildGraphQLRequest_ArrayFieldsSkipped(t *testing.T) {
	action := ActionDef{
		ResponseSchema: "issue",
	}
	execConfig := ExecutionConfig{
		GraphQLOperation: "query",
		GraphQLField:     "issue",
		BodyMapping:      map[string]string{"id": "id"},
	}
	params := map[string]any{"id": "abc"}
	schemas := map[string]SchemaDefinition{
		"issue": {
			Type: "object",
			Properties: map[string]SchemaPropertyDef{
				"id":     {Type: "string"},
				"title":  {Type: "string"},
				"labels": {Type: "array"}, // should be skipped
			},
		},
	}

	result := BuildGraphQLRequest(action, execConfig, params, schemas)
	query := result["query"].(string)

	if strings.Contains(query, "labels") {
		t.Errorf("array fields should be skipped: %s", query)
	}
}

func TestBuildGraphQLRequest_NotGraphQL(t *testing.T) {
	action := ActionDef{}
	execConfig := ExecutionConfig{
		Method: "GET",
		Path:   "/api/issues",
	}

	result := BuildGraphQLRequest(action, execConfig, nil, nil)
	if result != nil {
		t.Error("non-GraphQL action should return nil")
	}
}

func TestIsGraphQL(t *testing.T) {
	if !IsGraphQL(ExecutionConfig{GraphQLField: "issue"}) {
		t.Error("should be GraphQL when GraphQLField is set")
	}
	if !IsGraphQL(ExecutionConfig{GraphQLOperation: "query"}) {
		t.Error("should be GraphQL when GraphQLOperation is set")
	}
	if IsGraphQL(ExecutionConfig{Method: "GET", Path: "/api"}) {
		t.Error("should not be GraphQL for plain REST")
	}
}

func TestBuildGraphQLRequest_IntegerParam(t *testing.T) {
	action := ActionDef{ResponseSchema: "pr"}
	execConfig := ExecutionConfig{
		GraphQLOperation: "query",
		GraphQLField:     "pullRequest",
		BodyMapping:      map[string]string{"number": "number"},
	}
	params := map[string]any{"number": float64(456)} // JSON numbers are float64
	schemas := map[string]SchemaDefinition{
		"pr": {
			Type: "object",
			Properties: map[string]SchemaPropertyDef{
				"id":     {Type: "string"},
				"number": {Type: "integer"},
			},
		},
	}

	result := BuildGraphQLRequest(action, execConfig, params, schemas)
	query := result["query"].(string)

	if !strings.Contains(query, "number: 456") {
		t.Errorf("integer param should be unquoted: %s", query)
	}
}
