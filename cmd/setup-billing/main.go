// Command setup-billing idempotently reconciles Hiveloop's canonical plans
// with a payment provider.
//
// Usage:
//
//	go run ./cmd/setup-billing --provider=paystack --secret-key=$PAYSTACK_SECRET
//	go run ./cmd/setup-billing --provider=paystack --dry-run
//
// Output:
//
//	stdout — JSON array of ResolvedPlan for programmatic consumption
//	stderr — env-var block suitable for copy-paste into .env
//
// Exit codes:
//
//	0  clean (or dry-run that would have succeeded)
//	1  reconciliation error (network, API, etc.)
//	2  CLI / config error
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/usehiveloop/hiveloop/internal/billing/paystack"
	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

func main() {
	var (
		provider   = flag.String("provider", "paystack", "billing provider: paystack")
		secretKey  = flag.String("secret-key", "", "provider secret key (or set PAYSTACK_SECRET_KEY)")
		dryRun     = flag.Bool("dry-run", false, "print intended actions without mutating the provider")
		currencies = flag.String("currencies", "NGN", "comma-separated ISO-4217 codes to reconcile (empty = all)")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *secretKey == "" {
		*secretKey = os.Getenv("PAYSTACK_SECRET_KEY")
	}
	if *secretKey == "" {
		fmt.Fprintln(os.Stderr, "error: --secret-key or PAYSTACK_SECRET_KEY required")
		os.Exit(2)
	}

	specs := setup.FilterByCurrency(splitCurrencies(*currencies)...)
	if len(specs) == 0 {
		fmt.Fprintf(os.Stderr, "error: no canonical plans matched currencies %q\n", *currencies)
		os.Exit(2)
	}

	rec, err := buildReconciler(*provider, *secretKey, *dryRun, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	logger.InfoContext(ctx, "setup-billing: starting",
		"provider", *provider, "dry_run", *dryRun, "spec_count", len(specs), "currencies", *currencies)

	resolved, err := rec.Reconcile(ctx, specs)
	if err != nil {
		logger.ErrorContext(ctx, "setup-billing: reconcile failed", "error", err)
		os.Exit(1)
	}

	// Machine-readable JSON on stdout (for pipelines), human env-var block on
	// stderr (for eyeballs + copy-paste).
	if err := writeJSON(os.Stdout, resolved); err != nil {
		logger.ErrorContext(ctx, "setup-billing: failed to write JSON output", "error", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "# Copy the following into your .env or config. Safe to re-run at any time:")
	fmt.Fprint(os.Stderr, formatEnvVars(*provider, resolved))
}

// buildReconciler selects the provider adapter. Extending to Polar later
// means adding a case here and importing internal/billing/polar.
func buildReconciler(provider, secretKey string, dryRun bool, logger *slog.Logger) (setup.PlanReconciler, error) {
	switch provider {
	case "paystack":
		opts := []paystack.ReconcilerOption{paystack.WithLogger(logger)}
		if dryRun {
			opts = append(opts, paystack.WithDryRun())
		}
		return paystack.NewReconciler(secretKey, opts...), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q (supported: paystack)", provider)
	}
}

func splitCurrencies(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(strings.ToUpper(p)); s != "" {
			out = append(out, s)
		}
	}
	return out
}
