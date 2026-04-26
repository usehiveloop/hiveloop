package github

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestDetectRateLimit_HeaderShape(t *testing.T) {
	// Plain 403 (e.g. perms) is NOT a rate-limit event.
	hdr := http.Header{}
	if _, ok := detectRateLimit(http.StatusForbidden, hdr); ok {
		t.Fatal("plain 403 should not be detected as rate-limit")
	}

	// 200 with quota headers is also NOT a rate-limit event.
	hdr.Set("X-RateLimit-Remaining", "0")
	hdr.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	if _, ok := detectRateLimit(http.StatusOK, hdr); ok {
		t.Fatal("200 should not be detected as rate-limit")
	}

	// 403 + Remaining=0 + Reset header -> rate-limited, resetAt parsed.
	want := time.Now().Add(2 * time.Second).Unix()
	hdr.Set("X-RateLimit-Reset", strconv.FormatInt(want, 10))
	resetAt, ok := detectRateLimit(http.StatusForbidden, hdr)
	if !ok {
		t.Fatal("expected rate-limit detection")
	}
	if resetAt.Unix() != want {
		t.Fatalf("resetAt = %d, want %d", resetAt.Unix(), want)
	}

	// 429 path also qualifies (secondary rate limits).
	if _, ok := detectRateLimit(http.StatusTooManyRequests, hdr); !ok {
		t.Fatal("expected 429 to qualify as rate-limit")
	}
}

// TestRateLimit_RetriesUntilReset is the canonical recipe: fixture
// returns 403 + reset 1s ahead, withRateLimitRetry waits past the reset
// (with rateLimitMargin), retries, succeeds.
//
// We override rateLimitMargin in this test by computing a near-zero
// reset offset; the real margin (60s) would make the test painfully
// slow.
func TestRateLimit_RetriesUntilReset(t *testing.T) {
	// Save + restore the package-level margin to keep the test fast.
	saved := rateLimitMarginForTest
	rateLimitMarginForTest = 50 * time.Millisecond
	t.Cleanup(func() { rateLimitMarginForTest = saved })

	calls := int32(0)
	resetAt := time.Now().Add(100 * time.Millisecond)

	op := func() error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return &rateLimitError{resetAt: resetAt}
		}
		return nil
	}
	start := time.Now()
	err := withRateLimitRetryWithMargin(context.Background(), op, rateLimitMarginForTest)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (1 fail + 1 retry), got %d", calls)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("expected to sleep until reset (~100ms), elapsed=%v", elapsed)
	}
}

// TestRateLimit_ConnectorRetriesPage exercises the rate-limit retry
// path through a real connector run: the fixture proxy returns 403 +
// X-RateLimit-Reset on the first call; the retry loop must sleep past
// the reset and succeed on the second call. Different from
// TestRateLimit_RetriesUntilReset (which exercises the helper directly)
// by going through fetch_prs + the run goroutine.
func TestRateLimit_ConnectorRetriesPage(t *testing.T) {
	saved := rateLimitMarginForTest
	rateLimitMarginForTest = 25 * time.Millisecond
	t.Cleanup(func() { rateLimitMarginForTest = saved })

	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	prs := []GithubPR{makePR(1, "open", now)}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, prs), 0)
	// /repos/{owner}/{repo} is also subject to rate limits; the retry
	// helper handles it the same way. Arming injectRateLimit(1) makes
	// the very first call (the repo lookup) retry — the realistic shape.
	fp.injectRateLimit(1)

	docs, fails := runIngest(t, c, time.Time{}, time.Time{})
	if len(fails) != 0 {
		t.Fatalf("unexpected failures after retry: %v", fails)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc after retry, got %d", len(docs))
	}
}

// TestRateLimit_AbortsAfterMaxAttempts pins the safety bound: persistent
// rate-limit returns errRateLimited (joined with the last
// rateLimitError) so the caller surfaces a terminal EntityFailure rather
// than spinning forever.
func TestRateLimit_AbortsAfterMaxAttempts(t *testing.T) {
	saved := rateLimitMarginForTest
	rateLimitMarginForTest = 1 * time.Millisecond
	t.Cleanup(func() { rateLimitMarginForTest = saved })

	calls := int32(0)
	op := func() error {
		atomic.AddInt32(&calls, 1)
		// Reset already in the past — no real sleep.
		return &rateLimitError{resetAt: time.Now().Add(-time.Second)}
	}
	err := withRateLimitRetryWithMargin(context.Background(), op, rateLimitMarginForTest)
	if err == nil {
		t.Fatal("expected exhaustion error, got nil")
	}
	if !errors.Is(err, errRateLimited) {
		t.Fatalf("expected errors.Is(err, errRateLimited); got %v", err)
	}
	if calls != int32(rateLimitMaxAttempts) {
		t.Fatalf("expected %d attempts, got %d", rateLimitMaxAttempts, calls)
	}
}
