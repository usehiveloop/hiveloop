package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
	"github.com/usehivy/hivy/internal/slackapp"
)

type SlackChannelHandler struct {
	db                 *gorm.DB
	nango              *nango.Client
	enq                enqueue.TaskEnqueuer
	loadBotToken       func(context.Context, uuid.UUID) (string, error)
	listPublicChannels func(context.Context, string) ([]slackapp.Channel, error)
	listBotChannels    func(context.Context, string) ([]slackapp.Channel, error)
	joinChannel        func(context.Context, string, string) (slackapp.Channel, error)
}

func NewSlackChannelHandler(db *gorm.DB, nangoClient *nango.Client, enq enqueue.TaskEnqueuer) *SlackChannelHandler {
	h := &SlackChannelHandler{db: db, nango: nangoClient, enq: enq}
	h.loadBotToken = h.loadSlackBotToken
	h.listPublicChannels = slackapp.ListPublicChannels
	h.listBotChannels = slackapp.ListBotChannels
	h.joinChannel = slackapp.JoinChannel
	return h
}

type slackChannelResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsPrivate  bool   `json:"is_private"`
	IsArchived bool   `json:"is_archived"`
	IsMember   bool   `json:"is_member"`
	Topic      string `json:"topic,omitempty"`
	Purpose    string `json:"purpose,omitempty"`
	NumMembers int    `json:"num_members,omitempty"`
}

type slackChannelsResponse struct {
	Channels []slackChannelResponse `json:"channels"`
}

type joinSlackChannelsRequest struct {
	AllPublic  bool     `json:"all_public,omitempty"`
	ChannelIDs []string `json:"channel_ids,omitempty"`
}

type joinSlackChannelFailure struct {
	ChannelID string `json:"channel_id"`
	Error     string `json:"error"`
}

type joinSlackChannelsResponse struct {
	Joined        int                       `json:"joined"`
	AlreadyMember int                       `json:"already_member"`
	Failed        int                       `json:"failed"`
	Failures      []joinSlackChannelFailure `json:"failures,omitempty"`
	publicReady   bool                      `json:"-"`
	allReady      bool                      `json:"-"`
}

// List channels Hivy can be invited to or is already in.
// @Summary List Slack channels
// @Description Returns public Slack channels plus private channels where Hivy is already a member.
// @Tags slack
// @Produce json
// @Success 200 {object} slackChannelsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/slack/channels [get]
func (h *SlackChannelHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	token, err := h.loadBotToken(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "active Slack connection required"})
		return
	}
	channels, err := h.availableChannels(r.Context(), token)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "list Slack channels", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list Slack channels"})
		return
	}
	writeJSON(w, http.StatusOK, slackChannelsResponse{Channels: toSlackChannelResponses(channels)})
}

// JoinChannels invites Hivy to public Slack channels.
// @Summary Join Slack channels
// @Description Invites Hivy to all public channels or selected channels. Joined private channels are treated as already available.
// @Tags slack
// @Accept json
// @Produce json
// @Param body body joinSlackChannelsRequest true "Join request"
// @Success 200 {object} joinSlackChannelsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/slack/channels/join [post]
func (h *SlackChannelHandler) JoinChannels(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var req joinSlackChannelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if !req.AllPublic && len(req.ChannelIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "all_public or channel_ids is required"})
		return
	}
	token, err := h.loadBotToken(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "active Slack connection required"})
		return
	}

	result, err := h.joinRequestedChannels(r.Context(), token, req)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "join Slack channels", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to join Slack channels"})
		return
	}

	if result.Joined > 0 {
		h.autoCreateSlackRAGSource(r.Context(), org.ID)
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *SlackChannelHandler) availableChannels(ctx context.Context, botToken string) ([]slackapp.Channel, error) {
	publicChannels, err := h.listPublicChannels(ctx, botToken)
	if err != nil {
		return nil, err
	}
	botChannels, err := h.listBotChannels(ctx, botToken)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]slackapp.Channel, len(publicChannels)+len(botChannels))
	for _, ch := range publicChannels {
		if ch.IsArchived {
			continue
		}
		ch.IsPrivate = false
		byID[ch.ID] = ch
	}
	for _, ch := range botChannels {
		if ch.IsArchived || !ch.IsMember {
			continue
		}
		existing := byID[ch.ID]
		if existing.ID != "" {
			existing.IsMember = true
			existing.Topic = firstNonEmpty(existing.Topic, ch.Topic)
			existing.Purpose = firstNonEmpty(existing.Purpose, ch.Purpose)
			if existing.NumMembers == 0 {
				existing.NumMembers = ch.NumMembers
			}
			byID[ch.ID] = existing
			continue
		}
		if ch.IsPrivate {
			byID[ch.ID] = ch
		}
	}
	out := make([]slackapp.Channel, 0, len(byID))
	for _, ch := range byID {
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (h *SlackChannelHandler) joinRequestedChannels(ctx context.Context, botToken string, req joinSlackChannelsRequest) (joinSlackChannelsResponse, error) {
	channels, err := h.availableChannels(ctx, botToken)
	if err != nil {
		return joinSlackChannelsResponse{}, err
	}
	targets := make([]slackapp.Channel, 0, len(channels))
	if req.AllPublic {
		for _, ch := range channels {
			if !ch.IsPrivate {
				targets = append(targets, ch)
			}
		}
	} else {
		byID := make(map[string]slackapp.Channel, len(channels))
		for _, ch := range channels {
			byID[ch.ID] = ch
		}
		seen := map[string]bool{}
		for _, id := range req.ChannelIDs {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if ch, ok := byID[id]; ok {
				targets = append(targets, ch)
			} else {
				targets = append(targets, slackapp.Channel{ID: id, IsPrivate: true})
			}
		}
	}

	result := joinSlackChannelsResponse{}
	for _, ch := range targets {
		if ch.IsMember {
			result.AlreadyMember++
			if !ch.IsPrivate {
				result.publicReady = true
			}
			continue
		}
		if ch.IsPrivate {
			result.Failed++
			result.Failures = append(result.Failures, joinSlackChannelFailure{
				ChannelID: ch.ID,
				Error:     "private channels must already include Hivy",
			})
			continue
		}
		joined, err := h.joinChannel(ctx, botToken, ch.ID)
		if err != nil {
			result.Failed++
			result.Failures = append(result.Failures, joinSlackChannelFailure{ChannelID: ch.ID, Error: err.Error()})
			continue
		}
		if joined.IsMember || joined.ID != "" {
			result.Joined++
			result.publicReady = true
		} else {
			result.AlreadyMember++
		}
	}
	result.allReady = len(targets) > 0 && result.Failed == 0 && result.Joined+result.AlreadyMember == len(targets)
	return result, nil
}

func (h *SlackChannelHandler) loadSlackBotToken(ctx context.Context, orgID uuid.UUID) (string, error) {
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, slackapp.Provider).
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return "", fmt.Errorf("active Slack connection required: %w", err)
	}
	nangoConn, err := h.nango.GetConnection(ctx, conn.NangoConnectionID, nangoProviderConfigKey(conn.Integration.UniqueKey))
	if err != nil {
		return "", fmt.Errorf("load Slack connection credentials: %w", err)
	}
	creds, _ := nangoConn["credentials"].(map[string]any)
	for _, key := range []string{"bot_token", "access_token"} {
		if token, _ := creds[key].(string); strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}
	return "", fmt.Errorf("Slack connection credentials do not include a bot token")
}

func (h *SlackChannelHandler) autoCreateSlackRAGSource(ctx context.Context, orgID uuid.UUID) {
	if h.enq == nil {
		return
	}
	connID, err := h.activeSlackConnectionID(ctx, orgID)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag: %w", err))
		return
	}

	src := &ragmodel.RAGSource{
		OrgIDValue: orgID,
		KindValue:  ragmodel.RAGSourceKindIntegration,
		Name:       "Slack",
		Status:     ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:    true,
		AccessType: ragmodel.AccessTypeSync,
		RefreshFreqSeconds: intPtr(3600),
	}
	src.ConnectionID = &connID

	// Only create if one doesn't already exist for this connection.
	if err := h.db.Create(src).Error; err != nil {
		if isDuplicateKeyError(err) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag source for org %s: %w", orgID, err))
		return
	}

	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: src.ID})
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag: build ingest task for %s: %w", src.ID, err))
		return
	}
	if _, err := h.enq.Enqueue(task, asynq.Unique(60*time.Second)); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("auto-create slack rag: enqueue ingest for %s: %w", src.ID, err))
	}
}

func (h *SlackChannelHandler) activeSlackConnectionID(ctx context.Context, orgID uuid.UUID) (uuid.UUID, error) {
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, slackapp.Provider).
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return uuid.Nil, fmt.Errorf("no active Slack connection: %w", err)
	}
	return conn.ID, nil
}
