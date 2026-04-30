package sentry

import (
	"context"
	"net"
	"strings"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/redis/go-redis/v9"
)

func InstallRedisHook(client *redis.Client) {
	if !Enabled() || client == nil {
		return
	}
	client.AddHook(redisHook{})
}

type redisHook struct{}

func (redisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (redisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if sentrygo.TransactionFromContext(ctx) == nil {
			return next(ctx, cmd)
		}
		span := sentrygo.StartSpan(ctx, "db.redis")
		span.Description = cmd.Name()
		span.SetData("db.system", "redis")
		span.SetData("db.statement", redactedRedisArgs(cmd))
		err := next(ctx, cmd)
		if err != nil && err != redis.Nil {
			span.Status = sentrygo.SpanStatusInternalError
			span.SetData("error", err.Error())
		} else {
			span.Status = sentrygo.SpanStatusOK
		}
		span.Finish()
		return err
	}
}

func (redisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if sentrygo.TransactionFromContext(ctx) == nil {
			return next(ctx, cmds)
		}
		span := sentrygo.StartSpan(ctx, "db.redis.pipeline")
		span.SetData("db.system", "redis")
		span.SetData("db.redis.commands", len(cmds))
		err := next(ctx, cmds)
		if err != nil && err != redis.Nil {
			span.Status = sentrygo.SpanStatusInternalError
			span.SetData("error", err.Error())
		} else {
			span.Status = sentrygo.SpanStatusOK
		}
		span.Finish()
		return err
	}
}

// redactedRedisArgs returns command + key only — the value would leak cached
// credentials and session blobs into Sentry.
func redactedRedisArgs(cmd redis.Cmder) string {
	args := cmd.Args()
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	parts = append(parts, toString(args[0]))
	if len(args) > 1 {
		parts = append(parts, toString(args[1]))
	}
	return strings.Join(parts, " ")
}

func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}
