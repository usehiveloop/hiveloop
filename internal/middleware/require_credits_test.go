package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/middleware"
)

// fakeBalance is an in-memory BalanceChecker for middleware tests.
type fakeBalance struct {
	values map[uuid.UUID]int64
	err    error
	calls  int
}

func (f *fakeBalance) Balance(orgID uuid.UUID) (int64, error) {
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	return f.values[orgID], nil
}

// okHandler returns 200 so tests can distinguish pass-through from 402/401/500.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func runWithClaims(t *testing.T, claims *middleware.TokenClaims, checker middleware.BalanceChecker) *httptest.ResponseRecorder {
	t.Helper()
	h := middleware.RequireCredits(checker)(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/v1/proxy/chat/completions", nil)
	if claims != nil {
		r = middleware.WithClaims(r, claims)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRequireCredits_MissingClaimsReturns401(t *testing.T) {
	w := runWithClaims(t, nil, &fakeBalance{})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", w.Code)
	}
}

func TestRequireCredits_BYOKAlwaysPasses(t *testing.T) {
	orgID := uuid.New()
	checker := &fakeBalance{values: map[uuid.UUID]int64{orgID: 0}} // zero balance
	claims := &middleware.TokenClaims{
		OrgID:    orgID.String(),
		IsSystem: false, // BYOK — should skip the check entirely
	}
	w := runWithClaims(t, claims, checker)
	if w.Code != http.StatusOK {
		t.Fatalf("BYOK call rejected: status %d, body=%s", w.Code, w.Body.String())
	}
	if checker.calls != 0 {
		t.Errorf("BalanceChecker called %d times for BYOK; want 0 (skip entirely)", checker.calls)
	}
}

func TestRequireCredits_SystemWithBalancePasses(t *testing.T) {
	orgID := uuid.New()
	checker := &fakeBalance{values: map[uuid.UUID]int64{orgID: 1234}}
	claims := &middleware.TokenClaims{OrgID: orgID.String(), IsSystem: true}

	w := runWithClaims(t, claims, checker)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	if checker.calls != 1 {
		t.Errorf("BalanceChecker calls = %d, want 1", checker.calls)
	}
}

func TestRequireCredits_SystemWithZeroBalanceReturns402(t *testing.T) {
	orgID := uuid.New()
	checker := &fakeBalance{values: map[uuid.UUID]int64{orgID: 0}}
	claims := &middleware.TokenClaims{OrgID: orgID.String(), IsSystem: true}

	w := runWithClaims(t, claims, checker)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("zero balance should return 402, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestRequireCredits_SystemWithNegativeBalanceReturns402(t *testing.T) {
	// Happens when an in-flight post-deduct drove the balance below zero.
	// Next call must fail closed.
	orgID := uuid.New()
	checker := &fakeBalance{values: map[uuid.UUID]int64{orgID: -17}}
	claims := &middleware.TokenClaims{OrgID: orgID.String(), IsSystem: true}

	w := runWithClaims(t, claims, checker)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("negative balance should return 402, got %d", w.Code)
	}
}

func TestRequireCredits_InvalidOrgIDReturns401(t *testing.T) {
	checker := &fakeBalance{}
	claims := &middleware.TokenClaims{OrgID: "not-a-uuid", IsSystem: true}

	w := runWithClaims(t, claims, checker)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid org id should return 401, got %d", w.Code)
	}
}

func TestRequireCredits_BalanceLookupErrorReturns500(t *testing.T) {
	// DB outage must fail closed — otherwise platform-keys users would get
	// free inference whenever Postgres hiccups.
	orgID := uuid.New()
	checker := &fakeBalance{err: errors.New("db down")}
	claims := &middleware.TokenClaims{OrgID: orgID.String(), IsSystem: true}

	w := runWithClaims(t, claims, checker)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("balance lookup error should fail closed with 500, got %d", w.Code)
	}
}
