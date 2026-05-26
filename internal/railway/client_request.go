package railway

import (
	"context"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
)

func (c *Client) request(ctx context.Context, opName, query string, variables map[string]any, data any) error {
	resp := graphql.Response{Data: data}
	req := &graphql.Request{
		Query:     query,
		Variables: variables,
		OpName:    opName,
	}
	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		lastErr = c.gql.MakeRequest(ctx, req, &resp)
		if lastErr == nil || !isRetryableGraphQLError(lastErr) || attempt == c.maxAttempts {
			return lastErr
		}
		if err := sleepWithBackoff(ctx, c.initialDelay, c.maxDelay, attempt); err != nil {
			return err
		}
	}
	return lastErr
}

func isRetryableGraphQLError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"429",
		"500",
		"502",
		"503",
		"504",
		"deadline exceeded",
		"connection reset",
		"connection refused",
		"temporary",
		"timeout",
		"too many requests",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func isDuplicateServiceNameError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "service named") && strings.Contains(msg, "already exists")
}

func sleepWithBackoff(ctx context.Context, initial, maxDelay time.Duration, attempt int) error {
	delay := initial
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= maxDelay {
			delay = maxDelay
			break
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
