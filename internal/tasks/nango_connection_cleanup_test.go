package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/nango"
)

func TestAgentProfileNangoCleanupHandlerDeletesConnections(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s", r.Method)
		}
		paths = append(paths, r.URL.RequestURI())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	agentID := uuid.New()
	task, err := NewAgentProfileNangoCleanupTask(agentID, []NangoConnectionDeleteTarget{
		{ConnectionID: "conn-1", ProviderConfigKey: "in_github", ProfileID: uuid.New(), Provider: "github"},
		{ConnectionID: "conn-2", ProviderConfigKey: "in_slack", ProfileID: uuid.New(), Provider: "slack"},
	})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}

	handler := NewAgentProfileNangoCleanupHandler(nango.NewClient(server.URL, "test-secret"))
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %#v", paths)
	}
	if paths[0] != "/connection/conn-1?provider_config_key=in_github" {
		t.Fatalf("path[0] = %q", paths[0])
	}
	if paths[1] != "/connection/conn-2?provider_config_key=in_slack" {
		t.Fatalf("path[1] = %q", paths[1])
	}
}

func TestNewAgentProfileNangoCleanupTaskPayload(t *testing.T) {
	agentID := uuid.New()
	target := NangoConnectionDeleteTarget{ConnectionID: "conn", ProviderConfigKey: "in_github", ProfileID: uuid.New(), Provider: "github"}
	task, err := NewAgentProfileNangoCleanupTask(agentID, []NangoConnectionDeleteTarget{target})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if task.Type() != TypeAgentProfileNangoCleanup {
		t.Fatalf("task type = %q", task.Type())
	}
	var payload AgentProfileNangoCleanupPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.AgentID != agentID {
		t.Fatalf("agent id = %s", payload.AgentID)
	}
	if len(payload.Connections) != 1 || payload.Connections[0] != target {
		t.Fatalf("connections = %#v", payload.Connections)
	}
}
