package handler

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// Delete handles DELETE /v1/agents/{id}.
// @Summary Delete an agent
// @Description Deletes an agent and removes it from Bridge.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [delete]
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false", id, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	sandboxExternalIDs, err := loadAgentSandboxExternalIDs(h.db, agent.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare agent cleanup"})
		return
	}
	nangoConnections, err := loadAgentProfileNangoConnections(h.db, agent.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare agent profile cleanup"})
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := deleteAgentNonCascadingReferences(tx, agent.ID); err != nil {
			return err
		}
		return tx.Delete(&agent).Error
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete agent"})
		return
	}

	if h.enqueuer != nil && len(sandboxExternalIDs) > 0 {
		task, err := tasks.NewAgentCleanupTask(agent.ID, sandboxExternalIDs...)
		if err != nil {
			logging.Capture(r.Context(), fmt.Errorf("create agent cleanup task agent_id=%s: %w", agent.ID, err))
		} else if _, err := h.enqueuer.Enqueue(task); err != nil {
			logging.Capture(r.Context(), fmt.Errorf("enqueue agent cleanup agent_id=%s: %w", agent.ID, err))
		}
	}
	if h.enqueuer != nil && len(nangoConnections) > 0 {
		task, err := tasks.NewAgentProfileNangoCleanupTask(agent.ID, nangoConnections)
		if err != nil {
			logging.Capture(r.Context(), fmt.Errorf("create agent profile nango cleanup task agent_id=%s: %w", agent.ID, err))
		} else if _, err := h.enqueuer.Enqueue(task); err != nil {
			logging.Capture(r.Context(), fmt.Errorf("enqueue agent profile nango cleanup agent_id=%s: %w", agent.ID, err))
		}
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "agent hard-deleted", "agent_id", agent.ID, "org_id", org.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func loadAgentSandboxExternalIDs(db *gorm.DB, agentID uuid.UUID) ([]string, error) {
	var externalIDs []string
	if err := db.Model(&model.Sandbox{}).
		Where("agent_id = ? AND external_id <> ''", agentID).
		Pluck("external_id", &externalIDs).Error; err != nil {
		return nil, err
	}
	return externalIDs, nil
}

func loadAgentProfileNangoConnections(db *gorm.DB, agentID uuid.UUID) ([]tasks.NangoConnectionDeleteTarget, error) {
	var profiles []model.AgentProfile
	if err := db.
		Where("agent_id = ? AND deleted_at IS NULL AND revoked_at IS NULL", agentID).
		Find(&profiles).Error; err != nil {
		return nil, err
	}
	targets := make([]tasks.NangoConnectionDeleteTarget, 0, len(profiles))
	seen := map[string]struct{}{}
	for _, profile := range profiles {
		connectionID := stringFromAny(profile.Config["nango_connection_id"])
		providerConfigKey := stringFromAny(profile.Config["provider_config_key"])
		if connectionID == "" || providerConfigKey == "" {
			if filled, ok := loadNangoConnectionFromProfileReference(db, profile); ok {
				connectionID = filled.ConnectionID
				providerConfigKey = filled.ProviderConfigKey
			}
		}
		if connectionID == "" || providerConfigKey == "" {
			continue
		}
		key := providerConfigKey + "\x00" + connectionID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, tasks.NangoConnectionDeleteTarget{
			ConnectionID:      connectionID,
			ProviderConfigKey: providerConfigKey,
			ProfileID:         profile.ID,
			Provider:          profile.Provider,
		})
	}
	return targets, nil
}

func loadNangoConnectionFromProfileReference(db *gorm.DB, profile model.AgentProfile) (tasks.NangoConnectionDeleteTarget, bool) {
	rawID := stringFromAny(profile.Config["in_connection_id"])
	if rawID == "" {
		return tasks.NangoConnectionDeleteTarget{}, false
	}
	inConnectionID, err := uuid.Parse(rawID)
	if err != nil {
		return tasks.NangoConnectionDeleteTarget{}, false
	}
	var conn model.InConnection
	if err := db.Preload("InIntegration").Where("id = ? AND org_id = ?", inConnectionID, profile.OrgID).First(&conn).Error; err != nil {
		return tasks.NangoConnectionDeleteTarget{}, false
	}
	if conn.NangoConnectionID == "" || conn.InIntegration.UniqueKey == "" {
		return tasks.NangoConnectionDeleteTarget{}, false
	}
	return tasks.NangoConnectionDeleteTarget{
		ConnectionID:      conn.NangoConnectionID,
		ProviderConfigKey: inNangoKey(conn.InIntegration.UniqueKey),
		ProfileID:         profile.ID,
		Provider:          profile.Provider,
	}, true
}

func deleteAgentNonCascadingReferences(db *gorm.DB, agentID uuid.UUID) error {
	if err := deleteAgentTriggers(db, agentID); err != nil {
		return err
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.ConversationSubscription{}).Error; err != nil {
		return fmt.Errorf("delete conversation subscriptions: %w", err)
	}
	if err := db.Exec(`DELETE FROM chat_messages WHERE session_id IN (SELECT id FROM chat_sessions WHERE agent_id = ?)`, agentID).Error; err != nil {
		return fmt.Errorf("delete chat messages: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.ChatSession{}).Error; err != nil {
		return fmt.Errorf("delete chat sessions: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.RouterConversation{}).Error; err != nil {
		return fmt.Errorf("delete router conversations: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.EmployeeAsset{}).Error; err != nil {
		return fmt.Errorf("delete employee assets: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.HindsightBank{}).Error; err != nil {
		return fmt.Errorf("delete hindsight banks: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID.String()).Delete(&model.ToolUsage{}).Error; err != nil {
		return fmt.Errorf("delete tool usage: %w", err)
	}
	if err := db.Exec(`DELETE FROM generations WHERE token_jti IN (SELECT jti FROM tokens WHERE meta->>'agent_id' = ?)`, agentID.String()).Error; err != nil {
		return fmt.Errorf("delete agent generations: %w", err)
	}
	if err := db.Where("meta->>'agent_id' = ?", agentID.String()).Delete(&model.Token{}).Error; err != nil {
		return fmt.Errorf("delete agent proxy tokens: %w", err)
	}
	return nil
}
