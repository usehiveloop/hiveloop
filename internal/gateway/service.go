package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type Service struct {
	db       *gorm.DB
	runtime  RuntimeMessenger
	adapters map[string]Adapter
	now      func() time.Time
}

func NewService(db *gorm.DB, runtime RuntimeMessenger, adapters ...Adapter) *Service {
	s := &Service{
		db:       db,
		runtime:  runtime,
		adapters: map[string]Adapter{},
		now:      time.Now,
	}
	for _, adapter := range adapters {
		s.RegisterAdapter(adapter)
	}
	return s
}

func (s *Service) RegisterAdapter(adapter Adapter) {
	if adapter == nil {
		return
	}
	s.adapters[strings.ToLower(strings.TrimSpace(adapter.Provider()))] = adapter
}

func (s *Service) ReceiveWebhook(ctx context.Context, envelope WebhookEnvelope) (*ReceiveResult, error) {
	route, err := s.loadRoute(ctx, envelope.RouteID)
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapter(route.Provider)
	if err != nil {
		return nil, err
	}
	inbound, ok, err := adapter.DecodeInbound(ctx, envelope)
	if err != nil || !ok {
		return &ReceiveResult{Ignored: !ok}, err
	}
	inbound.RouteID = route.ID
	if inbound.Provider == "" {
		inbound.Provider = route.Provider
	}
	return s.Receive(ctx, inbound)
}

func (s *Service) Receive(ctx context.Context, inbound InboundEnvelope) (*ReceiveResult, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("gateway runtime messenger is not configured")
	}
	route, err := s.loadRoute(ctx, inbound.RouteID)
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapter(route.Provider)
	if err != nil {
		return nil, err
	}
	if inbound.IsFromBot {
		return &ReceiveResult{Ignored: true}, nil
	}
	if strings.TrimSpace(inbound.ThreadKey) == "" {
		return nil, fmt.Errorf("gateway inbound thread key is required")
	}
	if strings.TrimSpace(inbound.DedupeKey) == "" {
		inbound.DedupeKey = inbound.ExternalMessageID
	}
	if strings.TrimSpace(inbound.DedupeKey) == "" {
		return nil, fmt.Errorf("gateway inbound dedupe key is required")
	}
	if inbound.ReceivedAt.IsZero() {
		inbound.ReceivedAt = s.now().UTC()
	}

	event, duplicate, err := s.insertInboundEvent(ctx, route, inbound)
	if err != nil || duplicate {
		return &ReceiveResult{Event: event, Duplicate: duplicate}, err
	}

	req, err := adapter.FormatAgentRequest(ctx, inbound)
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}
	session, conversationID, err := s.findOrCreateSession(ctx, route, inbound.ThreadKey)
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}
	delivery, err := s.runtime.Send(ctx, RuntimeMessage{
		Route:                route,
		Session:              session,
		Text:                 req.Markdown,
		User:                 inbound.SenderID,
		UserDisplayName:      inbound.SenderName,
		ConversationID:       conversationID,
		GatewayEventID:       event.ID,
		GatewayDedupeKey:     inbound.DedupeKey,
		GatewayThreadKey:     inbound.ThreadKey,
		GatewayChannelID:     inbound.ChannelID,
		GatewayThreadID:      inbound.ThreadID,
		GatewayExternalMsgID: inbound.ExternalMessageID,
		GatewayProvider:      route.Provider,
		Metadata:             req.Metadata,
	})
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}
	if delivery == nil {
		delivery = &RuntimeDelivery{}
	}
	if err := s.markEventDelivered(ctx, event.ID, session.ID, conversationID, delivery); err != nil {
		return nil, err
	}
	event.EmployeeSessionID = &session.ID
	event.RuntimeConversationID = conversationID
	event.RuntimeSessionID = delivery.SessionID
	event.RuntimeStreamID = delivery.StreamID
	event.RuntimeTraceID = delivery.TraceID
	event.RuntimeTurnID = delivery.TurnID
	event.Status = "delivered"
	return &ReceiveResult{Event: event, Session: session, Runtime: *delivery}, nil
}

func (s *Service) HandleRuntimeFinal(ctx context.Context, response AgentResponse) (*model.EmployeeGatewayDelivery, error) {
	session := response.EmployeeSession
	if session.ID == uuid.Nil {
		loaded, found, err := s.loadActiveSessionByRuntimeID(ctx, response.RuntimeSessionID)
		if err != nil {
			return nil, fmt.Errorf("load gateway session: %w", err)
		}
		if !found {
			return nil, nil
		}
		session = loaded
	}
	if session.Source != Source || session.SourceID == nil {
		return nil, nil
	}
	route, err := s.loadRoute(ctx, *session.SourceID)
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapter(route.Provider)
	if err != nil {
		return nil, err
	}
	response.RouteID = route.ID
	response.Route = route
	response.EmployeeSession = session
	if response.RuntimeSessionID == "" {
		response.RuntimeSessionID = session.RuntimeConversationID
	}
	if response.ChannelID == "" || response.ThreadID == "" {
		if event, found, err := s.loadLatestEventForSession(ctx, session.ID); err != nil {
			return nil, err
		} else if found {
			if response.ChannelID == "" {
				response.ChannelID = event.ChannelID
			}
			if response.ThreadID == "" {
				response.ThreadID = event.ThreadID
			}
		}
	}
	dedupe := outboundDedupeKey(response)
	if existing, ok, err := s.loadDeliveryByDedupe(ctx, route.ID, dedupe); err != nil || ok {
		return existing, err
	}
	payload, err := adapter.RenderResponse(ctx, response)
	if err != nil {
		return s.insertDelivery(ctx, route, session, response, dedupe, nil, "failed", err.Error())
	}
	handles, err := adapter.SendResponse(ctx, payload)
	if err != nil {
		return s.insertDelivery(ctx, route, session, response, dedupe, handles, "failed", err.Error())
	}
	return s.insertDelivery(ctx, route, session, response, dedupe, handles, "sent", "")
}

func (s *Service) loadRoute(ctx context.Context, id uuid.UUID) (model.EmployeeGatewayRoute, error) {
	var route model.EmployeeGatewayRoute
	if err := s.db.WithContext(ctx).
		Where("id = ? AND enabled = true AND revoked_at IS NULL", id).
		First(&route).Error; err != nil {
		return route, fmt.Errorf("load gateway route: %w", err)
	}
	return route, nil
}

func (s *Service) adapter(provider string) (Adapter, error) {
	adapter := s.adapters[strings.ToLower(strings.TrimSpace(provider))]
	if adapter == nil {
		return nil, fmt.Errorf("gateway adapter %q is not registered", provider)
	}
	return adapter, nil
}
