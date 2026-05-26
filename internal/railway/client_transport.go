package railway

import (
	"context"
	"net/http"
	"sync"
	"time"
)

func railwayHTTPClient(template *http.Client, token string, base http.RoundTripper, minInterval time.Duration) *http.Client {
	limited := rateLimitedTransport{
		base: authTransport{
			base:  base,
			token: token,
		},
		limiter: &requestLimiter{minInterval: minInterval},
	}
	next := *template
	next.Transport = limited
	return &next
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := req.Clone(req.Context())
	next.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(next)
}

type requestLimiter struct {
	mu          sync.Mutex
	next        time.Time
	minInterval time.Duration
}

func (l *requestLimiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if now.Before(l.next) {
		wait = l.next.Sub(now)
		l.next = l.next.Add(l.minInterval)
	} else {
		l.next = now.Add(l.minInterval)
	}
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type rateLimitedTransport struct {
	base    http.RoundTripper
	limiter *requestLimiter
}

func (t rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.limiter != nil {
		if err := t.limiter.wait(req.Context()); err != nil {
			return nil, err
		}
	}
	return t.base.RoundTrip(req)
}
