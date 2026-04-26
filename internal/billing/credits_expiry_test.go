package billing

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Tests live in package billing (not billing_test) so they can exercise the
// unexported FIFO replay used by SweepOrgExpiredGrants.

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tt
}

func TestReplayBuckets_SingleGrantNoSpend(t *testing.T) {
	gid := uuid.New()
	exp := mustParseTime(t, "2026-04-01T00:00:00Z")
	entries := []model.CreditLedgerEntry{
		{ID: gid, Amount: 1000, Reason: ReasonPlanGrant, ExpiresAt: &exp, CreatedAt: mustParseTime(t, "2026-03-01T00:00:00Z")},
	}
	got := replayBuckets(entries)
	if len(got) != 1 || got[0].remaining != 1000 {
		t.Fatalf("want one bucket remaining=1000, got %+v", got)
	}
}

func TestReplayBuckets_PartialSpend(t *testing.T) {
	gid := uuid.New()
	exp := mustParseTime(t, "2026-04-01T00:00:00Z")
	entries := []model.CreditLedgerEntry{
		{ID: gid, Amount: 1000, Reason: ReasonPlanGrant, ExpiresAt: &exp, CreatedAt: mustParseTime(t, "2026-03-01T00:00:00Z")},
		{ID: uuid.New(), Amount: -300, Reason: ReasonLLMTokens, CreatedAt: mustParseTime(t, "2026-03-05T00:00:00Z")},
	}
	got := replayBuckets(entries)
	if len(got) != 1 || got[0].remaining != 700 {
		t.Fatalf("want remaining=700, got %d", got[0].remaining)
	}
}

func TestReplayBuckets_FullySpent(t *testing.T) {
	gid := uuid.New()
	exp := mustParseTime(t, "2026-04-01T00:00:00Z")
	entries := []model.CreditLedgerEntry{
		{ID: gid, Amount: 1000, Reason: ReasonPlanGrant, ExpiresAt: &exp, CreatedAt: mustParseTime(t, "2026-03-01T00:00:00Z")},
		{ID: uuid.New(), Amount: -1000, Reason: ReasonLLMTokens, CreatedAt: mustParseTime(t, "2026-03-15T00:00:00Z")},
	}
	got := replayBuckets(entries)
	if got[0].remaining != 0 {
		t.Fatalf("want remaining=0, got %d", got[0].remaining)
	}
}

func TestReplayBuckets_ExpirableSpentBeforePermanent(t *testing.T) {
	planID, topupID := uuid.New(), uuid.New()
	exp := mustParseTime(t, "2026-04-01T00:00:00Z")
	entries := []model.CreditLedgerEntry{
		{ID: topupID, Amount: 500, Reason: ReasonTopup, CreatedAt: mustParseTime(t, "2026-02-15T00:00:00Z")},                       // permanent, older
		{ID: planID, Amount: 1000, Reason: ReasonPlanGrant, ExpiresAt: &exp, CreatedAt: mustParseTime(t, "2026-03-01T00:00:00Z")}, // expirable, newer
		{ID: uuid.New(), Amount: -200, Reason: ReasonLLMTokens, CreatedAt: mustParseTime(t, "2026-03-10T00:00:00Z")},
	}
	got := replayBuckets(entries)
	byID := map[uuid.UUID]int64{}
	for _, b := range got {
		byID[b.entryID] = b.remaining
	}
	if byID[topupID] != 500 {
		t.Errorf("permanent should be untouched: got %d, want 500", byID[topupID])
	}
	if byID[planID] != 800 {
		t.Errorf("expirable should absorb the spend: got %d, want 800", byID[planID])
	}
}

func TestReplayBuckets_TwoExpiringGrants_EarliestFirst(t *testing.T) {
	earlyID, lateID := uuid.New(), uuid.New()
	earlyExp := mustParseTime(t, "2026-04-01T00:00:00Z")
	lateExp := mustParseTime(t, "2026-05-01T00:00:00Z")
	entries := []model.CreditLedgerEntry{
		// Late-expiring grant arrives first (older), but earlier-expiring grant should be drained first.
		{ID: lateID, Amount: 100, Reason: ReasonPlanGrant, ExpiresAt: &lateExp, CreatedAt: mustParseTime(t, "2026-02-01T00:00:00Z")},
		{ID: earlyID, Amount: 100, Reason: ReasonPlanGrant, ExpiresAt: &earlyExp, CreatedAt: mustParseTime(t, "2026-03-01T00:00:00Z")},
		{ID: uuid.New(), Amount: -150, Reason: ReasonLLMTokens, CreatedAt: mustParseTime(t, "2026-03-15T00:00:00Z")},
	}
	got := replayBuckets(entries)
	byID := map[uuid.UUID]int64{}
	for _, b := range got {
		byID[b.entryID] = b.remaining
	}
	if byID[earlyID] != 0 {
		t.Errorf("earlier expiry should be drained first: got %d, want 0", byID[earlyID])
	}
	if byID[lateID] != 50 {
		t.Errorf("later expiry takes the overflow: got %d, want 50", byID[lateID])
	}
}

func TestReplayBuckets_PriorExpiryEntry(t *testing.T) {
	gid := uuid.New()
	exp := mustParseTime(t, "2026-04-01T00:00:00Z")
	entries := []model.CreditLedgerEntry{
		{ID: gid, Amount: 1000, Reason: ReasonPlanGrant, ExpiresAt: &exp, CreatedAt: mustParseTime(t, "2026-03-01T00:00:00Z")},
		{ID: uuid.New(), Amount: -300, Reason: ReasonLLMTokens, CreatedAt: mustParseTime(t, "2026-03-15T00:00:00Z")},
		// Prior sweep wrote this offset, forfeiting the remaining 700.
		{ID: uuid.New(), Amount: -700, Reason: ReasonExpiry, RefType: RefTypeExpiredGrant, RefID: gid.String(), CreatedAt: mustParseTime(t, "2026-04-04T00:00:00Z")},
	}
	got := replayBuckets(entries)
	if got[0].remaining != 0 {
		t.Fatalf("prior expiry should leave remaining=0, got %d", got[0].remaining)
	}
}

func TestReplayBuckets_OverdrawClampsAtZero(t *testing.T) {
	// Defensive: a malformed expiry entry trying to over-deduct shouldn't push
	// remaining negative, which would otherwise make the sweep emit a positive
	// "expiry" entry.
	gid := uuid.New()
	exp := mustParseTime(t, "2026-04-01T00:00:00Z")
	entries := []model.CreditLedgerEntry{
		{ID: gid, Amount: 100, Reason: ReasonPlanGrant, ExpiresAt: &exp, CreatedAt: mustParseTime(t, "2026-03-01T00:00:00Z")},
		{ID: uuid.New(), Amount: -500, Reason: ReasonExpiry, RefType: RefTypeExpiredGrant, RefID: gid.String(), CreatedAt: mustParseTime(t, "2026-04-04T00:00:00Z")},
	}
	got := replayBuckets(entries)
	if got[0].remaining != 0 {
		t.Fatalf("clamp at zero, got %d", got[0].remaining)
	}
}

func TestDeductSpend_CascadesAcrossBuckets(t *testing.T) {
	exp1 := mustParseTime(t, "2026-04-01T00:00:00Z")
	exp2 := mustParseTime(t, "2026-05-01T00:00:00Z")
	a := &bucket{entryID: uuid.New(), expiresAt: &exp1, remaining: 100, createdAt: mustParseTime(t, "2026-03-01T00:00:00Z")}
	b := &bucket{entryID: uuid.New(), expiresAt: &exp2, remaining: 100, createdAt: mustParseTime(t, "2026-03-02T00:00:00Z")}
	c := &bucket{entryID: uuid.New(), expiresAt: nil, remaining: 100, createdAt: mustParseTime(t, "2026-02-01T00:00:00Z")}
	deductSpend([]*bucket{c, a, b}, 250)
	if a.remaining != 0 || b.remaining != 0 || c.remaining != 50 {
		t.Errorf("cascade: a=%d b=%d c=%d, want 0/0/50", a.remaining, b.remaining, c.remaining)
	}
}
