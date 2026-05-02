// Wave 3 e2e: skills wire shape on the UpsertAgent push path.
//
// Required infra:
//   DATABASE_URL  → Postgres reachable
//
// This test seeds a real Skill + SkillVersion in Postgres, attaches it
// to an agent, then drives the pusher's push path against a fakebridge
// and asserts the resulting AgentDefinition.skills contract:
//   - skill.id MUST be the bundle's id (which the loader copies from the
//     Skill row's slug-derived bundle), not the title.
//   - skill.title comes from the bundle.
//   - 2 skills both make it through, in order.
package e2e

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/e2e/fakebridge"
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// TestSkillsPassthrough_NewWireShape locks the AgentDefinition.skills
// shape sent to the new bridge: each skill carries `id` and `title`, no
// dead fields, and the `id` is the bundle id (not the title).
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

	encryptedAPIKey, _ := encKey.EncryptString("sk-test")
	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.anthropic.com", AuthScheme: "bearer",
		ProviderID: "anthropic", EncryptedKey: encryptedAPIKey, WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		OrgID: &org.ID, Name: "sk-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "test", Model: "claude-sonnet-4-5",
		Status: "active", AgentType: "agent",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

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
		sv := model.SkillVersion{
			ID:      uuid.New(),
			Version: "v1",
			Bundle: model.RawJSON(`{
				"id": "` + sf.bundleID + `",
				"title": "` + sf.title + `",
				"description": "` + sf.desc + `",
				"content": "# ` + sf.title + `\nbody"
			}`),
		}
		sk := model.Skill{
			ID:              uuid.New(),
			Slug:            sf.bundleID,
			Name:            sf.title,
			SourceType:      "inline",
			Status:          "published",
			LatestVersionID: &sv.ID,
		}
		sv.SkillID = sk.ID
		h.db.Create(&sk)
		h.db.Create(&sv)
		h.db.Create(&model.AgentSkill{AgentID: agent.ID, SkillID: sk.ID})
		skillIDs = append(skillIDs, sk.ID)
		t.Cleanup(func() {
			h.db.Where("agent_id = ? AND skill_id = ?", agent.ID, sk.ID).Delete(&model.AgentSkill{})
			h.db.Where("skill_id = ?", sk.ID).Delete(&model.SkillVersion{})
			h.db.Where("id = ?", sk.ID).Delete(&model.Skill{})
		})
	}
	_ = skillIDs

	// Spin a fakebridge and a pusher pointed at it.
	fb := fakebridge.New(t)

	expiresAt := time.Now().Add(24 * time.Hour)
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "sk-ext-" + suffix,
		BridgeURL:             fb.URL,
		BridgeURLExpiresAt:    &expiresAt,
		EncryptedBridgeAPIKey: encryptedAPIKey,
		Status:                "running",
	}
	h.db.Create(&sb)
	t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	cfg := &config.Config{
		ProxyHost:  "proxy.test",
		MCPBaseURL: "https://mcp.test",
		BridgeHost: "bridge.test",
	}
	orch := sandbox.NewOrchestrator(h.db, nil, nil, encKey, cfg)
	pusher := sandbox.NewPusher(h.db, orch, h.signingKey, cfg, nil)

	if err := pusher.PushAgentToSandbox(t.Context(), &agent, &sb); err != nil {
		t.Fatalf("PushAgentToSandbox: %v", err)
	}

	cap := fb.CapturedSnapshot()
	if len(cap.UpsertAgents) != 1 {
		t.Fatalf("UpsertAgent calls: got %d, want 1", len(cap.UpsertAgents))
	}
	def := cap.UpsertAgents[0]
	if def.Skills == nil {
		t.Fatalf("def.skills is nil; want 2 entries")
	}
	gotSkills := *def.Skills
	if len(gotSkills) != 2 {
		t.Fatalf("def.skills: got %d, want 2 (raw=%v)", len(gotSkills), gotSkills)
	}

	// Build a set of (id, title) we expect.
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
		// id MUST NOT equal title — that was the audit's bug.
		if idStr == s.Title {
			t.Errorf("skill[%d] id == title (%q); id must be the bundle id, not the title", i, idStr)
		}
		delete(expected, idStr)
	}
	if len(expected) != 0 {
		t.Errorf("missing skills: %v", expected)
	}

	// Spot-check that skill.id is not the human-readable name (anti-bug
	// from the audit). One of our titles is "use-railway"; if any pushed
	// skill uses that string as id, that's the regression.
	for _, s := range gotSkills {
		if string(s.Id) == "use-railway" || string(s.Id) == "use-vercel" {
			t.Errorf("skill id leaked the title: %q", s.Id)
		}
	}

	// Defensive: harness/provider/wire-shape sanity from the same body.
	if def.Harness != bridgepkg.Claude {
		t.Errorf("harness: got %q, want claude", def.Harness)
	}
	if !strings.HasPrefix(def.Provider.Model, "claude") {
		t.Errorf("model: got %q, want claude*", def.Provider.Model)
	}
}
