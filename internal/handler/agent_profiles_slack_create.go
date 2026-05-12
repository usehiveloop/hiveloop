package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

type createSlackProfileRequest struct {
	Label    string `json:"label"`
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
}

type createSlackProfileResponse struct {
	Profile  agentProfileResponse `json:"profile"`
	Channels []slackprov.Channel  `json:"channels"`
}

// @Summary Create a Slack profile for an AI employee
// @Description Validates Slack bot+app tokens, stores them encrypted, and returns the profile plus public channels.
// @Tags agent-profiles
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param body body createSlackProfileRequest true "Slack tokens"
// @Success 201 {object} createSlackProfileResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/slack [post]
func (h *AgentProfileHandler) CreateSlack(w http.ResponseWriter, r *http.Request) {
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

	var req createSlackProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	secrets := slackprov.Secrets{BotToken: req.BotToken, AppToken: req.AppToken}
	identity, err := slackprov.VerifyAndIntrospect(r.Context(), secrets)
	if err != nil {
		var verr *slackprov.ValidationError
		if errors.As(err, &verr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": verr.Msg})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not verify Slack tokens"})
		return
	}

	channels, err := slackprov.ListPublicChannels(r.Context(), secrets.BotToken)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "tokens valid but listing channels failed — make sure the Slack app has the channels:read scope",
		})
		return
	}

	plaintext, err := slackprov.EncodeSecrets(secrets)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode secrets failed"})
		return
	}
	dek, err := crypto.GenerateDEK()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}
	encrypted, err := crypto.EncryptCredential(plaintext, dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}
	wrappedDEK, err := h.kms.Wrap(r.Context(), dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key wrapping failed"})
		return
	}
	for i := range dek {
		dek[i] = 0
	}

	identityJSON, err := jsonRoundTripToMap(identity)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode identity failed"})
		return
	}

	now := time.Now().UTC()
	label := req.Label
	if label == "" {
		label = identity.TeamName
	}
	profile := model.AgentProfile{
		ID:               uuid.New(),
		OrgID:            orgID,
		AgentID:          agent.ID,
		Provider:         slackprov.Provider,
		ExternalID:       identity.TeamID,
		Label:            label,
		Identity:         identityJSON,
		Config:           model.JSON{},
		EncryptedSecrets: encrypted,
		WrappedDEK:       wrappedDEK,
		Status:           "active",
		LastVerifiedAt:   &now,
	}

	if err := h.db.Create(&profile).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "this Slack workspace is already connected to this employee",
			})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create slack profile",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "team_id", identity.TeamID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create profile"})
		return
	}
	h.ensureSlackKnowledgeSource(r.Context(), agent, &profile)

	writeJSON(w, http.StatusCreated, createSlackProfileResponse{
		Profile:  toAgentProfileResponse(profile),
		Channels: channels,
	})
}
