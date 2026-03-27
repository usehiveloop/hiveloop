// Package observe holds shared observability types used by both the proxy and
// middleware packages, breaking the import cycle between them.
package observe

import (
	"context"
)

// UsageData holds normalized token usage extracted from an LLM provider response.
type UsageData struct {
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	ReasoningTokens int
}

// CapturedData holds all observability data extracted from a proxy round-trip.
type CapturedData struct {
	Usage          UsageData
	Model          string // extracted from request body
	ProviderID     string // from credential
	IsStreaming    bool
	TTFBMs         int    // time to first byte in milliseconds
	TotalMs        int    // total round-trip time in milliseconds
	UpstreamStatus int    // HTTP status code from upstream
	ErrorType      string // timeout, upstream_error, or empty
	ErrorMessage   string
}

type capturedDataKeyType struct{}

var capturedDataKey = capturedDataKeyType{}

// CapturedDataFromContext retrieves captured generation data from the context.
func CapturedDataFromContext(ctx context.Context) (*CapturedData, bool) {
	d, ok := ctx.Value(capturedDataKey).(*CapturedData)
	return d, ok
}

// WithCapturedData sets captured generation data on the context.
func WithCapturedData(ctx context.Context, d *CapturedData) context.Context {
	return context.WithValue(ctx, capturedDataKey, d)
}
