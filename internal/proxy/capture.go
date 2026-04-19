package proxy

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ziraloop/ziraloop/internal/observe"
)

// CaptureTransport wraps an http.RoundTripper to capture response metadata
// (usage, TTFB, status) without adding latency to the response.
type CaptureTransport struct {
	Inner http.RoundTripper
}

// RoundTrip executes the HTTP request and captures response data.
// For streaming (SSE) responses, it wraps the body to parse usage as chunks
// flow through. For non-streaming responses, it reads and re-serves the body.
func (ct *CaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	captured, hasCaptured := observe.CapturedDataFromContext(req.Context())

	start := time.Now()

	resp, err := ct.Inner.RoundTrip(req)
	if err != nil {
		totalMs := int(time.Since(start).Milliseconds())
		if hasCaptured {
			captured.TotalMs = totalMs
			captured.ErrorType = classifyTransportError(err)
			captured.ErrorMessage = err.Error()
		}
		// Log via slog — the wrapped handler mirrors this to PostHog. Upstream
		// failures are operationally important; currently they were only
		// recorded in the captured-data struct and never surfaced to alerts.
		slog.Error("proxy upstream transport error",
			"method", req.Method,
			"host", req.URL.Host,
			"path", req.URL.Path,
			"duration_ms", totalMs,
			"error", err,
		)
		return nil, err
	}

	if !hasCaptured {
		return resp, nil
	}

	captured.UpstreamStatus = resp.StatusCode

	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(contentType, "text/event-stream")
	captured.IsStreaming = isSSE

	if resp.StatusCode >= 400 {
		// Capture error responses but don't parse usage
		captured.TotalMs = int(time.Since(start).Milliseconds())
		captured.TTFBMs = captured.TotalMs
		captured.ErrorType = classifyHTTPError(resp.StatusCode)
		// Read a snippet for error message
		if resp.Body != nil {
			snippet := make([]byte, 512)
			n, _ := resp.Body.Read(snippet)
			if n > 0 {
				captured.ErrorMessage = string(snippet[:n])
				resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(snippet[:n]), resp.Body))
			}
		}
		// Surface 5xx upstream errors via slog (mirrored to PostHog). 4xx are
		// normal user-facing errors and don't warrant a capture — they already
		// show up in the usage/audit tables.
		if resp.StatusCode >= 500 {
			slog.Error("proxy upstream 5xx response",
				"method", req.Method,
				"host", req.URL.Host,
				"path", req.URL.Path,
				"status", resp.StatusCode,
				"duration_ms", captured.TotalMs,
				"snippet", captured.ErrorMessage,
			)
		}
		return resp, nil
	}

	if isSSE {
		resp.Body = &streamingCapture{
			inner:      resp.Body,
			captured:   captured,
			start:      start,
			providerID: captured.ProviderID,
		}
	} else {
		ct.captureNonStreaming(resp, captured, start)
	}

	return resp, nil
}

func (ct *CaptureTransport) captureNonStreaming(resp *http.Response, captured *observe.CapturedData, start time.Time) {
	if resp.Body == nil {
		captured.TotalMs = int(time.Since(start).Milliseconds())
		captured.TTFBMs = captured.TotalMs
		return
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	captured.TotalMs = int(time.Since(start).Milliseconds())
	captured.TTFBMs = captured.TotalMs // for non-streaming, TTFB ≈ total

	if err == nil && len(body) > 0 {
		captured.Usage = toObserveUsage(ParseUsageNonStreaming(captured.ProviderID, body))
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
}

// streamingCapture wraps an SSE response body, parsing usage from chunks
// as they flow through without adding latency.
type streamingCapture struct {
	inner      io.ReadCloser
	captured   *observe.CapturedData
	start      time.Time
	providerID string
	gotFirst   bool
	buf        bytes.Buffer // accumulates partial lines
}

func (sc *streamingCapture) Read(p []byte) (int, error) {
	n, err := sc.inner.Read(p)

	if n > 0 && !sc.gotFirst {
		sc.gotFirst = true
		sc.captured.TTFBMs = int(time.Since(sc.start).Milliseconds())
	}

	if n > 0 {
		sc.buf.Write(p[:n])
		sc.tryParseEvents()
	}

	if err != nil {
		sc.captured.TotalMs = int(time.Since(sc.start).Milliseconds())
	}

	return n, err
}

func (sc *streamingCapture) Close() error {
	sc.captured.TotalMs = int(time.Since(sc.start).Milliseconds())
	sc.tryParseEvents()
	return sc.inner.Close()
}

func (sc *streamingCapture) tryParseEvents() {
	data := sc.buf.Bytes()
	scanner := bufio.NewScanner(bytes.NewReader(data))

	var lastComplete int
	for scanner.Scan() {
		line := scanner.Text()
		lastComplete = lastComplete + len(line) + 1

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		u := ParseStreamingChunk(sc.providerID, []byte(payload))
		if u.InputTokens > 0 || u.OutputTokens > 0 {
			sc.captured.Usage = toObserveUsage(u)
		}
	}

	if lastComplete > 0 && lastComplete <= len(data) {
		remaining := data[lastComplete:]
		sc.buf.Reset()
		sc.buf.Write(remaining)
	}
}

// toObserveUsage converts proxy.UsageData to observe.UsageData.
func toObserveUsage(u UsageData) observe.UsageData {
	return observe.UsageData{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		CachedTokens:    u.CachedTokens,
		ReasoningTokens: u.ReasoningTokens,
	}
}

func classifyTransportError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") {
		return "timeout"
	}
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") {
		return "connection_error"
	}
	return "transport_error"
}

func classifyHTTPError(status int) string {
	switch {
	case status == 429:
		return "rate_limit"
	case status == 401 || status == 403:
		return "auth"
	case status >= 500:
		return "upstream_error"
	default:
		return "client_error"
	}
}
