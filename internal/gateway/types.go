package gateway

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const Source = "gateway"

type WebhookEnvelope struct {
	RouteID uuid.UUID
	Headers map[string]string
	Body    []byte
}

type InboundEnvelope struct {
	Provider          string
	RouteID           uuid.UUID
	ExternalMessageID string
	DedupeKey         string
	ThreadKey         string
	ChannelID         string
	ThreadID          string
	SenderID          string
	SenderName        string
	Text              string
	IsFromBot         bool
	Raw               map[string]any
	ReceivedAt        time.Time
}

type AgentRequest struct {
	Markdown string
	Metadata map[string]any
}

type AgentResponse struct {
	RouteID          uuid.UUID
	Route            model.EmployeeGatewayRoute
	EmployeeSession  model.EmployeeConversation
	RuntimeSessionID string
	TraceID          string
	TurnID           string
	ChannelID        string
	ThreadID         string
	Text             string
	Raw              map[string]any
}

type ProviderResponsePayload struct {
	Route     model.EmployeeGatewayRoute
	Session   model.EmployeeConversation
	ChannelID string
	ThreadID  string
	Text      string
	Blocks    []map[string]any
	Raw       map[string]any
}

type MessageHandle struct {
	ProviderMessageID string         `json:"provider_message_id"`
	ChannelID         string         `json:"channel_id"`
	ThreadID          string         `json:"thread_id"`
	Raw               map[string]any `json:"raw,omitempty"`
}

type Adapter interface {
	Provider() string
	DecodeInbound(context.Context, WebhookEnvelope) (InboundEnvelope, bool, error)
	FormatAgentRequest(context.Context, InboundEnvelope) (AgentRequest, error)
	RenderResponse(context.Context, AgentResponse) (ProviderResponsePayload, error)
	SendResponse(context.Context, ProviderResponsePayload) ([]MessageHandle, error)
}

type RuntimeMessage struct {
	Route                model.EmployeeGatewayRoute
	Session              model.EmployeeConversation
	Text                 string
	User                 string
	UserDisplayName      string
	ConversationID       string
	GatewayEventID       uuid.UUID
	GatewayDedupeKey     string
	GatewayThreadKey     string
	GatewayChannelID     string
	GatewayThreadID      string
	GatewayExternalMsgID string
	GatewayProvider      string
	Metadata             map[string]any
}

type RuntimeDelivery struct {
	SessionID string
	StreamID  string
	TraceID   string
	TurnID    string
}

type RuntimeMessenger interface {
	Send(context.Context, RuntimeMessage) (*RuntimeDelivery, error)
}

type ReceiveResult struct {
	Event     model.EmployeeGatewayEvent
	Session   model.EmployeeConversation
	Runtime   RuntimeDelivery
	Duplicate bool
	Ignored   bool
}
