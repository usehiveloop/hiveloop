package evals

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJudgeClassifyFinalTextUsesProxyAndJSONSchema(t *testing.T) {
	var gotAuth string
	var gotModel string
	var gotFormat map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/proxy/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel, _ = body["model"].(string)
		gotFormat, _ = body["response_format"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"behavior":"clarify","confidence":0.98,"reason":"asks for missing context"}`,
				},
			}},
		})
	}))
	defer srv.Close()

	judgement, err := NewJudge("deepseek-v4-flash").ClassifyFinalText(t.Context(), srv.URL+"/v1/proxy/v1", "ptok_test", Case{
		Message:          "Can you help with the thing?",
		ExpectedBehavior: BehaviorClarify,
	}, "Can you remind me what the thing is?")
	if err != nil {
		t.Fatalf("ClassifyFinalText: %v", err)
	}
	if judgement.Behavior != BehaviorClarify || judgement.Model != "deepseek-v4-flash" {
		t.Fatalf("judgement = %#v", judgement)
	}
	if gotAuth != "Bearer ptok_test" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotModel != "deepseek-v4-flash" {
		t.Fatalf("model = %q", gotModel)
	}
	if gotFormat["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", gotFormat)
	}
}
