package setup_test

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

func spec(name, cur string, cy billing.Cycle, amt int64, desc string) setup.PlanSpec {
	return setup.PlanSpec{
		Slug:        "pro",
		Name:        name,
		AmountMinor: amt,
		Currency:    cur,
		Cycle:       cy,
		Description: desc,
	}
}

func existing(code, name, cur string, cy billing.Cycle, amt int64, desc string) setup.ExistingPlan {
	return setup.ExistingPlan{
		PlanCode:    code,
		Name:        name,
		Currency:    cur,
		Cycle:       cy,
		AmountMinor: amt,
		Description: desc,
	}
}

func TestDecide_EmptyUpstream_AllCreate(t *testing.T) {
	desired := []setup.PlanSpec{
		spec("Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
		spec("Pro (Annual)", "NGN", billing.CycleAnnual, 561_000_00, "Pro"),
	}
	got := setup.Decide(nil, desired)
	if len(got) != 2 {
		t.Fatalf("got %d actions, want 2", len(got))
	}
	for i, a := range got {
		if a.Kind != setup.ActionCreate {
			t.Errorf("action[%d].Kind = %s, want create", i, a.Kind)
		}
		if a.ExistingCode != "" {
			t.Errorf("action[%d].ExistingCode = %q, want empty for Create", i, a.ExistingCode)
		}
	}
}

func TestDecide_ExactMatch_NoOp(t *testing.T) {
	ex := []setup.ExistingPlan{
		existing("PLN_pro_m", "Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	desired := []setup.PlanSpec{
		spec("Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	got := setup.Decide(ex, desired)
	if len(got) != 1 || got[0].Kind != setup.ActionNoOp {
		t.Fatalf("got %+v, want single NoOp", got)
	}
	if got[0].ExistingCode != "PLN_pro_m" {
		t.Errorf("ExistingCode = %q, want PLN_pro_m", got[0].ExistingCode)
	}
}

func TestDecide_AmountDrift_Update(t *testing.T) {
	ex := []setup.ExistingPlan{
		existing("PLN_pro_m", "Pro (Monthly)", "NGN", billing.CycleMonthly, 50_000_00, "Pro"),
	}
	desired := []setup.PlanSpec{
		spec("Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	got := setup.Decide(ex, desired)
	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1", len(got))
	}
	if got[0].Kind != setup.ActionUpdate {
		t.Errorf("Kind = %s, want update", got[0].Kind)
	}
	if got[0].ExistingCode != "PLN_pro_m" {
		t.Errorf("ExistingCode = %q", got[0].ExistingCode)
	}
	if len(got[0].DriftFields) != 1 || got[0].DriftFields[0] != "amount" {
		t.Errorf("DriftFields = %v, want [amount]", got[0].DriftFields)
	}
}

func TestDecide_DescriptionDrift_Update(t *testing.T) {
	ex := []setup.ExistingPlan{
		existing("PLN_pro_m", "Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Old description"),
	}
	desired := []setup.PlanSpec{
		spec("Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "New description"),
	}
	got := setup.Decide(ex, desired)
	if got[0].Kind != setup.ActionUpdate {
		t.Fatalf("Kind = %s, want update", got[0].Kind)
	}
	if got[0].DriftFields[0] != "description" {
		t.Errorf("DriftFields = %v, want [description]", got[0].DriftFields)
	}
}

func TestDecide_MultipleDriftFields_SingleUpdate(t *testing.T) {
	ex := []setup.ExistingPlan{
		existing("PLN_pro_m", "Pro (Monthly)", "NGN", billing.CycleMonthly, 50_000_00, "Old"),
	}
	desired := []setup.PlanSpec{
		spec("Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "New"),
	}
	got := setup.Decide(ex, desired)
	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1", len(got))
	}
	if len(got[0].DriftFields) != 2 {
		t.Errorf("DriftFields = %v, want [amount description]", got[0].DriftFields)
	}
}

func TestDecide_SameNameDifferentCurrency_BothCreate(t *testing.T) {
	// Critical: "Pro (Monthly)" in NGN and "Pro (Monthly)" in USD are
	// separate plans. The match key includes currency, so an existing
	// NGN plan must not cause the USD spec to become a NoOp.
	ex := []setup.ExistingPlan{
		existing("PLN_pro_ngn_m", "Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	desired := []setup.PlanSpec{
		spec("Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
		spec("Pro (Monthly)", "USD", billing.CycleMonthly, 39_00, "Pro"),
	}
	got := setup.Decide(ex, desired)
	if got[0].Kind != setup.ActionNoOp {
		t.Errorf("NGN spec got %s, want noop", got[0].Kind)
	}
	if got[1].Kind != setup.ActionCreate {
		t.Errorf("USD spec got %s, want create (must not match the NGN row)", got[1].Kind)
	}
}

func TestDecide_SameNameDifferentCycle_BothCreate(t *testing.T) {
	ex := []setup.ExistingPlan{
		existing("PLN_pro_m", "Pro", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	desired := []setup.PlanSpec{
		spec("Pro", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
		spec("Pro", "NGN", billing.CycleAnnual, 561_000_00, "Pro"),
	}
	got := setup.Decide(ex, desired)
	if got[0].Kind != setup.ActionNoOp {
		t.Errorf("monthly got %s, want noop", got[0].Kind)
	}
	if got[1].Kind != setup.ActionCreate {
		t.Errorf("annual got %s, want create", got[1].Kind)
	}
}

func TestDecide_UpstreamHasStrangerPlan_IgnoredNotDeleted(t *testing.T) {
	// An existing plan we don't want any more (e.g. discontinued "Legacy
	// Pro") must NOT generate a delete action. Decide never emits deletes.
	ex := []setup.ExistingPlan{
		existing("PLN_legacy", "Legacy Pro", "NGN", billing.CycleMonthly, 10_000_00, "Legacy"),
		existing("PLN_pro_m", "Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	desired := []setup.PlanSpec{
		spec("Pro (Monthly)", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	got := setup.Decide(ex, desired)
	if len(got) != 1 {
		t.Fatalf("got %d actions, want 1 (stranger plan ignored)", len(got))
	}
	if got[0].Kind != setup.ActionNoOp {
		t.Errorf("Kind = %s, want noop", got[0].Kind)
	}
}

func TestDecide_DuplicateUpstreamEntries_LastWins(t *testing.T) {
	// Paystack doesn't enforce uniqueness so the same (name, currency,
	// cycle) could have multiple plan_codes from earlier broken runs.
	// Decide picks the last one seen — not ideal but deterministic, and
	// operators can reconcile manually by archiving older duplicates.
	ex := []setup.ExistingPlan{
		existing("PLN_dup_first", "Pro", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
		existing("PLN_dup_second", "Pro", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	desired := []setup.PlanSpec{
		spec("Pro", "NGN", billing.CycleMonthly, 58_500_00, "Pro"),
	}
	got := setup.Decide(ex, desired)
	if got[0].ExistingCode != "PLN_dup_second" {
		t.Errorf("ExistingCode = %q, want last-seen PLN_dup_second", got[0].ExistingCode)
	}
}

func TestDecide_ActionsOrderedBySpecs(t *testing.T) {
	// Downstream logs/outputs depend on actions being in spec order.
	desired := []setup.PlanSpec{
		spec("First", "NGN", billing.CycleMonthly, 100, ""),
		spec("Second", "NGN", billing.CycleMonthly, 200, ""),
		spec("Third", "NGN", billing.CycleMonthly, 300, ""),
	}
	got := setup.Decide(nil, desired)
	wantNames := []string{"First", "Second", "Third"}
	for i, a := range got {
		if a.Spec.Name != wantNames[i] {
			t.Errorf("action[%d].Name = %q, want %q", i, a.Spec.Name, wantNames[i])
		}
	}
}
