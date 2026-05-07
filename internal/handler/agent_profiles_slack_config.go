package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

type updateSlackConfigRequest struct {
	HomeChannelID string `json:"home_channel_id"`
}

type updateSlackConfigResponse struct {
	Profile agentProfileResponse `json:"profile"`
	Channel slackprov.Channel    `json:"channel"`
}

// @Summary Update an AI employee's Slack profile config
// @Description Sets the home channel for the agent and auto-joins the bot to that channel.
// @Tags agent-profiles
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param body body updateSlackConfigRequest true "Slack profile config"
// @Success 200 {object} updateSlackConfigResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/slack/config [patch]
func (h *AgentProfileHandler) UpdateSlackConfig(w http.ResponseWriter, r *http.Request) {
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

	var req updateSlackConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	channelID := strings.TrimSpace(req.HomeChannelID)
	if channelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "home_channel_id is required"})
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

	channel, err := slackprov.JoinChannel(r.Context(), secrets.BotToken, channelID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "slack join channel failed",
			"error", err, "org_id", orgID, "profile_id", profile.ID, "channel_id", channelID)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "could not invite the bot to that channel — make sure the channel is public and try again",
		})
		return
	}

	if profile.Config == nil {
		profile.Config = model.JSON{}
	}
	profile.Config["home_channel_id"] = channel.ID
	profile.Config["home_channel_name"] = channel.Name

	if err := h.db.Model(&profile).
		Update("config", profile.Config).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to save slack profile config",
			"error", err, "org_id", orgID, "profile_id", profile.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save profile config"})
		return
	}

	writeJSON(w, http.StatusOK, updateSlackConfigResponse{
		Profile: toAgentProfileResponse(profile),
		Channel: channel,
	})
}
