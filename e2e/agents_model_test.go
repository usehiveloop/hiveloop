package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/llmvault/llmvault/internal/model"
)

// TestAgentModels_CRUD tests all agent-related models against real Postgres.
func TestAgentModels_CRUD(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	// --- Setup: org, credential, identity ---
	org := model.Org{Name: "e2e-agents-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	cred := model.Credential{
		OrgID: org.ID, Label: "test-cred-" + suffix, BaseURL: "https://api.openai.com",
		AuthScheme: "bearer", ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	identity := model.Identity{OrgID: org.ID, ExternalID: "user-" + suffix}
	h.db.Create(&identity)
	t.Cleanup(func() { h.db.Where("id = ?", identity.ID).Delete(&model.Identity{}) })

	// === SandboxTemplate ===
	t.Run("SandboxTemplate_CRUD", func(t *testing.T) {
		st := model.SandboxTemplate{
			OrgID: org.ID, Name: "python-ml-" + suffix,
			BuildCommands: "pip install numpy pandas", BuildStatus: "pending",
			Config: model.JSON{"cpu": "2", "memory": "4096"},
		}
		h.db.Create(&st)
		t.Cleanup(func() { h.db.Where("id = ?", st.ID).Delete(&model.SandboxTemplate{}) })

		var read model.SandboxTemplate
		h.db.Where("id = ?", st.ID).First(&read)
		if read.BuildCommands != st.BuildCommands {
			t.Errorf("build_commands mismatch")
		}
		if read.BuildStatus != "pending" {
			t.Errorf("build_status: got %q", read.BuildStatus)
		}

		// Simulate build completion
		extID := "daytona-tmpl-123"
		h.db.Model(&read).Updates(map[string]any{"build_status": "ready", "external_id": extID})
		var built model.SandboxTemplate
		h.db.Where("id = ?", st.ID).First(&built)
		if built.BuildStatus != "ready" || built.ExternalID == nil || *built.ExternalID != extID {
			t.Errorf("build completion not reflected")
		}
	})

	// === Agent ===
	t.Run("Agent_CRUD", func(t *testing.T) {
		desc := "A test agent"
		agent := model.Agent{
			OrgID: org.ID, IdentityID: identity.ID, Name: "test-agent-" + suffix,
			Description: &desc, CredentialID: cred.ID, SandboxType: "shared",
			SystemPrompt: "You are a helpful assistant.", Model: "gpt-4o",
			Tools: model.JSON{"0": map[string]any{"name": "read"}},
			AgentConfig: model.JSON{"max_tokens": float64(4096), "temperature": 0.3},
			Permissions: model.JSON{"bash": "require_approval"}, Status: "active",
		}
		h.db.Create(&agent)
		t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

		var read model.Agent
		h.db.Preload("Credential").Preload("Identity").Where("id = ?", agent.ID).First(&read)
		if read.Name != agent.Name {
			t.Errorf("name mismatch")
		}
		if read.IdentityID != identity.ID {
			t.Errorf("identity_id: got %s, want %s", read.IdentityID, identity.ID)
		}
		if read.Identity.ExternalID != "user-"+suffix {
			t.Error("identity association not loaded")
		}
		if read.Credential.ID != cred.ID {
			t.Error("credential association not loaded")
		}
		bashPerm, _ := read.Permissions["bash"]
		if bashPerm != "require_approval" {
			t.Errorf("permissions.bash: got %v", bashPerm)
		}

		// Unique constraint
		dup := model.Agent{
			OrgID: org.ID, IdentityID: identity.ID, Name: agent.Name,
			CredentialID: cred.ID, SandboxType: "dedicated", SystemPrompt: "dup", Model: "gpt-4o",
		}
		if err := h.db.Create(&dup).Error; err == nil {
			h.db.Where("id = ?", dup.ID).Delete(&model.Agent{})
			t.Fatal("expected unique constraint violation")
		}
	})

	// === Sandbox (with identity) ===
	t.Run("Sandbox_CRUD", func(t *testing.T) {
		sandbox := model.Sandbox{
			OrgID: org.ID, IdentityID: identity.ID, SandboxType: "shared",
			ExternalID: "daytona-ws-" + suffix, BridgeURL: "https://sandbox.test:8080",
			EncryptedBridgeAPIKey: []byte("encrypted-bridge-key"), Status: "creating",
		}
		h.db.Create(&sandbox)
		t.Cleanup(func() { h.db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{}) })

		var read model.Sandbox
		h.db.Where("id = ?", sandbox.ID).First(&read)
		if read.IdentityID != identity.ID {
			t.Errorf("identity_id mismatch")
		}
		if read.Status != "creating" {
			t.Errorf("status: got %q", read.Status)
		}

		now := time.Now()
		h.db.Model(&read).Updates(map[string]any{"status": "running", "last_active_at": now})
		var running model.Sandbox
		h.db.Where("id = ?", sandbox.ID).First(&running)
		if running.Status != "running" || running.LastActiveAt == nil {
			t.Error("status transition failed")
		}
	})

	// === WorkspaceStorage ===
	t.Run("WorkspaceStorage_CRUD", func(t *testing.T) {
		ws := model.WorkspaceStorage{
			OrgID: org.ID, TursoDatabaseName: "llmvault-" + suffix,
			StorageURL: "libsql://llmvault-" + suffix + ".turso.io", StorageAuthToken: "tok", WrappedDEK: []byte("dek"),
		}
		h.db.Create(&ws)
		t.Cleanup(func() { h.db.Where("id = ?", ws.ID).Delete(&model.WorkspaceStorage{}) })

		var read model.WorkspaceStorage
		h.db.Where("id = ?", ws.ID).First(&read)
		if read.StorageURL != ws.StorageURL {
			t.Errorf("storage_url mismatch")
		}

		// Unique per org
		dup := model.WorkspaceStorage{OrgID: org.ID, TursoDatabaseName: "dup", StorageURL: "x", StorageAuthToken: "x"}
		if err := h.db.Create(&dup).Error; err == nil {
			h.db.Where("id = ?", dup.ID).Delete(&model.WorkspaceStorage{})
			t.Fatal("expected unique constraint violation")
		}
	})

	// === AgentConversation + ConversationEvent ===
	t.Run("AgentConversation_and_Events", func(t *testing.T) {
		agent := model.Agent{
			OrgID: org.ID, IdentityID: identity.ID, Name: "conv-agent-" + suffix,
			CredentialID: cred.ID, SandboxType: "shared", SystemPrompt: "test", Model: "gpt-4o",
		}
		h.db.Create(&agent)
		t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

		sandbox := model.Sandbox{
			OrgID: org.ID, IdentityID: identity.ID, SandboxType: "shared",
			ExternalID: "conv-ws-" + suffix, BridgeURL: "https://conv.test:8080",
			EncryptedBridgeAPIKey: []byte("encrypted-bridge-key"), Status: "running",
		}
		h.db.Create(&sandbox)
		t.Cleanup(func() { h.db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{}) })

		conv := model.AgentConversation{
			OrgID: org.ID, AgentID: agent.ID, SandboxID: sandbox.ID,
			BridgeConversationID: "bridge-conv-" + suffix, Status: "active",
			IntegrationScopes: model.JSON{"scopes": []any{"github"}},
		}
		h.db.Create(&conv)
		t.Cleanup(func() { h.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{}) })

		events := []model.ConversationEvent{
			{OrgID: org.ID, ConversationID: conv.ID, EventType: "message_received", Payload: model.JSON{"content": "Hello"}},
			{OrgID: org.ID, ConversationID: conv.ID, EventType: "response_completed", Payload: model.JSON{"content": "Hi!"}},
			{OrgID: org.ID, ConversationID: conv.ID, EventType: "turn_completed", Payload: model.JSON{}},
		}
		for i := range events {
			h.db.Create(&events[i])
		}
		t.Cleanup(func() { h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{}) })

		var readEvents []model.ConversationEvent
		h.db.Where("conversation_id = ?", conv.ID).Order("created_at ASC").Find(&readEvents)
		if len(readEvents) != 3 {
			t.Fatalf("expected 3 events, got %d", len(readEvents))
		}

		now := time.Now()
		h.db.Model(&conv).Updates(map[string]any{"status": "ended", "ended_at": now})
		var ended model.AgentConversation
		h.db.Where("id = ?", conv.ID).First(&ended)
		if ended.Status != "ended" || ended.EndedAt == nil {
			t.Error("conversation end failed")
		}
	})
}

// TestAgentModels_CascadeDelete verifies org cascade deletes all agent-related records.
func TestAgentModels_CascadeDelete(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	org := model.Org{Name: "e2e-cascade-" + suffix}
	h.db.Create(&org)

	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)

	identity := model.Identity{OrgID: org.ID, ExternalID: "cascade-user-" + suffix}
	h.db.Create(&identity)

	st := model.SandboxTemplate{OrgID: org.ID, Name: "cascade-tmpl-" + suffix}
	h.db.Create(&st)

	agent := model.Agent{
		OrgID: org.ID, IdentityID: identity.ID, Name: "cascade-agent-" + suffix,
		CredentialID: cred.ID, SandboxType: "shared", SystemPrompt: "test", Model: "gpt-4o",
	}
	h.db.Create(&agent)

	sandbox := model.Sandbox{
		OrgID: org.ID, IdentityID: identity.ID, SandboxType: "shared",
		ExternalID: "cascade-ws-" + suffix, BridgeURL: "https://test:8080",
		EncryptedBridgeAPIKey: []byte("encrypted-bridge-key"), Status: "running",
	}
	h.db.Create(&sandbox)

	ws := model.WorkspaceStorage{
		OrgID: org.ID, TursoDatabaseName: "cascade-" + suffix,
		StorageURL: "libsql://cascade.turso.io", StorageAuthToken: "tok",
	}
	h.db.Create(&ws)

	conv := model.AgentConversation{
		OrgID: org.ID, AgentID: agent.ID, SandboxID: sandbox.ID,
		BridgeConversationID: "cascade-conv-" + suffix, Status: "active",
	}
	h.db.Create(&conv)

	event := model.ConversationEvent{
		OrgID: org.ID, ConversationID: conv.ID, EventType: "message_received", Payload: model.JSON{},
	}
	h.db.Create(&event)

	// Delete org — everything cascades
	h.db.Delete(&org)

	checks := []struct {
		name  string
		model any
		id    uuid.UUID
	}{
		{"sandbox_template", &model.SandboxTemplate{}, st.ID},
		{"agent", &model.Agent{}, agent.ID},
		{"sandbox", &model.Sandbox{}, sandbox.ID},
		{"workspace_storage", &model.WorkspaceStorage{}, ws.ID},
		{"agent_conversation", &model.AgentConversation{}, conv.ID},
		{"conversation_event", &model.ConversationEvent{}, event.ID},
	}
	for _, c := range checks {
		var count int64
		h.db.Model(c.model).Where("id = ?", c.id).Count(&count)
		if count != 0 {
			t.Errorf("%s should have been cascade-deleted", c.name)
		}
	}
}
