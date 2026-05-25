package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const FakeSlackProvider = "fake-slack"

type FakeSlackAdapter struct {
	mu   sync.Mutex
	sent []ProviderResponsePayload
}

type fakeSlackWebhook struct {
	Type  string         `json:"type"`
	Event fakeSlackEvent `json:"event"`
}

type fakeSlackEvent struct {
	Type      string `json:"type"`
	TeamID    string `json:"team_id"`
	ChannelID string `json:"channel"`
	UserID    string `json:"user"`
	UserName  string `json:"user_name"`
	Text      string `json:"text"`
	TS        string `json:"ts"`
	ThreadTS  string `json:"thread_ts"`
	BotID     string `json:"bot_id"`
	Subtype   string `json:"subtype"`
}

func NewFakeSlackAdapter() *FakeSlackAdapter {
	return &FakeSlackAdapter{}
}

func (a *FakeSlackAdapter) Provider() string {
	return FakeSlackProvider
}

func (a *FakeSlackAdapter) DecodeInbound(_ context.Context, envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	var payload fakeSlackWebhook
	if err := json.Unmarshal(envelope.Body, &payload); err != nil {
		return InboundEnvelope{}, false, fmt.Errorf("decode fake-slack webhook: invalid JSON payload: %w", err)
	}
	if payload.Type == "url_verification" {
		return InboundEnvelope{}, false, nil
	}
	event := payload.Event
	if event.Type != "app_mention" && event.Type != "message" {
		return InboundEnvelope{}, false, nil
	}
	if event.BotID != "" || event.Subtype == "bot_message" {
		return InboundEnvelope{}, false, nil
	}
	if strings.TrimSpace(event.ChannelID) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("decode fake-slack webhook: event.channel is required")
	}
	if strings.TrimSpace(event.TS) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("decode fake-slack webhook: event.ts is required")
	}
	threadID := firstNonEmpty(event.ThreadTS, event.TS)
	teamID := firstNonEmpty(event.TeamID, "workspace")
	return InboundEnvelope{
		Provider:          FakeSlackProvider,
		RouteID:           envelope.RouteID,
		ExternalMessageID: event.TS,
		DedupeKey:         strings.Join([]string{FakeSlackProvider, teamID, event.ChannelID, event.TS}, ":"),
		ThreadKey:         strings.Join([]string{FakeSlackProvider, teamID, event.ChannelID, threadID}, ":"),
		ChannelID:         event.ChannelID,
		ThreadID:          threadID,
		SenderID:          event.UserID,
		SenderName:        event.UserName,
		Text:              strings.TrimSpace(event.Text),
		Raw:               rawMap(payload),
		ReceivedAt:        time.Now().UTC(),
	}, true, nil
}

func (a *FakeSlackAdapter) FormatAgentRequest(_ context.Context, inbound InboundEnvelope) (AgentRequest, error) {
	text := strings.TrimSpace(inbound.Text)
	if text == "" {
		return AgentRequest{}, fmt.Errorf("format fake-slack message: text is required")
	}
	sender := firstNonEmpty(inbound.SenderName, inbound.SenderID, "unknown")
	return AgentRequest{
		Markdown: fmt.Sprintf("From %s in %s:\n\n%s", sender, inbound.ChannelID, text),
		Metadata: map[string]any{
			"sender_id":   inbound.SenderID,
			"sender_name": inbound.SenderName,
		},
	}, nil
}

func (a *FakeSlackAdapter) RenderResponse(_ context.Context, response AgentResponse) (ProviderResponsePayload, error) {
	text := strings.TrimSpace(response.Text)
	if text == "" {
		return ProviderResponsePayload{}, fmt.Errorf("render fake-slack response: response text is required")
	}
	return ProviderResponsePayload{
		Route:     response.Route,
		Session:   response.EmployeeSession,
		ChannelID: response.ChannelID,
		ThreadID:  response.ThreadID,
		Text:      text,
		Blocks: []map[string]any{
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": text,
				},
			},
		},
		Raw: map[string]any{"provider": FakeSlackProvider},
	}, nil
}

func (a *FakeSlackAdapter) SendResponse(_ context.Context, payload ProviderResponsePayload) ([]MessageHandle, error) {
	if strings.TrimSpace(payload.Text) == "" {
		return nil, fmt.Errorf("send fake-slack response: text is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = append(a.sent, payload)
	index := len(a.sent)
	return []MessageHandle{{
		ProviderMessageID: fmt.Sprintf("fake-slack-%d", index),
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadID,
		Raw:               map[string]any{"index": index},
	}}, nil
}

func (a *FakeSlackAdapter) SentMessages() []ProviderResponsePayload {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ProviderResponsePayload, len(a.sent))
	copy(out, a.sent)
	return out
}

func rawMap(value any) map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return map[string]any{}
	}
	return decoded
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
