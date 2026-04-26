package testhelpers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// testRedisAddr matches the docker-compose Redis used by `make
// test-services-up`. Override via REDIS_ADDR for CI runners that bind
// the service on a non-default port.
const testRedisAddr = "localhost:6379"

// testRedisDB is the dedicated DB index for RAG scheduler / handler
// tests. Asynq uses DB 0 by default in production; we steer test
// fixtures into DB 11 so a developer running `redis-cli` can keep
// using DB 0 alongside the test suite.
const testRedisDB = 11

// RedisAddr returns the address the test suite uses to talk to Redis.
// Honors $REDIS_ADDR.
func RedisAddr() string {
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		return v
	}
	return testRedisAddr
}

// RedisDB returns the DB index the RAG tests use.
func RedisDB() int { return testRedisDB }

// ConnectTestRedis opens a real Redis connection on the test DB,
// flushes that DB so tests start from a clean slate, and registers a
// t.Cleanup to flush + close on test end.
//
// A test that needs Redis but the service isn't running should see a
// hard, loud failure — see TESTING.md hard rule #7.
func ConnectTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	cli := redis.NewClient(&redis.Options{
		Addr: RedisAddr(),
		DB:   testRedisDB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		t.Fatalf("Redis not reachable at %s/%d (run `make test-services-up`): %v",
			RedisAddr(), testRedisDB, err)
	}
	if err := cli.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush test redis DB %d: %v", testRedisDB, err)
	}
	t.Cleanup(func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer flushCancel()
		_ = cli.FlushDB(flushCtx).Err()
		_ = cli.Close()
	})
	return cli
}
