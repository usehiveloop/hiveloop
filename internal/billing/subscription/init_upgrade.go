package subscription

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type UpgradeInit struct {
	AccessCode  string
	Reference   string
	AmountMinor int64
	Currency    string
}

func (s *Service) InitUpgrade(ctx context.Context, orgID uuid.UUID, quoteID uuid.UUID) (*UpgradeInit, error) {
	quote, err := s.loadQuote(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	if quote.OrgID != orgID {
		return nil, ErrQuoteWrongOrg
	}
	if quote.ConsumedAt != nil {
		return nil, ErrQuoteConsumed
	}
	if s.clock().After(quote.ExpiresAt) {
		return nil, ErrQuoteExpired
	}
	if quote.Kind != string(KindUpgrade) {
		return nil, ErrNotAnUpgrade
	}

	var sub model.Subscription
	if err := s.db.WithContext(ctx).Where("id = ?", quote.SubscriptionID).First(&sub).Error; err != nil {
		return nil, fmt.Errorf("load subscription: %w", err)
	}
	provider, err := s.registry.Get(sub.Provider)
	if err != nil {
		return nil, err
	}

	email, err := s.lookupOrgOwnerEmail(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("lookup org owner email: %w", err)
	}

	session, err := provider.CreateCheckout(ctx, sub.ExternalCustomerID, billing.CheckoutIntent{
		OrgID:         orgID,
		CustomerEmail: email,
		AmountMinor:   quote.AmountMinor,
		Currency:      quote.Currency,
		Metadata: map[string]string{
			"org_id":   orgID.String(),
			"quote_id": quote.ID.String(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("init upgrade transaction: %w", err)
	}

	return &UpgradeInit{
		AccessCode:  session.AccessCode,
		Reference:   session.Reference,
		AmountMinor: quote.AmountMinor,
		Currency:    quote.Currency,
	}, nil
}
