package paystack

// Config wires the Paystack adapter with credentials. We manage the
// recurring lifecycle ourselves (period tracking, upgrades, downgrades,
// cancellation, proration) so the adapter only needs the secret key it
// uses for /transaction/initialize and /transaction/charge_authorization.
type Config struct {
	// SecretKey is the Paystack secret key ("sk_live_…" or "sk_test_…").
	SecretKey string
}
