package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type ReceiveConnectionResult struct {
	Inbound              InboundEnvelope
	Session              model.EmployeeSession
	RuntimeConversationID string
	RuntimeSessionID     string
	StreamURL            string
	RuntimeURL           string
	RuntimeAPIKey        string
	TraceID              string
	TurnID               string
	ActionToken          string
}

func (s *Service) ReceiveWebhookFromConnection(ctx context.Context, envelope WebhookEnvelope) (*ReceiveConnectionResult, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("gateway runtime messenger is not configured")
	}

	adapter, err := s.adapter(envelope.Provider)
	if err != nil {
		return nil, err
	}

	inbound, ok, err := adapter.DecodeInbound(ctx, envelope)
	if err != nil || !ok {
		return nil, fmt.Errorf("decode inbound: %w", err)
	}
	inbound.Provider = envelope.Provider

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

	route := model.EmployeeGatewayRoute{
		ID:         uuid.Nil,
		OrgID:      envelope.OrgID,
		EmployeeID: envelope.EmployeeID,
		Provider:   envelope.Provider,
	}

	event, duplicate, err := s.insertInboundEvent(ctx, route, inbound)
	if err != nil {
		return nil, err
	}
	if duplicate {
		return nil, fmt.Errorf("duplicate event")
	}

	req, err := adapter.FormatAgentRequest(ctx, inbound)
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}

	session, conversationID, err := s.findOrCreateSessionByConnection(ctx, envelope, inbound.ThreadKey)
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}

	delivery, err := s.runtime.Send(ctx, RuntimeMessage{
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
		GatewayProvider:      envelope.Provider,
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

	var sandbox model.Sandbox
	if err := s.db.WithContext(ctx).Where("id = ?", session.SandboxID).First(&sandbox).Error; err != nil {
		return nil, fmt.Errorf("load sandbox: %w", err)
	}

	actionToken := ""
	if slackAdapter, ok := adapter.(*SlackAdapter); ok {
		actionToken = slackAdapter.ActionToken(inbound.Raw)
	}

	runtimeURL := ""
	if sandbox.RuntimeURL != "" {
		runtimeURL = sandbox.RuntimeURL
	}

	return &ReceiveConnectionResult{
		Inbound:              inbound,
		Session:              session,
		RuntimeConversationID: conversationID,
		RuntimeSessionID:     delivery.SessionID,
		StreamURL:            runtimeURL + "/gateway/http/streams/" + delivery.StreamID,
		RuntimeURL:           runtimeURL,
		RuntimeAPIKey:        "",
		TraceID:              delivery.TraceID,
		TurnID:               delivery.TurnID,
		ActionToken:          actionToken,
	}, nil
}

func (s *Service) findOrCreateSessionByConnection(ctx context.Context, envelope WebhookEnvelope, threadKey string) (model.EmployeeSession, string, error) {
	conversationID := stableConversationID(envelope.ConnectionID, threadKey)
	sessionID := runtimeSessionID(conversationID)

	var sandbox model.Sandbox
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND status <> ?", envelope.OrgID, envelope.EmployeeID, "error").
		Order("created_at DESC").
		First(&sandbox).Error; err != nil {
		return model.EmployeeSession{}, "", fmt.Errorf("load employee sandbox: %w", err)
	}

	connectionID := envelope.ConnectionID
	session := model.EmployeeSession{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("org_id = ? AND employee_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			envelope.OrgID, envelope.EmployeeID, Source, envelope.ConnectionID, threadKey, "active").
			First(&session).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		session = model.EmployeeSession{
			OrgID:                 envelope.OrgID,
			EmployeeID:            envelope.EmployeeID,
			SandboxID:             sandbox.ID,
			RuntimeConversationID: sessionID,
			Source:                Source,
			SourceID:              &connectionID,
			SourceResourceKey:     threadKey,
			Status:                "active",
			Name:                  "Gateway: " + threadKey,
			IntegrationScopes:     model.JSON{},
		}
		return tx.Create(&session).Error
	})
	if err != nil {
		return model.EmployeeSession{}, "", fmt.Errorf("find or create gateway session: %w", err)
	}
	return session, conversationID, nil
}
