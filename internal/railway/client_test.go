package railway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCreateServiceFromImageUsesGraphQLTransport(t *testing.T) {
	var capturedAuth string
	var captured struct {
		OperationName string         `json:"operationName"`
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"serviceCreate":{"id":"svc_123","name":"warm-1"}}}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Config{Endpoint: server.URL, Token: "railway-token", HTTP: server.Client()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	service, err := client.CreateServiceFromImage(t.Context(), CreateServiceInput{
		Name:          "warm-1",
		ProjectID:     "project",
		EnvironmentID: "env",
		Image:         "runtime:test",
		Variables:     map[string]string{"HIVY_RUNTIME_SECRET": "bootstrap"},
	})
	if err != nil {
		t.Fatalf("CreateServiceFromImage: %v", err)
	}
	if service.ID != "svc_123" || service.Name != "warm-1" {
		t.Fatalf("service = %#v", service)
	}
	if capturedAuth != "Bearer railway-token" {
		t.Fatalf("Authorization = %q", capturedAuth)
	}
	if captured.OperationName != "CreateRailwaySandboxService" {
		t.Fatalf("operation = %q", captured.OperationName)
	}
	if captured.Variables["projectId"] != "project" || captured.Variables["environmentId"] != "env" {
		t.Fatalf("variables = %#v", captured.Variables)
	}
}
