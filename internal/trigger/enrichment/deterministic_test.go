package enrichment

import (
	"testing"
)

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
