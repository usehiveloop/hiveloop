package e2e

import (
	"encoding/base64"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

// TestSkillsPassthrough_NewWireShape locks the bundle id, not the title, as
// skill.name in the runtime definition.
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

	def, err := employeeruntime.Compile(t.Context(), employeeruntime.CompileDeps{
		DB:         h.db,
		SigningKey: h.signingKey,
		Cfg: &config.Config{
			ProxyHost:  "proxy.test",
			MCPBaseURL: "https://mcp.test",
		},
	}, &agent)
	if err != nil {
		t.Fatalf("compile employee runtime definition: %v", err)
	}

	gotSkills := def.Skills
	if len(gotSkills) != 2 {
		t.Fatalf("def.skills: got %d, want 2 (raw=%v)", len(gotSkills), gotSkills)
	}

	expected := map[string]string{
		skillFixtures[0].bundleID: skillFixtures[0].title,
		skillFixtures[1].bundleID: skillFixtures[1].title,
	}
	for i, s := range gotSkills {
		wantTitle, ok := expected[s.Name]
		if !ok {
			t.Errorf("skill[%d].name = %q, not in expected set %v", i, s.Name, expected)
			continue
		}
		if s.Trigger == nil {
			t.Errorf("skill[%d].trigger is nil", i)
		}
		if s.Name == wantTitle {
			t.Errorf("skill[%d] name == title (%q); name must be the bundle id", i, s.Name)
		}
		delete(expected, s.Name)
	}
	if len(expected) != 0 {
		t.Errorf("missing skills: %v", expected)
	}

	for _, s := range gotSkills {
		if s.Name == "use-railway" || s.Name == "use-vercel" {
			t.Errorf("skill name leaked the title: %q", s.Name)
		}
	}

	if def.Mode != "employee" {
		t.Errorf("mode: got %q, want employee", def.Mode)
	}
	if def.Model.ModelID != "claude-sonnet-4-5" {
		t.Errorf("model: got %q, want claude-sonnet-4-5", def.Model.ModelID)
	}
}
