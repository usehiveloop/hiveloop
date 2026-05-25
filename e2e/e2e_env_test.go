package e2e

import "github.com/usehivy/hivy/internal/testdb"

func testDatabaseURL() string {
	return testdb.DatabaseURL("HIVY_DATABASE_URL", "DATABASE_URL", "TEST_DATABASE_URL")
}

func testRedisAddrOrEnv() string {
	return testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
}
