package subscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// QuoteTTL is how long a SubscriptionChangeQuote stays valid for. Long
// enough to cover the popup → verify roundtrip, short enough that any
// in-flight quote reflects current pricing.
const QuoteTTL = 15 * time.Minute

// Service-layer errors. Handlers map these to HTTP statuses.
var (
	// ErrNoActiveSubscription is returned when the org has no active row to
	// preview a change against.
	ErrNoActiveSubscription = errors.New("subscription: no active subscription for this org")

	// ErrUnknownPlan is returned when a target slug doesn't resolve to a row.
	ErrUnknownPlan = errors.New("subscription: target plan does not exist")

	// ErrQuoteExpired / ErrQuoteConsumed / ErrQuoteWrongOrg / ErrQuoteUnknown
	// are returned by ApplyChange when the quote can't be honored.
	ErrQuoteExpired  = errors.New("subscription: quote has expired")
	ErrQuoteConsumed = errors.New("subscription: quote has already been applied")
	ErrQuoteWrongOrg = errors.New("subscription: quote belongs to a different org")
	ErrQuoteUnknown  = errors.New("subscription: quote not found")

	// ErrAmountMismatch is returned when ApplyChange's verified Paystack
	// amount doesn't match the quote amount. The customer paid the wrong
	// total — either we have a bug or someone is tampering with the popup.
	ErrAmountMismatch = errors.New("subscription: paid amount does not match quote")

	// ErrCurrencyMismatchOnVerify is returned when the verified Paystack
	// currency differs from the quote currency.
	ErrCurrencyMismatchOnVerify = errors.New("subscription: paid currency does not match quote")

	// ErrChargeRejected is returned when ResolveCheckout reports a non-success
	// status — the customer's transaction did not actually succeed.
	ErrChargeRejected = errors.New("subscription: provider reports charge did not succeed")

	// ErrUnsupportedChannel is returned when the verified payment came in
	// over a channel we don't support for subscriptions (everything except
	// card and bank).
	ErrUnsupportedChannel = errors.New("subscription: payment channel is not eligible for recurring billing")

	// ErrCannotResume / ErrCannotCancel describe state-machine refusals.
	ErrCannotResume = errors.New("subscription: cannot resume a canceled subscription")
	ErrCannotCancel = errors.New("subscription: subscription is already canceled")
)

// Service wraps the pure billing core with DB persistence and a provider
// transport. Its methods own the read-modify-write transactions that
// keep subscriptions, quotes, and the credit ledger in sync.
type Service struct {
	db       *gorm.DB
	registry *billing.Registry
	credits  *billing.CreditsService
	clock    func() time.Time
	quoteTTL time.Duration
}

// NewService returns a Service with sensible defaults.
func NewService(db *gorm.DB, registry *billing.Registry, credits *billing.CreditsService) *Service {
	return &Service{
		db:       db,
		registry: registry,
		credits:  credits,
		clock:    time.Now,
		quoteTTL: QuoteTTL,
	}
}

// SetClock injects a deterministic time source for tests.
func (s *Service) SetClock(fn func() time.Time) { s.clock = fn }

// SetQuoteTTL overrides the default quote validity window. Tests use
// this to exercise the expiry path without sleeping.
func (s *Service) SetQuoteTTL(d time.Duration) { s.quoteTTL = d }

// PreviewChange computes the proration for switching the org's active
// subscription to targetSlug, persists a quote row, and returns the
// quote view. The quote can be applied via ApplyChange.
func (s *Service) PreviewChange(ctx context.Context, orgID uuid.UUID, targetSlug string) (*model.SubscriptionChangeQuote, *ChangePreview, error) {
	now := s.clock()

	sub, err := s.activeSubscription(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}

	var target model.Plan
	if err := s.db.WithContext(ctx).
		Where("slug = ? AND active = true", targetSlug).First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrUnknownPlan
		}
		return nil, nil, fmt.Errorf("load target plan: %w", err)
	}

	preview, err := PreviewChange(viewOf(sub), planViewOf(target), now)
	if err != nil {
		return nil, nil, err
	}

	quote := model.SubscriptionChangeQuote{
		OrgID:                orgID,
		SubscriptionID:       sub.ID,
		FromPlanID:           sub.PlanID,
		ToPlanID:             target.ID,
		Kind:                 string(preview.Kind),
		AmountMinor:          preview.AmountMinor,
		Currency:             preview.Currency,
		ProrationCreditMinor: preview.CreditGrantMinor,
		EffectiveAt:          preview.EffectiveAt,
		ExpiresAt:            now.Add(s.quoteTTL),
	}
	if err := s.db.WithContext(ctx).Create(&quote).Error; err != nil {
		return nil, nil, fmt.Errorf("store quote: %w", err)
	}
	return &quote, preview, nil
}

// ---- helpers ----

func (s *Service) activeSubscription(ctx context.Context, orgID uuid.UUID) (*model.Subscription, error) {
	var sub model.Subscription
	err := s.db.WithContext(ctx).Preload("Plan").
		Where("org_id = ? AND status = ?", orgID, string(billing.StatusActive)).
		Order("created_at DESC").First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNoActiveSubscription
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) loadQuote(ctx context.Context, quoteID uuid.UUID) (*model.SubscriptionChangeQuote, error) {
	var q model.SubscriptionChangeQuote
	err := s.db.WithContext(ctx).Where("id = ?", quoteID).First(&q).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrQuoteUnknown
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func (s *Service) providerForSubscription(ctx context.Context, subID uuid.UUID) (billing.Provider, error) {
	var sub model.Subscription
	if err := s.db.WithContext(ctx).Where("id = ?", subID).First(&sub).Error; err != nil {
		return nil, err
	}
	return s.registry.Get(sub.Provider)
}

func viewOf(sub *model.Subscription) SubscriptionView {
	return SubscriptionView{
		ID:                 sub.ID,
		Plan:               planViewOf(sub.Plan),
		CurrentPeriodStart: sub.CurrentPeriodStart,
		CurrentPeriodEnd:   sub.CurrentPeriodEnd,
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
	}
}

func planViewOf(p model.Plan) PlanView {
	return PlanView{
		ID:             p.ID,
		Slug:           p.Slug,
		PriceMinor:     p.PriceCents,
		Currency:       p.Currency,
		MonthlyCredits: p.MonthlyCredits,
	}
}

func applyPaymentMethod(sub *model.Subscription, res *billing.ResolveCheckoutResult) {
	pm := res.PaymentMethod
	sub.AuthorizationCode = pm.AuthorizationCode
	sub.PaymentChannel = string(pm.Channel)
	sub.CardLast4 = pm.CardLast4
	sub.CardBrand = pm.CardBrand
	sub.CardExpMonth = pm.CardExpMonth
	sub.CardExpYear = pm.CardExpYear
	sub.PaymentBankName = pm.BankName
	sub.PaymentAccountName = pm.AccountName
	sub.LastChargeReference = res.Reference
	sub.LastChargeAmount = res.PaidAmountMinor
	sub.LastChargedAt = res.PaidAt
	if res.ExternalCustomerID != "" {
		sub.ExternalCustomerID = res.ExternalCustomerID
	}
}
