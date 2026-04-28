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

// 15 min: covers popup → verify roundtrip, short enough that pricing changes propagate.
const QuoteTTL = 15 * time.Minute

var (
	ErrNoActiveSubscription     = errors.New("subscription: no active subscription for this org")
	ErrUnknownPlan              = errors.New("subscription: target plan does not exist")
	ErrQuoteExpired             = errors.New("subscription: quote has expired")
	ErrQuoteConsumed            = errors.New("subscription: quote has already been applied")
	ErrQuoteWrongOrg            = errors.New("subscription: quote belongs to a different org")
	ErrQuoteUnknown             = errors.New("subscription: quote not found")
	ErrAmountMismatch           = errors.New("subscription: paid amount does not match quote")
	ErrCurrencyMismatchOnVerify = errors.New("subscription: paid currency does not match quote")
	ErrChargeRejected           = errors.New("subscription: provider reports charge did not succeed")
	ErrUnsupportedChannel       = errors.New("subscription: payment channel is not eligible for recurring billing")
	ErrCannotResume             = errors.New("subscription: cannot resume a canceled subscription")
	ErrCannotCancel             = errors.New("subscription: subscription is already canceled")
)

type Service struct {
	db       *gorm.DB
	registry *billing.Registry
	credits  *billing.CreditsService
	clock    func() time.Time
	quoteTTL time.Duration
}

func NewService(db *gorm.DB, registry *billing.Registry, credits *billing.CreditsService) *Service {
	return &Service{
		db:       db,
		registry: registry,
		credits:  credits,
		clock:    time.Now,
		quoteTTL: QuoteTTL,
	}
}

func (s *Service) SetClock(fn func() time.Time)    { s.clock = fn }
func (s *Service) SetQuoteTTL(d time.Duration)     { s.quoteTTL = d }

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
