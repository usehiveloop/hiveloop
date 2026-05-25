package e2e

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	bridgepkg "github.com/usehivy/hivy/internal/bridge"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// TestSkillsPassthrough_NewWireShape locks the bundle id (not the title)
// as skill.id on the wire — the prior bug used the title and broke the
// bridge's lookup.
func TestSkillsPassthrough_NewWireShape(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 89)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("symmetric key: %v", err)
	}

	org := model.Org{Name: "sk-org-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	orgID := org.ID

	encryptedAPIKey, _ := encKey.EncryptString("sk-test")
	cred := model.Credential{
		OrgID: &orgID, BaseURL: "https://api.anthropic.com", AuthScheme: "bearer",
		ProviderID: "anthropic", EncryptedKey: encryptedAPIKey, WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Employee{
		OrgID: &orgID, Name: "sk-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "test", Model: "claude-sonnet-4-5",
		Status: "active",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Employee{}) })

	skillFixtures := []struct {
		bundleID string
		title    string
		desc     string
	}{
		{"use-railway-" + suffix, "use-railway", "Operate Railway"},
		{"use-vercel-" + suffix, "use-vercel", "Deploy to Vercel"},
	}
	skillIDs := make([]uuid.UUID, 0, len(skillFixtures))
	for _, sf := range skillFixtures {
		sk := model.Skill{
			ID:         uuid.New(),
			Slug:       sf.bundleID,
			Name:       sf.title,
			SourceType: "inline",
			Status:     "published",
			Bundle: model.RawJSON(`{
				"id": "` + sf.bundleID + `",
				"title": "` + sf.title + `",
				"description": "` + sf.desc + `",
				"content": "# ` + sf.title + `\nbody"
			}`),
		}
		h.db.Create(&sk)
		h.db.Create(&model.EmployeeSkill{EmployeeID: agent.ID, SkillID: sk.ID})
		skillIDs = append(skillIDs, sk.ID)
		t.Cleanup(func() {
			h.db.Where("employee_id = ? AND skill_id = ?", agent.ID, sk.ID).Delete(&model.EmployeeSkill{})
			h.db.Where("id = ?", sk.ID).Delete(&model.Skill{})
		})
	}
	_ = skillIDs

	pushTarget := newAgentPushCapture(t)

	expiresAt := time.Now().Add(24 * time.Hour)
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		EmployeeID:            &agent.ID,
		ExternalID:            "sk-ext-" + suffix,
		BridgeURL:             pushTarget.URL,
		BridgeURLExpiresAt:    &expiresAt,
		EncryptedBridgeAPIKey: encryptedAPIKey,
		Status:                "running",
	}
	h.db.Create(&sb)
	t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	cfg := &config.Config{
		ProxyHost:             "proxy.test",
		MCPBaseURL:            "https://mcp.test",
		SpecialistSandboxHost: "bridge.test",
	}
	orch := sandbox.NewOrchestrator(h.db, &e2eSandboxProvider{endpoint: pushTarget.URL}, encKey, cfg)
	pusher := sandbox.NewPusher(h.db, orch, h.signingKey, cfg, nil)

	if err := pusher.PushSpecialistToSandbox(t.Context(), &agent, &sb); err != nil {
		t.Fatalf("PushSpecialistToSandbox: %v", err)
	}

	if len(pushTarget.UpsertAgents) != 1 {
		t.Fatalf("UpsertAgent calls: got %d, want 1", len(pushTarget.UpsertAgents))
	}
	def := pushTarget.UpsertAgents[0]
	if def.Skills == nil {
		t.Fatalf("def.skills is nil; want 2 entries")
	}
	gotSkills := *def.Skills
	if len(gotSkills) != 2 {
		t.Fatalf("def.skills: got %d, want 2 (raw=%v)", len(gotSkills), gotSkills)
	}

	expected := map[string]string{
		skillFixtures[0].bundleID: skillFixtures[0].title,
		skillFixtures[1].bundleID: skillFixtures[1].title,
	}
	for i, s := range gotSkills {
		idStr := string(s.Id)
		wantTitle, ok := expected[idStr]
		if !ok {
			t.Errorf("skill[%d].id = %q, not in expected set %v", i, idStr, expected)
			continue
		}
		if s.Title != wantTitle {
			t.Errorf("skill[%d].title = %q, want %q", i, s.Title, wantTitle)
		}
		if idStr == s.Title {
			t.Errorf("skill[%d] id == title (%q); id must be the bundle id, not the title", i, idStr)
		}
		delete(expected, idStr)
	}
	if len(expected) != 0 {
		t.Errorf("missing skills: %v", expected)
	}

	for _, s := range gotSkills {
		if string(s.Id) == "use-railway" || string(s.Id) == "use-vercel" {
			t.Errorf("skill id leaked the title: %q", s.Id)
		}
	}

	if def.Harness != bridgepkg.OpenCode {
		t.Errorf("harness: got %q, want open_code (default for agents without an explicit harness)", def.Harness)
	}
	if !strings.HasPrefix(def.Provider.Model, "claude") {
		t.Errorf("model: got %q, want claude*", def.Provider.Model)
	}
}

type agentPushCapture struct {
	URL          string
	UpsertAgents []bridgepkg.AgentDefinition
}

func newAgentPushCapture(t *testing.T) *agentPushCapture {
	t.Helper()

	capture := &agentPushCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/push/agents/") {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var def bridgepkg.AgentDefinition
		if err := json.Unmarshal(body, &def); err != nil {
			http.Error(w, "invalid agent definition", http.StatusBadRequest)
			return
		}
		capture.UpsertAgents = append(capture.UpsertAgents, def)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	capture.URL = server.URL
	return capture
}
