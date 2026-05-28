package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

type GatewaySlackHandler struct {
	db          *gorm.DB
	nangoClient *nango.Client
}

func NewGatewaySlackHandler(db *gorm.DB, nangoClient *nango.Client) *GatewaySlackHandler {
	return &GatewaySlackHandler{db: db, nangoClient: nangoClient}
}

func (h *GatewaySlackHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var payload GatewaySlackPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: unmarshal payload: %w", err), map[string]any{
			"task_type": t.Type(),
		})
		return err
	}

	fields := map[string]any{
		"connection_id": payload.ConnectionID,
		"org_id":        payload.OrgID,
		"employee_id":   payload.EmployeeID,
		"channel_id":    payload.ChannelID,
		"thread_ts":     payload.ThreadTS,
		"session_id":    payload.SessionID,
		"trace_id":      payload.TraceID,
		"turn_id":       payload.TurnID,
	}

	botToken, err := h.loadBotToken(ctx, payload.NangoConnID, payload.ProviderKey)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: load bot token: %w", err), fields)
		return err
	}

	client := slacksdk.New(botToken)

	streamURL := payload.StreamURL
	subscriber := gateway.NewSSESubscriber(&http.Client{Timeout: 610 * time.Second})
	events, err := subscriber.Subscribe(ctx, streamURL, payload.RuntimeAPIKey)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: subscribe to stream: %w", err), fields)
		return err
	}

	statusCleared := false
	tokenCount := 0

	for event := range events {
		if ctx.Err() != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: context cancelled"), fields)
			h.stopStream(ctx, client, payload.ChannelID, payload.ThreadTS, "Request cancelled.")
			return ctx.Err()
		}

		switch event.Type {
		case "token":
			var data struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(event.Data, &data); err != nil || data.Text == "" {
				continue
			}

			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS)
				statusCleared = true
			}

			h.appendStream(ctx, client, payload.ChannelID, payload.ThreadTS, data.Text)
			tokenCount++

		case "final":
			var data struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(event.Data, &data)

			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS)
			}

			text := data.Text
			if text == "" {
				text = "No response generated."
			}
			h.stopStream(ctx, client, payload.ChannelID, payload.ThreadTS, text)
			h.recordDelivery(ctx, payload, text)

			logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_completed",
				"connection_id", payload.ConnectionID,
				"org_id", payload.OrgID,
				"channel_id", payload.ChannelID,
				"thread_ts", payload.ThreadTS,
				"token_count", tokenCount,
				"response_length", len(text),
			)
			return nil

		case "done":
			logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_stream_done",
				"connection_id", payload.ConnectionID,
				"token_count", tokenCount,
			)
			return nil

		case "error":
			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS)
			}
			h.stopStream(ctx, client, payload.ChannelID, payload.ThreadTS, "Something went wrong. Please try again.")
			logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: agent error in stream"), fields)
			return fmt.Errorf("agent error in stream")

		case "thinking", "tool_call", "tool_result", "model_usage", "turn_started", "turn_completed":
			continue
		}
	}

	if !statusCleared {
		h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS)
	}
	h.stopStream(ctx, client, payload.ChannelID, payload.ThreadTS, "Response timed out. Please try again.")
	logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: stream ended without final/done"), fields)
	return fmt.Errorf("stream ended without final/done")
}

func (h *GatewaySlackHandler) clearStatus(ctx context.Context, client *slacksdk.Client, channelID, threadTS string) {
	if err := client.SetAssistantThreadsStatusContext(ctx, slacksdk.AssistantThreadsSetStatusParameters{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Status:    "",
	}); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: clear status failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
	}
}

func (h *GatewaySlackHandler) appendStream(ctx context.Context, client *slacksdk.Client, channelID, threadTS, text string) {
	if _, _, err := client.AppendStreamContext(ctx, channelID, threadTS,
		slacksdk.MsgOptionMarkdownText(text),
	); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: append stream failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
	}
}

func (h *GatewaySlackHandler) stopStream(ctx context.Context, client *slacksdk.Client, channelID, threadTS, text string) {
	if _, _, err := client.StopStreamContext(ctx, channelID, threadTS,
		slacksdk.MsgOptionMarkdownText(text),
	); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: stop stream failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
	}
}

func (h *GatewaySlackHandler) loadBotToken(ctx context.Context, nangoConnID, providerKey string) (string, error) {
	nangoConn, err := h.nangoClient.GetConnection(ctx, nangoConnID, providerKey)
	if err != nil {
		return "", fmt.Errorf("load nango connection: %w", err)
	}
	creds, _ := nangoConn["credentials"].(map[string]any)
	for _, key := range []string{"bot_token", "access_token"} {
		if token, ok := creds[key].(string); ok && strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}
	return "", fmt.Errorf("no bot token in nango credentials")
}

func (h *GatewaySlackHandler) recordDelivery(ctx context.Context, payload GatewaySlackPayload, text string) {
	orgID, _ := parseUUID(payload.OrgID)
	employeeID, _ := parseUUID(payload.EmployeeID)
	sessionID, _ := parseUUID(payload.SessionID)

	delivery := model.EmployeeGatewayDelivery{
		OrgID:             orgID,
		EmployeeID:        employeeID,
		Provider:          gateway.SlackProvider,
		DedupeKey:         payload.TraceID + ":" + payload.TurnID,
		RuntimeSessionID:  payload.SessionID,
		RuntimeTraceID:    payload.TraceID,
		RuntimeTurnID:     payload.TurnID,
		ThreadKey:         payload.ChannelID + ":" + payload.ThreadTS,
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadTS,
		ResponseText:      text,
		Status:            "sent",
		EmployeeSessionID: sessionID,
	}

	if err := h.db.WithContext(ctx).Create(&delivery).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: record delivery: %w", err), map[string]any{
			"connection_id": payload.ConnectionID,
			"org_id":        payload.OrgID,
			"channel_id":    payload.ChannelID,
			"thread_ts":     payload.ThreadTS,
		})
	}
}

func parseUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(s)
}
