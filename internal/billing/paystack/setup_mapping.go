package paystack

import (
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

// paystackPlan mirrors the fields returned by GET /plan and GET /plan/:code
// that we care about. Non-exhaustive — we ignore send_invoices/send_sms/
// hosted_page/etc. because Decide doesn't track them.
type paystackPlan struct {
	ID          int64  `json:"id"`
	PlanCode    string `json:"plan_code"`
	Name        string `json:"name"`
	Amount      int64  `json:"amount"` // smallest currency subunit
	Currency    string `json:"currency"`
	Interval    string `json:"interval"`
	Description string `json:"description"`
}

// intervalToCycle maps Paystack's native interval strings onto our billing
// cycles. Intervals outside this map (daily/weekly/quarterly/biannually)
// round-trip as "" and therefore never match a spec — safe.
func intervalToCycle(interval string) billing.Cycle {
	switch interval {
	case "monthly":
		return billing.CycleMonthly
	case "annually":
		return billing.CycleAnnual
	}
	return ""
}

// cycleToInterval is the inverse, used when POST/PUT-ing plans. Errors on
// unsupported cycles rather than silently defaulting — billing configs
// shouldn't accept mystery values.
func cycleToInterval(c billing.Cycle) (string, error) {
	switch c {
	case billing.CycleMonthly:
		return "monthly", nil
	case billing.CycleAnnual:
		return "annually", nil
	}
	return "", fmt.Errorf("paystack: unsupported cycle %q", c)
}

// toExistingPlans shapes Paystack's native plan records into the
// provider-agnostic ExistingPlan type that setup.Decide consumes. Plans
// with intervals we don't manage (weekly, quarterly, etc.) are dropped —
// Decide's "ignore plans that don't match a spec" behaviour would do the
// same thing, but filtering here keeps Decide's input smaller.
func toExistingPlans(pp []paystackPlan) []setup.ExistingPlan {
	out := make([]setup.ExistingPlan, 0, len(pp))
	for _, p := range pp {
		cycle := intervalToCycle(p.Interval)
		if cycle == "" {
			continue
		}
		out = append(out, setup.ExistingPlan{
			PlanCode:    p.PlanCode,
			Name:        p.Name,
			Currency:    p.Currency,
			Cycle:       cycle,
			AmountMinor: p.Amount,
			Description: p.Description,
		})
	}
	return out
}
