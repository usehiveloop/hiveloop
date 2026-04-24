package paystack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

// Reconciler implements setup.PlanReconciler against Paystack's /plan API.
// It's safe for concurrent use of Reconcile across different callers;
// internally, operations within one Reconcile pass are sequential and
// rate-limited.
type Reconciler struct {
	client  *client
	limiter *rate.Limiter
	dryRun  bool
	logger  *slog.Logger
}

// ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*Reconciler)

// WithDryRun makes the reconciler log every intended action without making
// any mutating API calls. Lists still happen (we need them to decide).
func WithDryRun() ReconcilerOption { return func(r *Reconciler) { r.dryRun = true } }

// WithLogger replaces the default slog.Default() logger.
func WithLogger(l *slog.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		if l != nil {
			r.logger = l
		}
	}
}

// WithRateLimit overrides the default 1 request/second limiter.
// Pass a nil limiter to disable rate-limiting entirely (tests use this).
func WithRateLimit(l *rate.Limiter) ReconcilerOption {
	return func(r *Reconciler) { r.limiter = l }
}

// NewReconciler builds a Reconciler using the given Paystack secret key.
func NewReconciler(secretKey string, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client:  newClient(secretKey),
		limiter: rate.NewLimiter(rate.Every(time.Second), 1),
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Reconcile implements setup.PlanReconciler.
//
// Flow: list all existing plans (paginated) → hand them + desired specs to
// setup.Decide → execute each action (create/update/noop) → return the
// resolved plan_codes. Fails fast on the first mutation error; the script
// is idempotent so operators can simply re-run.
func (r *Reconciler) Reconcile(ctx context.Context, specs []setup.PlanSpec) ([]setup.ResolvedPlan, error) {
	existing, err := r.listAllPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	r.logger.InfoContext(ctx, "paystack setup: fetched existing plans", "count", len(existing))

	actions := setup.Decide(toExistingPlans(existing), specs)

	resolved := make([]setup.ResolvedPlan, 0, len(actions))
	for _, action := range actions {
		rp, err := r.apply(ctx, action)
		if err != nil {
			return nil, fmt.Errorf("apply %s for %s/%s/%s: %w",
				action.Kind, action.Spec.Slug, action.Spec.Currency, action.Spec.Cycle, err)
		}
		resolved = append(resolved, rp)
	}
	return resolved, nil
}

func (r *Reconciler) apply(ctx context.Context, action setup.Action) (setup.ResolvedPlan, error) {
	spec := action.Spec
	attrs := []any{
		"slug", spec.Slug, "currency", spec.Currency, "cycle", string(spec.Cycle),
		"name", spec.Name, "amount_minor", spec.AmountMinor,
	}

	switch action.Kind {
	case setup.ActionNoOp:
		r.logger.InfoContext(ctx, "paystack setup: noop", append(attrs, "plan_code", action.ExistingCode)...)
		return setup.ResolvedPlan{Key: spec.Key(), PlanCode: action.ExistingCode, Action: setup.ActionNoOp}, nil

	case setup.ActionCreate:
		if r.dryRun {
			r.logger.InfoContext(ctx, "paystack setup: would create", attrs...)
			return setup.ResolvedPlan{Key: spec.Key(), PlanCode: "(dry-run)", Action: setup.ActionCreate}, nil
		}
		code, err := r.createPlan(ctx, spec)
		if err != nil {
			return setup.ResolvedPlan{}, err
		}
		r.logger.InfoContext(ctx, "paystack setup: created", append(attrs, "plan_code", code)...)
		return setup.ResolvedPlan{Key: spec.Key(), PlanCode: code, Action: setup.ActionCreate}, nil

	case setup.ActionUpdate:
		attrs = append(attrs, "plan_code", action.ExistingCode, "drift_fields", action.DriftFields)
		if r.dryRun {
			r.logger.InfoContext(ctx, "paystack setup: would update", attrs...)
			return setup.ResolvedPlan{Key: spec.Key(), PlanCode: action.ExistingCode, Action: setup.ActionUpdate}, nil
		}
		if err := r.updatePlan(ctx, action.ExistingCode, spec); err != nil {
			return setup.ResolvedPlan{}, err
		}
		r.logger.InfoContext(ctx, "paystack setup: updated", attrs...)
		return setup.ResolvedPlan{Key: spec.Key(), PlanCode: action.ExistingCode, Action: setup.ActionUpdate}, nil
	}

	return setup.ResolvedPlan{}, fmt.Errorf("unknown action kind %q", action.Kind)
}

// rateWait blocks until the internal limiter allows the next call. When no
// limiter is configured (tests), returns immediately.
func (r *Reconciler) rateWait(ctx context.Context) error {
	if r.limiter == nil {
		return nil
	}
	return r.limiter.Wait(ctx)
}

// listAllPlans fetches every plan across paginated responses. Bypasses the
// shared client.do() because list endpoints return meta alongside data,
// which the single-object envelope doesn't model.
func (r *Reconciler) listAllPlans(ctx context.Context) ([]paystackPlan, error) {
	var all []paystackPlan
	const perPage = 100
	page := 1
	for {
		if err := r.rateWait(ctx); err != nil {
			return nil, err
		}
		url := fmt.Sprintf("%s/plan?perPage=%d&page=%d", r.client.baseURL, perPage, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+r.client.secretKey)
		req.Header.Set("Accept", "application/json")

		resp, err := r.client.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list page %d: %w", page, err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read page %d: %w", page, err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("list page %d: http %d: %s", page, resp.StatusCode, truncate(string(body), 200))
		}

		var env struct {
			Status bool           `json:"status"`
			Data   []paystackPlan `json:"data"`
			Meta   struct {
				Total     int `json:"total"`
				Page      int `json:"page"`
				PerPage   int `json:"perPage"`
				PageCount int `json:"pageCount"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, fmt.Errorf("decode page %d: %w", page, err)
		}
		all = append(all, env.Data...)
		if env.Meta.PageCount <= page {
			break
		}
		page++
	}
	return all, nil
}

// createPlan POSTs /plan and returns the new plan_code.
func (r *Reconciler) createPlan(ctx context.Context, spec setup.PlanSpec) (string, error) {
	interval, err := cycleToInterval(spec.Cycle)
	if err != nil {
		return "", err
	}
	if err := r.rateWait(ctx); err != nil {
		return "", err
	}
	body := map[string]any{
		"name":        spec.Name,
		"amount":      spec.AmountMinor,
		"interval":    interval,
		"currency":    spec.Currency,
		"description": spec.Description,
	}
	var resp paystackPlan
	if err := r.client.do(ctx, http.MethodPost, "/plan", body, &resp); err != nil {
		return "", err
	}
	if resp.PlanCode == "" {
		return "", errors.New("paystack create plan: empty plan_code in response")
	}
	return resp.PlanCode, nil
}

// updatePlan PUTs /plan/:code with only the fields we manage. Always sets
// update_existing_subscriptions=false so repricing never silently bleeds
// into active subscribers — that's an explicit separate action.
func (r *Reconciler) updatePlan(ctx context.Context, planCode string, spec setup.PlanSpec) error {
	interval, err := cycleToInterval(spec.Cycle)
	if err != nil {
		return err
	}
	if err := r.rateWait(ctx); err != nil {
		return err
	}
	body := map[string]any{
		"name":                          spec.Name,
		"amount":                        spec.AmountMinor,
		"interval":                      interval,
		"currency":                      spec.Currency,
		"description":                   spec.Description,
		"update_existing_subscriptions": false,
	}
	return r.client.do(ctx, http.MethodPut, "/plan/"+planCode, body, nil)
}

// Compile-time check the adapter satisfies the reconciler interface.
var _ setup.PlanReconciler = (*Reconciler)(nil)
