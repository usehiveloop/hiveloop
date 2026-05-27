package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type slackEvent struct {
	Type     string `json:"type"`
	TeamID   string `json:"team_id"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	BotID    string `json:"bot_id"`
	Subtype  string `json:"subtype"`
	UserName string `json:"user_name"`
}

type slackEventCallback struct {
	Type      string     `json:"type"`
	Challenge string     `json:"challenge"`
	Event     slackEvent `json:"event"`
}

func isSlackProvider(conn *model.Connection) bool {
	return conn != nil && conn.Integration.Provider == slackapp.Provider
}

func handleSlackWebhookEvent(ctx context.Context, w http.ResponseWriter, wh *nangoWebhook, wctx *webhookContext) bool {
	body, _ := unwrapSlackEvent(wh.Payload)
	if len(body) == 0 {
		return false
	}

	var callback slackEventCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "slack_webhook: failed to parse event",
			"org_id", wctx.orgID.String(),
			"connection_id", wctx.connection.ID.String(),
			"error", err,
		)
		return true
	}

	if callback.Type == "url_verification" {
		writeJSON(w, http.StatusOK, map[string]string{"challenge": callback.Challenge})
		return true
	}

	if callback.Type != "event_callback" {
		logging.FromContext(ctx).InfoContext(ctx, "slack_webhook_ignored",
			"org_id", wctx.orgID.String(),
			"connection_id", wctx.connection.ID.String(),
			"callback_type", callback.Type,
		)
		return true
	}

	event := callback.Event

	if event.Type != "app_mention" && event.Type != "message" {
		logging.FromContext(ctx).InfoContext(ctx, "slack_webhook_event_type_ignored",
			"org_id", wctx.orgID.String(),
			"connection_id", wctx.connection.ID.String(),
			"event_type", event.Type,
		)
		return true
	}

	if event.BotID != "" || event.Subtype == "bot_message" {
		logging.FromContext(ctx).InfoContext(ctx, "slack_webhook_bot_message_ignored",
			"org_id", wctx.orgID.String(),
			"connection_id", wctx.connection.ID.String(),
			"event_type", event.Type,
			"bot_id", event.BotID,
			"subtype", event.Subtype,
		)
		return true
	}

	logSlackEvent(ctx, &event, wctx, wh.Payload)
	return true
}

func unwrapSlackEvent(payload json.RawMessage) ([]byte, map[string]string) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(payload, &probe); err != nil {
		return payload, nil
	}
	dataField, hasData := probe["data"]
	headersField, hasHeaders := probe["headers"]
	if !hasData || !hasHeaders {
		return payload, nil
	}
	headers := make(map[string]string)
	var headerProbe map[string]any
	if err := json.Unmarshal(headersField, &headerProbe); err == nil {
		for key, value := range headerProbe {
			if str, ok := value.(string); ok {
				headers[key] = str
			}
		}
	}
	return dataField, headers
}

func logSlackEvent(ctx context.Context, event *slackEvent, wctx *webhookContext, payload json.RawMessage) {
	text := event.Text
	if len(text) > 100 {
		text = text[:100] + "..."
	}

	logging.FromContext(ctx).InfoContext(ctx, "slack_webhook_received",
		"provider", slackapp.Provider,
		"org_id", wctx.orgID.String(),
		"connection_id", wctx.connection.ID.String(),
		"event_type", event.Type,
		"team_id", event.TeamID,
		"channel", event.Channel,
		"thread_ts", event.ThreadTS,
		"user", event.User,
		"user_name", event.UserName,
		"ts", event.TS,
		"text", text,
		"payload", string(payload),
	)
}
