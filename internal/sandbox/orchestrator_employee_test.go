package sandbox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

func TestCreateEmployeeSandbox_ClonesSelectedGitHubProfileRepositories(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	provider.endpointOverride = employeeRuntimeEndpoint(t)
	orch.cfg.EmployeeSandboxBaseImagePrefix = "employee-snapshot-v1"

	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)
	agent.IsEmployee = true
	agent.Resources = model.JSON{
		"github": map[string]any{
			"repository": []any{
				map[string]any{"id": "123456", "name": "api", "full_name": "octo-org/api"},
				map[string]any{"id": "789012", "name": "web", "full_name": "octo-org/web"},
			},
		},
	}
	if err := db.Save(&agent).Error; err != nil {
		t.Fatalf("save employee: %v", err)
	}

	var commands []string
	provider.executeCommandFn = func(_ context.Context, _ string, command string) (string, error) {
		commands = append(commands, command)
		return "", nil
	}

	sb, err := orch.CreateEmployeeSandbox(context.Background(), &agent, employeeStartupSecrets())
	if err != nil {
		t.Fatalf("CreateEmployeeSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	want := []string{
		"mkdir -p /workspace/repos",
		"git clone --depth=1 https://github.com/octo-org/api.git /workspace/repos/api",
		"git clone --depth=1 https://github.com/octo-org/web.git /workspace/repos/web",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
	for _, command := range commands {
		if strings.Contains(command, "123456") || strings.Contains(command, "789012") {
			t.Fatalf("clone command used numeric GitHub repository id: %q", command)
		}
	}
}

func TestCreateEmployeeSandbox_NoGitHubSelectionSkipsRepositoryClone(t *testing.T) {
	tests := []struct {
		name      string
		resources model.JSON
	}{
		{name: "no resources"},
		{name: "empty selection", resources: model.JSON{"github": map[string]any{"repository": []any{}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, provider, db := setupOrchestrator(t)
			provider.endpointOverride = employeeRuntimeEndpoint(t)
			orch.cfg.EmployeeSandboxBaseImagePrefix = "employee-snapshot-v1"

			org := createTestOrg(t, db)
			cred := createTestCred(t, db, org.ID)
			agent := createTestAgent(t, db, org.ID, cred.ID)
			agent.IsEmployee = true
			if err := db.Save(&agent).Error; err != nil {
				t.Fatalf("save employee: %v", err)
			}
			agent.Resources = tt.resources
			if err := db.Save(&agent).Error; err != nil {
				t.Fatalf("save employee resources: %v", err)
			}

			var commands []string
			provider.executeCommandFn = func(_ context.Context, _ string, command string) (string, error) {
				commands = append(commands, command)
				return "", nil
			}

			sb, err := orch.CreateEmployeeSandbox(context.Background(), &agent, employeeStartupSecrets())
			if err != nil {
				t.Fatalf("CreateEmployeeSandbox: %v", err)
			}
			t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want no repository clone commands", commands)
			}
		})
	}
}

func TestCreateEmployeeSandbox_RepositoryCloneFailureMarksSandboxError(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	provider.endpointOverride = employeeRuntimeEndpoint(t)
	orch.cfg.EmployeeSandboxBaseImagePrefix = "employee-snapshot-v1"

	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)
	agent.IsEmployee = true
	agent.Resources = model.JSON{
		"github": map[string]any{
			"repository": []any{
				map[string]any{"id": "123456", "name": "api", "full_name": "octo-org/api"},
			},
		},
	}
	if err := db.Save(&agent).Error; err != nil {
		t.Fatalf("save employee: %v", err)
	}

	provider.executeCommandFn = func(_ context.Context, _ string, command string) (string, error) {
		if strings.HasPrefix(command, "git clone ") {
			return "", errors.New("clone failed")
		}
		return "", nil
	}

	sb, err := orch.CreateEmployeeSandbox(context.Background(), &agent, employeeStartupSecrets())
	if err == nil {
		t.Fatal("CreateEmployeeSandbox succeeded, want repository clone failure")
	}
	if sb != nil {
		t.Fatalf("sandbox return = %#v, want nil on failure", sb)
	}

	var stored model.Sandbox
	if err := db.Where("agent_id = ?", agent.ID).Order("created_at DESC").First(&stored).Error; err != nil {
		t.Fatalf("load stored sandbox: %v", err)
	}
	if stored.Status != "error" {
		t.Fatalf("stored sandbox status = %q, want error", stored.Status)
	}
	if stored.ErrorMessage == nil || !strings.Contains(*stored.ErrorMessage, "repository cloning failed") {
		t.Fatalf("stored sandbox error_message = %v, want repository cloning failure", stored.ErrorMessage)
	}
}

func employeeRuntimeEndpoint(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func employeeStartupSecrets() *employeeruntime.StartupSecrets {
	return &employeeruntime.StartupSecrets{
		SlackBotToken: "xoxb-test",
		SlackAppToken: "xapp-test",
		ProxyToken:    "ptok-test",
	}
}
