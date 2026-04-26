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
	hdr := http.Header{}
	if _, ok := detectRateLimit(http.StatusForbidden, hdr); ok {
		t.Fatal("plain 403 should not be detected as rate-limit")
	}

	hdr.Set("X-RateLimit-Remaining", "0")
	hdr.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	if _, ok := detectRateLimit(http.StatusOK, hdr); ok {
		t.Fatal("200 should not be detected as rate-limit")
	}

	want := time.Now().Add(2 * time.Second).Unix()
	hdr.Set("X-RateLimit-Reset", strconv.FormatInt(want, 10))
	resetAt, ok := detectRateLimit(http.StatusForbidden, hdr)
	if !ok {
		t.Fatal("expected rate-limit detection")
	}
	if resetAt.Unix() != want {
		t.Fatalf("resetAt = %d, want %d", resetAt.Unix(), want)
	}

	if _, ok := detectRateLimit(http.StatusTooManyRequests, hdr); !ok {
		t.Fatal("expected 429 to qualify as rate-limit")
	}
}

func TestRateLimit_RetriesUntilReset(t *testing.T) {
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
	fp.injectRateLimit(1)

	docs, fails := runIngest(t, c, time.Time{}, time.Time{})
	if len(fails) != 0 {
		t.Fatalf("unexpected failures after retry: %v", fails)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc after retry, got %d", len(docs))
	}
}

// Persistent rate-limit must surface a terminal EntityFailure rather
// than spin forever.
func TestRateLimit_AbortsAfterMaxAttempts(t *testing.T) {
	saved := rateLimitMarginForTest
	rateLimitMarginForTest = 1 * time.Millisecond
	t.Cleanup(func() { rateLimitMarginForTest = saved })

	calls := int32(0)
	op := func() error {
		atomic.AddInt32(&calls, 1)
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
