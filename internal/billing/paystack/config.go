package paystack

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Config wires the Paystack adapter with credentials and a plan resolver.
//
// Plans is the bridge between our plan catalog (the plans table) and the
// Paystack-side plan codes (PLN_xxx). Production uses DBPlanResolver; tests
// can inject a fake.
type Config struct {
	// SecretKey is the Paystack secret key ("sk_live_…" or "sk_test_…").
	// Used both for API authorization and for webhook HMAC verification.
	SecretKey string

	// Plans resolves slug ↔ plan_code. Required.
	Plans PlanResolver
}

// PlanResolver bridges our plan catalog with Paystack plan codes.
type PlanResolver interface {
	// PlanCode resolves (slug, currency, cycle) → Paystack plan_code.
	// Returns billing.ErrUnknownPlan when no row matches the slug, and
	// billing.ErrUnsupportedCurrency when the row exists but its currency
	// doesn't match.
	PlanCode(slug, currency string, cycle billing.Cycle) (string, error)

	// SlugForPlanCode returns the plan slug a Paystack plan_code resolves to,
	// or an empty string when the code isn't ours. Used by the webhook parser
	// to filter out events for plans on the same Paystack account that we
	// don't manage.
	SlugForPlanCode(code string) string
}

// DBPlanResolver implements PlanResolver against the plans table.
//
// We only persist a single plan_code per row (the primary cycle/currency
// the UI transacts in — monthly NGN today). Annual or multi-currency
// support would mean a side table; deferring that until needed.
type DBPlanResolver struct {
	db *gorm.DB
}

// NewDBPlanResolver returns a resolver that reads plans from the database.
func NewDBPlanResolver(db *gorm.DB) *DBPlanResolver {
	return &DBPlanResolver{db: db}
}

func (r *DBPlanResolver) PlanCode(slug, currency string, cycle billing.Cycle) (string, error) {
	if cycle != billing.CycleMonthly {
		return "", fmt.Errorf("paystack: %w: cycle %q not supported", billing.ErrUnknownPlan, cycle)
	}
	var plan model.Plan
	err := r.db.Where("slug = ? AND active = ? AND provider = ?", slug, true, Name).First(&plan).Error
	if err != nil {
		return "", fmt.Errorf("paystack: %w: %q", billing.ErrUnknownPlan, slug)
	}
	if !strings.EqualFold(plan.Currency, currency) {
		return "", fmt.Errorf("paystack: %w: %s in %s", billing.ErrUnsupportedCurrency, slug, currency)
	}
	if plan.ProviderPlanID == "" {
		return "", fmt.Errorf("paystack: %w: %s/%s has no provider_plan_id", billing.ErrUnknownPlan, slug, currency)
	}
	return plan.ProviderPlanID, nil
}

func (r *DBPlanResolver) SlugForPlanCode(code string) string {
	if code == "" {
		return ""
	}
	var plan model.Plan
	if err := r.db.
		Where("provider = ? AND provider_plan_id = ? AND active = ?", Name, code, true).
		First(&plan).Error; err != nil {
		return ""
	}
	return plan.Slug
}
