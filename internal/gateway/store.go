package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) loadActiveSessionByRuntimeID(ctx context.Context, runtimeID string) (model.EmployeeSession, bool, error) {
	var session model.EmployeeSession
	err := s.db.WithContext(ctx).
		Where("runtime_conversation_id = ? AND source = ? AND status = ?", runtimeID, Source, "active").
		First(&session).Error
	if err == nil {
		return session, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.EmployeeSession{}, false, nil
	}
	return model.EmployeeSession{}, false, err
}

func (s *Service) insertInboundEvent(ctx context.Context, route model.EmployeeGatewayRoute, inbound InboundEnvelope) (model.EmployeeGatewayEvent, bool, error) {
	event := model.EmployeeGatewayEvent{
		OrgID:             route.OrgID,
		EmployeeID:        route.EmployeeID,
		RouteID:           route.ID,
		Provider:          route.Provider,
		ExternalMessageID: inbound.ExternalMessageID,
		DedupeKey:         inbound.DedupeKey,
		ThreadKey:         inbound.ThreadKey,
		ChannelID:         inbound.ChannelID,
		ThreadID:          inbound.ThreadID,
		SenderID:          inbound.SenderID,
		Status:            "received",
		Payload:           rawJSON(inbound.Raw, "{}"),
		ReceivedAt:        inbound.ReceivedAt.UTC(),
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return event, false, fmt.Errorf("insert gateway event: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var existing model.EmployeeGatewayEvent
		err := s.db.WithContext(ctx).
			Where("route_id = ? AND dedupe_key = ?", route.ID, inbound.DedupeKey).
			First(&existing).Error
		return existing, true, err
	}
	return event, false, nil
}

func (s *Service) findOrCreateSession(ctx context.Context, route model.EmployeeGatewayRoute, threadKey string) (model.EmployeeSession, string, error) {
	conversationID := stableConversationID(route.ID, threadKey)
	sessionID := runtimeSessionID(conversationID)
	var sandbox model.Sandbox
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND status <> ?", route.OrgID, route.EmployeeID, "error").
		Order("created_at DESC").
		First(&sandbox).Error; err != nil {
		return model.EmployeeSession{}, "", fmt.Errorf("load employee sandbox: %w", err)
	}
	sourceID := route.ID
	session := model.EmployeeSession{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("org_id = ? AND employee_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			route.OrgID, route.EmployeeID, Source, route.ID, threadKey, "active").
			First(&session).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		session = model.EmployeeSession{
			OrgID:                 route.OrgID,
			EmployeeID:            route.EmployeeID,
			SandboxID:             sandbox.ID,
			RuntimeConversationID: sessionID,
			Source:                Source,
			SourceID:              &sourceID,
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

func (s *Service) markEventDelivered(ctx context.Context, eventID, sessionID uuid.UUID, conversationID string, delivery *RuntimeDelivery) error {
	updates := map[string]any{
		"employee_session_id":     sessionID,
		"status":                  "delivered",
		"processed_at":            s.now().UTC(),
		"runtime_conversation_id": conversationID,
		"runtime_session_id":      delivery.SessionID,
		"runtime_stream_id":       delivery.StreamID,
		"runtime_trace_id":        delivery.TraceID,
		"runtime_turn_id":         delivery.TurnID,
	}
	return s.db.WithContext(ctx).Model(&model.EmployeeGatewayEvent{}).Where("id = ?", eventID).Updates(updates).Error
}

func (s *Service) markEventFailed(ctx context.Context, eventID uuid.UUID, err error) error {
	return s.db.WithContext(ctx).Model(&model.EmployeeGatewayEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]any{"status": "failed", "error": err.Error(), "processed_at": s.now().UTC()}).Error
}

func (s *Service) loadDeliveryByDedupe(ctx context.Context, routeID uuid.UUID, dedupe string) (*model.EmployeeGatewayDelivery, bool, error) {
	var delivery model.EmployeeGatewayDelivery
	err := s.db.WithContext(ctx).Where("route_id = ? AND dedupe_key = ?", routeID, dedupe).First(&delivery).Error
	if err == nil {
		return &delivery, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	return nil, false, err
}

func (s *Service) loadLatestEventForSession(ctx context.Context, sessionID uuid.UUID) (model.EmployeeGatewayEvent, bool, error) {
	var event model.EmployeeGatewayEvent
	err := s.db.WithContext(ctx).
		Where("employee_session_id = ?", sessionID).
		Order("received_at DESC, created_at DESC").
		First(&event).Error
	if err == nil {
		return event, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.EmployeeGatewayEvent{}, false, nil
	}
	return model.EmployeeGatewayEvent{}, false, fmt.Errorf("load latest gateway event: %w", err)
}

func (s *Service) insertDelivery(ctx context.Context, route model.EmployeeGatewayRoute, session model.EmployeeSession, response AgentResponse, dedupe string, handles []MessageHandle, status string, errText string) (*model.EmployeeGatewayDelivery, error) {
	row := model.EmployeeGatewayDelivery{
		OrgID:             route.OrgID,
		EmployeeID:        route.EmployeeID,
		RouteID:           route.ID,
		EmployeeSessionID: session.ID,
		Provider:          route.Provider,
		DedupeKey:         dedupe,
		RuntimeSessionID:  response.RuntimeSessionID,
		RuntimeTraceID:    response.TraceID,
		RuntimeTurnID:     response.TurnID,
		ThreadKey:         session.SourceResourceKey,
		ResponseText:      response.Text,
		ProviderHandles:   handlesJSON(handles),
		Status:            status,
		Error:             errText,
		ChannelID:         response.ChannelID,
		ThreadID:          response.ThreadID,
	}
	if len(handles) > 0 {
		row.ChannelID = handles[0].ChannelID
		row.ThreadID = handles[0].ThreadID
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("insert gateway delivery: %w", err)
	}
	return &row, nil
}
