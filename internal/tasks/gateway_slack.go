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
	db                 *gorm.DB
	nangoClient        *nango.Client
	slackClientFactory func(string) slackGatewayClient
}

func NewGatewaySlackHandler(db *gorm.DB, nangoClient *nango.Client) *GatewaySlackHandler {
	return &GatewaySlackHandler{db: db, nangoClient: nangoClient}
}

type slackGatewayClient interface {
	SetAssistantThreadsStatusContext(context.Context, slacksdk.AssistantThreadsSetStatusParameters) error
	StartStreamContext(context.Context, string, ...slacksdk.MsgOption) (string, string, error)
	AppendStreamContext(context.Context, string, string, ...slacksdk.MsgOption) (string, string, error)
	StopStreamContext(context.Context, string, string, ...slacksdk.MsgOption) (string, string, error)
	PostMessageContext(context.Context, string, ...slacksdk.MsgOption) (string, string, error)
}

const (
	slackAssistantStatus   = "is thinking..."
	slackStreamFlushBytes  = 800
	slackStreamFlushWindow = 750 * time.Millisecond
)

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

	client := h.newSlackClient(botToken)

	streamURL := payload.StreamURL
	subscriber := gateway.NewSSESubscriber(&http.Client{Timeout: 610 * time.Second})
	events, err := subscriber.Subscribe(ctx, streamURL, payload.RuntimeAPIKey)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: subscribe to stream: %w", err), fields)
		return err
	}

	text, delivered, err := h.deliverSlackResponse(ctx, payload, client, events, fields)
	if delivered {
		h.recordDelivery(ctx, payload, text)
	}
	return err
}

func (h *GatewaySlackHandler) newSlackClient(botToken string) slackGatewayClient {
	if h.slackClientFactory != nil {
		return h.slackClientFactory(botToken)
	}
	return slacksdk.New(botToken)
}

func (h *GatewaySlackHandler) deliverSlackResponse(ctx context.Context, payload GatewaySlackPayload, client slackGatewayClient, events <-chan gateway.SSEEvent, fields map[string]any) (string, bool, error) {
	statusCleared := false
	tokenCount := 0
	appendFailureCount := 0
	var streamedText strings.Builder
	var pendingText strings.Builder
	var slackStreamTS string
	streamStartFailed := false
	lastFlush := time.Now()

	h.setStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)

	for event := range events {
		if ctx.Err() != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: context cancelled"), fields)
			if slackStreamTS != "" {
				_ = h.stopStream(ctx, client, payload.ChannelID, slackStreamTS, fields)
			}
			return "", false, ctx.Err()
		}

		switch event.Type {
		case "token":
			var data struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(event.Data, &data); err != nil || data.Text == "" {
				continue
			}
			streamedText.WriteString(data.Text)
			pendingText.WriteString(data.Text)

			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
				statusCleared = true
			}

			if slackStreamTS == "" && !streamStartFailed {
				var err error
				slackStreamTS, err = h.startStream(ctx, client, payload, fields)
				if err != nil {
					streamStartFailed = true
				}
			}
			if slackStreamTS != "" && shouldFlushSlackStream(pendingText.Len(), lastFlush) {
				if err := h.flushPendingStream(ctx, client, payload.ChannelID, slackStreamTS, &pendingText, fields); err != nil {
					appendFailureCount++
				}
				lastFlush = time.Now()
			}
			tokenCount++

		case "final":
			var data struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(event.Data, &data)

			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
			}

			text := firstNonEmpty(data.Text, streamedText.String())
			if text == "" {
				text = "No response generated."
			}
			method, err := h.finishSlackResponse(ctx, client, payload, slackStreamTS, text, &streamedText, &pendingText, fields)
			if err != nil {
				return "", false, err
			}

			logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_completed",
				"connection_id", payload.ConnectionID,
				"org_id", payload.OrgID,
				"channel_id", payload.ChannelID,
				"thread_ts", payload.ThreadTS,
				"slack_stream_ts", slackStreamTS,
				"delivery_method", method,
				"token_count", tokenCount,
				"append_failure_count", appendFailureCount,
				"response_length", len(text),
			)
			return text, true, nil

		case "done":
			if slackStreamTS != "" {
				text := streamedText.String()
				if strings.TrimSpace(text) == "" {
					text = "Done."
				}
				method, err := h.finishSlackResponse(ctx, client, payload, slackStreamTS, text, &streamedText, &pendingText, fields)
				if err != nil {
					return "", false, err
				}
				logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_stream_done",
					"connection_id", payload.ConnectionID,
					"token_count", tokenCount,
					"delivery_method", method,
				)
				return text, true, nil
			}
			logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_stream_done",
				"connection_id", payload.ConnectionID,
				"token_count", tokenCount,
			)
			return "", false, nil

		case "error":
			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
			}
			if _, err := h.finishSlackResponse(ctx, client, payload, slackStreamTS, "Something went wrong. Please try again.", &streamedText, &pendingText, fields); err != nil {
				return "", false, err
			}
			logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: agent error in stream"), fields)
			return "", false, fmt.Errorf("agent error in stream")

		case "thinking", "tool_call", "tool_result", "model_usage", "turn_started", "turn_completed":
			continue
		}
	}

	if !statusCleared {
		h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
	}
	if slackStreamTS != "" && strings.TrimSpace(streamedText.String()) != "" {
		text := streamedText.String()
		method, err := h.finishSlackResponse(ctx, client, payload, slackStreamTS, text, &streamedText, &pendingText, fields)
		if err != nil {
			return "", false, err
		}
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: stream ended without final/done; finalized accumulated stream",
			"connection_id", payload.ConnectionID,
			"org_id", payload.OrgID,
			"channel_id", payload.ChannelID,
			"thread_ts", payload.ThreadTS,
			"slack_stream_ts", slackStreamTS,
			"delivery_method", method,
			"token_count", tokenCount,
			"append_failure_count", appendFailureCount,
			"response_length", len(text),
		)
		return text, true, nil
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: stream ended without final/done"), fields)
	return "", false, fmt.Errorf("stream ended without final/done")
}

func shouldFlushSlackStream(pendingBytes int, lastFlush time.Time) bool {
	return pendingBytes >= slackStreamFlushBytes || time.Since(lastFlush) >= slackStreamFlushWindow
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (h *GatewaySlackHandler) startStream(ctx context.Context, client slackGatewayClient, payload GatewaySlackPayload, fields map[string]any) (string, error) {
	opts := []slacksdk.MsgOption{slacksdk.MsgOptionTS(payload.ThreadTS)}
	if strings.TrimSpace(payload.TeamID) != "" {
		opts = append(opts, slacksdk.MsgOptionRecipientTeamID(strings.TrimSpace(payload.TeamID)))
	}
	if strings.TrimSpace(payload.SenderID) != "" {
		opts = append(opts, slacksdk.MsgOptionRecipientUserID(strings.TrimSpace(payload.SenderID)))
	}
	_, streamTS, err := client.StartStreamContext(ctx, payload.ChannelID, opts...)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: start stream: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: start stream failed, falling back to chat.postMessage",
			"channel_id", payload.ChannelID,
			"thread_ts", payload.ThreadTS,
			"error", err,
		)
		return "", err
	}
	return streamTS, nil
}

func (h *GatewaySlackHandler) finishSlackResponse(ctx context.Context, client slackGatewayClient, payload GatewaySlackPayload, slackStreamTS, text string, streamedText, pendingText *strings.Builder, fields map[string]any) (string, error) {
	if slackStreamTS != "" {
		if err := h.appendFinalSuffix(ctx, client, payload.ChannelID, slackStreamTS, text, streamedText, pendingText, fields); err != nil {
			return "", err
		}
		if err := h.stopStream(ctx, client, payload.ChannelID, slackStreamTS, fields); err == nil {
			return "stream", nil
		}
		if strings.TrimSpace(streamedText.String()) != "" {
			return "stream_unconfirmed", nil
		}
	}
	if err := h.postThreadReply(ctx, client, payload.ChannelID, payload.ThreadTS, text, fields); err != nil {
		return "", err
	}
	return "post_message", nil
}

func (h *GatewaySlackHandler) appendFinalSuffix(ctx context.Context, client slackGatewayClient, channelID, streamTS, finalText string, streamedText, pendingText *strings.Builder, fields map[string]any) error {
	streamed := streamedText.String()
	if strings.HasPrefix(finalText, streamed) && len(finalText) > len(streamed) {
		pendingText.WriteString(finalText[len(streamed):])
		streamedText.WriteString(finalText[len(streamed):])
	}
	return h.flushPendingStream(ctx, client, channelID, streamTS, pendingText, fields)
}

func (h *GatewaySlackHandler) flushPendingStream(ctx context.Context, client slackGatewayClient, channelID, streamTS string, pendingText *strings.Builder, fields map[string]any) error {
	text := pendingText.String()
	if text == "" {
		return nil
	}
	if err := h.appendStream(ctx, client, channelID, streamTS, text, fields); err != nil {
		return err
	}
	pendingText.Reset()
	return nil
}

func (h *GatewaySlackHandler) setStatus(ctx context.Context, client slackGatewayClient, channelID, threadTS string, fields map[string]any) {
	if err := client.SetAssistantThreadsStatusContext(ctx, slacksdk.AssistantThreadsSetStatusParameters{
		ChannelID:       channelID,
		ThreadTS:        threadTS,
		Status:          slackAssistantStatus,
		LoadingMessages: []string{"is thinking...", "is working on your request..."},
	}); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: set status: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: set status failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
	}
}

func (h *GatewaySlackHandler) clearStatus(ctx context.Context, client slackGatewayClient, channelID, threadTS string, fields map[string]any) {
	if err := client.SetAssistantThreadsStatusContext(ctx, slacksdk.AssistantThreadsSetStatusParameters{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Status:    "",
	}); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: clear status: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: clear status failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
	}
}

func (h *GatewaySlackHandler) appendStream(ctx context.Context, client slackGatewayClient, channelID, streamTS, text string, fields map[string]any) error {
	if _, _, err := client.AppendStreamContext(ctx, channelID, streamTS,
		slacksdk.MsgOptionMarkdownText(text),
	); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: append stream: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: append stream failed",
			"channel_id", channelID,
			"slack_stream_ts", streamTS,
			"error", err,
		)
		return err
	}
	return nil
}

func (h *GatewaySlackHandler) stopStream(ctx context.Context, client slackGatewayClient, channelID, streamTS string, fields map[string]any) error {
	if _, _, err := client.StopStreamContext(ctx, channelID, streamTS); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: stop stream: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: stop stream failed",
			"channel_id", channelID,
			"slack_stream_ts", streamTS,
			"error", err,
		)
		return err
	}
	return nil
}

func (h *GatewaySlackHandler) postThreadReply(ctx context.Context, client slackGatewayClient, channelID, threadTS, text string, fields map[string]any) error {
	if _, _, err := client.PostMessageContext(ctx, channelID,
		slacksdk.MsgOptionText(text, false),
		slacksdk.MsgOptionTS(threadTS),
	); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: post thread reply: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: post thread reply failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
		return err
	}
	return nil
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
