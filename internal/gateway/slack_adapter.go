package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const SlackProvider = "slack"

type SlackAdapter struct{}

func NewSlackAdapter() *SlackAdapter {
	return &SlackAdapter{}
}

func (a *SlackAdapter) Provider() string {
	return SlackProvider
}

type slackEventCallback struct {
	Type      string     `json:"type"`
	TeamID    string     `json:"team_id"`
	EventID   string     `json:"event_id"`
	Event     slackEvent `json:"event"`
}

type slackEvent struct {
	Type           string                    `json:"type"`
	Team           string                    `json:"team"`
	Channel        string                    `json:"channel"`
	User           string                    `json:"user"`
	Text           string                    `json:"text"`
	TS             string                    `json:"ts"`
	ThreadTS       string                    `json:"thread_ts"`
	BotID          string                    `json:"bot_id"`
	Subtype        string                    `json:"subtype"`
	AssistantThread *slackAssistantThread    `json:"assistant_thread,omitempty"`
}

type slackAssistantThread struct {
	ActionToken string `json:"action_token"`
}

func (a *SlackAdapter) DecodeInbound(_ context.Context, envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	var callback slackEventCallback
	if err := json.Unmarshal(envelope.Body, &callback); err != nil {
		return InboundEnvelope{}, false, fmt.Errorf("decode slack event: %w", err)
	}

	if callback.Type == "url_verification" {
		return InboundEnvelope{}, false, nil
	}

	if callback.Type != "event_callback" {
		return InboundEnvelope{}, false, nil
	}

	event := callback.Event

	if event.Type != "app_mention" && event.Type != "message" {
		return InboundEnvelope{}, false, nil
	}

	if event.BotID != "" || event.Subtype == "bot_message" {
		return InboundEnvelope{}, false, nil
	}

	if strings.TrimSpace(event.Channel) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("slack event missing channel")
	}

	if strings.TrimSpace(event.TS) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("slack event missing ts")
	}

	threadTS := event.ThreadTS
	if threadTS == "" {
		threadTS = event.TS
	}

	threadKey := event.Channel + ":" + threadTS

	raw := map[string]any{
		"team_id":  callback.TeamID,
		"event_id": callback.EventID,
		"ts":       event.TS,
	}
	if event.AssistantThread != nil && event.AssistantThread.ActionToken != "" {
		raw["action_token"] = event.AssistantThread.ActionToken
	}

	return InboundEnvelope{
		Provider:          SlackProvider,
		ExternalMessageID: event.TS,
		DedupeKey:         callback.EventID,
		ThreadKey:         threadKey,
		ChannelID:         event.Channel,
		ThreadID:          threadTS,
		SenderID:          event.User,
		Text:              strings.TrimSpace(event.Text),
		Raw:               raw,
		ReceivedAt:        time.Now().UTC(),
	}, true, nil
}

func (a *SlackAdapter) FormatAgentRequest(_ context.Context, inbound InboundEnvelope) (AgentRequest, error) {
	text := strings.TrimSpace(inbound.Text)
	if text == "" {
		return AgentRequest{}, fmt.Errorf("format slack message: text is required")
	}
	sender := firstNonEmpty(inbound.SenderID, "unknown")
	return AgentRequest{
		Markdown: fmt.Sprintf("From %s in %s:\n\n%s", sender, inbound.ChannelID, text),
		Metadata: map[string]any{
			"sender_id": inbound.SenderID,
		},
	}, nil
}

func (a *SlackAdapter) RenderResponse(_ context.Context, response AgentResponse) (ProviderResponsePayload, error) {
	text := strings.TrimSpace(response.Text)
	if text == "" {
		return ProviderResponsePayload{}, fmt.Errorf("render slack response: text is required")
	}
	return ProviderResponsePayload{
		Session:   response.EmployeeSession,
		ChannelID: response.ChannelID,
		ThreadID:  response.ThreadID,
		Text:      text,
	}, nil
}

func (a *SlackAdapter) SendResponse(_ context.Context, _ ProviderResponsePayload) ([]MessageHandle, error) {
	return nil, fmt.Errorf("slack adapter SendResponse not implemented: use streaming path")
}

func (a *SlackAdapter) ActionToken(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if token, ok := raw["action_token"].(string); ok {
		return token
	}
	return ""
}
