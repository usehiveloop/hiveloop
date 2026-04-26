package testhelpers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

const testRedisAddr = "localhost:6379"

// testRedisDB steers test fixtures away from production's default DB 0
// so developers can run `redis-cli` alongside the suite.
const testRedisDB = 11

func RedisAddr() string {
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		return v
	}
	return testRedisAddr
}

func RedisDB() int { return testRedisDB }

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
