package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
)

// Service orchestrates subscription writes. It owns the DB handle, the catalog
// (for resource-type lookup), and an AgentProviderResolver for the
// integration-access check.
type Service struct {
	db       *gorm.DB
	catalog  *catalog.Catalog
	resolver *AgentProviderResolver
}

// NewService constructs a Service. Nil dependencies panic — this is a
// constructor bug, not a runtime condition.
func NewService(db *gorm.DB, cat *catalog.Catalog) *Service {
	if db == nil || cat == nil {
		panic("subscriptions.NewService: db and catalog are required")
	}
	return &Service{
		db:       db,
		catalog:  cat,
		resolver: NewAgentProviderResolver(db),
	}
}

// SubscribeRequest captures the parsed input of an agent's subscribe_to_events
// tool call.
type SubscribeRequest struct {
	OrgID          uuid.UUID
	AgentID        uuid.UUID
	ConversationID uuid.UUID
	ResourceType   string
	ResourceID     string
}

// SubscribeResult is returned on successful (or idempotent) subscription.
type SubscribeResult struct {
	SubscriptionID uuid.UUID
	ResourceKey    string
	ResourceType   string
	ResourceID     string
	Provider       string
	Idempotent     bool // true when the subscription already existed
	Events         []string
}

// Errors returned by Subscribe. Each is crafted to be surfaced directly to the
// agent — the phrasing names the expected format / missing integration / etc.
// so the agent can self-correct on the next turn.
var (
	ErrUnknownResourceType = errors.New("unknown resource_type")
	ErrIntegrationMissing  = errors.New("agent does not have the required integration")
	ErrInvalidResourceID   = errors.New("resource_id does not match expected format")
)

// Subscribe validates the request and writes (or finds existing) subscription.
// Flow:
//  1. Look up resource_type in the catalog → get provider + pattern/template.
//  2. Check the agent has a connection for that provider.
//  3. Parse resource_id against the pattern → canonical key.
//  4. Upsert into conversation_subscriptions (idempotent on conv+key).
func (svc *Service) Subscribe(ctx context.Context, req SubscribeRequest) (*SubscribeResult, error) {
	if req.OrgID == uuid.Nil || req.AgentID == uuid.Nil || req.ConversationID == uuid.Nil {
		return nil, errors.New("org_id, agent_id, and conversation_id are all required")
	}
	if req.ResourceType == "" {
		return nil, fmt.Errorf("%w: resource_type is required", ErrInvalidResourceID)
	}

	provider, def, ok := svc.catalog.GetSubscribableResource(req.ResourceType)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownResourceType, req.ResourceType)
	}

	agent, err := svc.loadAgent(ctx, req.AgentID, req.OrgID)
	if err != nil {
		return nil, err
	}

	hasProvider, err := svc.resolver.HasProvider(ctx, agent, provider)
	if err != nil {
		return nil, fmt.Errorf("checking agent providers: %w", err)
	}
	if !hasProvider {
		return nil, fmt.Errorf(
			"%w: resource_type %q requires the %q integration on this agent",
			ErrIntegrationMissing, req.ResourceType, provider,
		)
	}

	parsed, err := ParseResourceID(def, req.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResourceID, err)
	}

	sub, idempotent, err := svc.upsertActiveSubscription(ctx, model.ConversationSubscription{
		ID:             uuid.New(),
		ConversationID: req.ConversationID,
		OrgID:          req.OrgID,
		AgentID:        req.AgentID,
		Provider:       provider,
		ResourceType:   req.ResourceType,
		ResourceID:     req.ResourceID,
		ResourceKey:    parsed.CanonicalKey,
		Source:         model.SubscriptionSourceAgent,
		Status:         model.SubscriptionStatusActive,
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}

	return &SubscribeResult{
		SubscriptionID: sub.ID,
		ResourceKey:    sub.ResourceKey,
		ResourceType:   sub.ResourceType,
		ResourceID:     sub.ResourceID,
		Provider:       provider,
		Idempotent:     idempotent,
		Events:         def.Events,
	}, nil
}

// upsertActiveSubscription implements idempotent insert for subscriptions.
// The schema enforces uniqueness of (conversation_id, resource_key) among
// active rows via a partial unique index; partial indexes can't be targeted
// by Postgres ON CONFLICT without matching the predicate, which GORM
// doesn't expose cleanly, so we do a transactional find-or-create instead.
//
// Returns the final stored row, whether it already existed (idempotent=true),
// and any error.
func (svc *Service) upsertActiveSubscription(ctx context.Context, candidate model.ConversationSubscription) (model.ConversationSubscription, bool, error) {
	var result model.ConversationSubscription
	var idempotent bool

	err := svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.ConversationSubscription
		findErr := tx.Where(
			"conversation_id = ? AND resource_key = ? AND status = ?",
			candidate.ConversationID, candidate.ResourceKey, model.SubscriptionStatusActive,
		).First(&existing).Error

		switch {
		case findErr == nil:
			result = existing
			idempotent = true
			return nil
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			if err := tx.Create(&candidate).Error; err != nil {
				return fmt.Errorf("inserting subscription: %w", err)
			}
			result = candidate
			return nil
		default:
			return fmt.Errorf("checking for existing subscription: %w", findErr)
		}
	})
	if err != nil {
		return model.ConversationSubscription{}, false, err
	}
	return result, idempotent, nil
}

// ListActive returns all active subscriptions for a conversation, ordered by
// creation time ascending.
func (svc *Service) ListActive(ctx context.Context, conversationID uuid.UUID) ([]model.ConversationSubscription, error) {
	var subs []model.ConversationSubscription
	err := svc.db.WithContext(ctx).
		Where("conversation_id = ? AND status = ?", conversationID, model.SubscriptionStatusActive).
		Order("created_at ASC").
		Find(&subs).Error
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions: %w", err)
	}
	return subs, nil
}

// ProvidersForAgent exposes the resolver so callers (e.g. the MCP tool
// handler) can render "available providers" in error messages without
// building a second resolver instance.
func (svc *Service) ProvidersForAgent(ctx context.Context, agent *model.Agent) ([]string, error) {
	return svc.resolver.Providers(ctx, agent)
}

// loadAgent fetches the agent row, scoped to the org for safety.
func (svc *Service) loadAgent(ctx context.Context, agentID, orgID uuid.UUID) (*model.Agent, error) {
	var agent model.Agent
	query := svc.db.WithContext(ctx).Where("id = ?", agentID)
	// System agents have org_id = NULL; user agents must match the calling org.
	query = query.Where("(org_id = ? OR org_id IS NULL)", orgID)
	if err := query.First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("agent %s not found in org %s", agentID, orgID)
		}
		return nil, fmt.Errorf("loading agent: %w", err)
	}
	return &agent, nil
}

