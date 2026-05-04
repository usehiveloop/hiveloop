package handler

import (
	"errors"
	"net/http"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

type listSlackChannelsResponse struct {
	Channels []slackprov.Channel `json:"channels"`
}

// @Summary List Slack public channels for an AI employee's profile
// @Description Returns public channels visible to the bot of the agent's Slack profile.
// @Tags agent-profiles
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Success 200 {object} listSlackChannelsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/slack/channels [get]
func (h *AgentProfileHandler) ListSlackChannels(w http.ResponseWriter, r *http.Request) {
	agent, orgID, err := h.resolveEmployeeAgent(r)
	if err != nil {
		switch {
		case errors.Is(err, errAgentNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		case err.Error() == "missing org context":
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		case err.Error() == "invalid agent id" || err.Error() == "profiles can only be attached to AI employees":
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		}
		return
	}

	var profile model.AgentProfile
	err = h.db.
		Where("agent_id = ? AND provider = ? AND deleted_at IS NULL AND revoked_at IS NULL", agent.ID, slackprov.Provider).
		First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no slack profile for this agent"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profile"})
		return
	}

	dek, err := h.kms.Unwrap(r.Context(), profile.WrappedDEK)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "kms unwrap failed",
			"error", err, "org_id", orgID, "profile_id", profile.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decrypt profile"})
		return
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	plaintext, err := crypto.DecryptCredential(profile.EncryptedSecrets, dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decrypt profile"})
		return
	}
	secrets, err := slackprov.DecodeSecrets(plaintext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decode profile secrets"})
		return
	}

	channels, err := slackprov.ListPublicChannels(r.Context(), secrets.BotToken)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "slack list channels failed",
			"error", err, "org_id", orgID, "profile_id", profile.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "slack channel listing failed"})
		return
	}

	writeJSON(w, http.StatusOK, listSlackChannelsResponse{Channels: channels})
}
