package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

const HTTPProvider = "http"

type HTTPAdapter struct {
	client *http.Client
}

type httpInboundPayload struct {
	Markdown    string `json:"markdown"`
	Text        string `json:"text"`
	ThreadID    string `json:"thread_id"`
	MessageID   string `json:"message_id"`
	ID          string `json:"id"`
	SenderID    string `json:"sender_id"`
	SenderName  string `json:"sender_name"`
	CallbackURL string `json:"callback_url"`
}

func NewHTTPAdapter(client *http.Client) *HTTPAdapter {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPAdapter{client: client}
}

func (a *HTTPAdapter) Provider() string {
	return HTTPProvider
}

func (a *HTTPAdapter) DecodeInbound(_ context.Context, envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	contentType := strings.ToLower(envelope.Headers["content-type"])
	if strings.Contains(contentType, "application/json") {
		return a.decodeJSON(envelope)
	}
	return a.decodeText(envelope)
}

func (a *HTTPAdapter) FormatAgentRequest(_ context.Context, inbound InboundEnvelope) (AgentRequest, error) {
	if strings.TrimSpace(inbound.Text) == "" {
		return AgentRequest{}, fmt.Errorf("format http gateway message: markdown is required")
	}
	metadata := map[string]any{
		"sender_id":   inbound.SenderID,
		"sender_name": inbound.SenderName,
	}
	if callbackURL, _ := inbound.Raw["callback_url"].(string); strings.TrimSpace(callbackURL) != "" {
		metadata["callback_url"] = strings.TrimSpace(callbackURL)
	}
	return AgentRequest{Markdown: inbound.Text, Metadata: metadata}, nil
}

func (a *HTTPAdapter) RenderResponse(_ context.Context, response AgentResponse) (ProviderResponsePayload, error) {
	if strings.TrimSpace(response.Text) == "" {
		return ProviderResponsePayload{}, fmt.Errorf("render http gateway response: response markdown is required")
	}
	callbackURL := httpResponseURL(response.Route, response.Raw)
	return ProviderResponsePayload{
		Route:     response.Route,
		Session:   response.EmployeeSession,
		ChannelID: response.ChannelID,
		ThreadID:  response.ThreadID,
		Text:      response.Text,
		Raw: map[string]any{
			"provider":     HTTPProvider,
			"callback_url": callbackURL,
		},
	}, nil
}

func (a *HTTPAdapter) SendResponse(ctx context.Context, payload ProviderResponsePayload) ([]MessageHandle, error) {
	callbackURL, _ := payload.Raw["callback_url"].(string)
	callbackURL = strings.TrimSpace(callbackURL)
	if callbackURL == "" {
		return nil, fmt.Errorf("send http gateway response: route config must include response_url, or allow_request_callback_url must be true and inbound payload must include callback_url")
	}
	body, err := json.Marshal(map[string]any{
		"markdown":  payload.Text,
		"text":      payload.Text,
		"thread_id": payload.ThreadID,
		"route_id":  payload.Route.ID.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("send http gateway response: encode callback payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("send http gateway response: build callback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send http gateway response: post callback: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("send http gateway response: callback returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return []MessageHandle{{
		ProviderMessageID: hashID(callbackURL + ":" + payload.ThreadID + ":" + payload.Text),
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadID,
		Raw:               map[string]any{"callback_url": callbackURL},
	}}, nil
}

func (a *HTTPAdapter) decodeJSON(envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	var payload httpInboundPayload
	if err := json.Unmarshal(envelope.Body, &payload); err != nil {
		return InboundEnvelope{}, false, fmt.Errorf("decode http gateway webhook: invalid JSON payload: %w", err)
	}
	markdown := firstNonEmpty(payload.Markdown, payload.Text)
	threadID := firstNonEmpty(payload.ThreadID, envelope.Headers["x-hivy-thread-id"])
	messageID := firstNonEmpty(payload.MessageID, payload.ID, envelope.Headers["x-hivy-message-id"])
	return httpInboundEnvelope(envelope, markdown, threadID, messageID, payload.SenderID, payload.SenderName, payload.CallbackURL, rawMap(payload))
}

func (a *HTTPAdapter) decodeText(envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	markdown := string(envelope.Body)
	threadID := envelope.Headers["x-hivy-thread-id"]
	messageID := envelope.Headers["x-hivy-message-id"]
	senderID := envelope.Headers["x-hivy-sender-id"]
	senderName := envelope.Headers["x-hivy-sender-name"]
	callbackURL := envelope.Headers["x-hivy-callback-url"]
	raw := map[string]any{
		"markdown":     markdown,
		"thread_id":    threadID,
		"message_id":   messageID,
		"sender_id":    senderID,
		"sender_name":  senderName,
		"callback_url": callbackURL,
	}
	return httpInboundEnvelope(envelope, markdown, threadID, messageID, senderID, senderName, callbackURL, raw)
}

func httpInboundEnvelope(envelope WebhookEnvelope, markdown, threadID, messageID, senderID, senderName, callbackURL string, raw map[string]any) (InboundEnvelope, bool, error) {
	if strings.TrimSpace(markdown) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("decode http gateway webhook: markdown is required; send JSON {\"markdown\":\"...\"} or a text/markdown body")
	}
	threadID = firstNonEmpty(threadID, "default")
	if messageID == "" {
		messageID = hashID(threadID + ":" + markdown)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	raw["callback_url"] = strings.TrimSpace(callbackURL)
	return InboundEnvelope{
		Provider:          HTTPProvider,
		RouteID:           envelope.RouteID,
		ExternalMessageID: messageID,
		DedupeKey:         strings.Join([]string{HTTPProvider, envelope.RouteID.String(), messageID}, ":"),
		ThreadKey:         strings.Join([]string{HTTPProvider, envelope.RouteID.String(), threadID}, ":"),
		ChannelID:         envelope.RouteID.String(),
		ThreadID:          threadID,
		SenderID:          firstNonEmpty(senderID, "http"),
		SenderName:        senderName,
		Text:              markdown,
		Raw:               raw,
		ReceivedAt:        time.Now().UTC(),
	}, true, nil
}

func httpResponseURL(route model.EmployeeGatewayRoute, raw map[string]any) string {
	if value, ok := route.Config["response_url"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if allowed, _ := route.Config["allow_request_callback_url"].(bool); allowed {
		if value, ok := raw["callback_url"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func hashID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:32]
}
