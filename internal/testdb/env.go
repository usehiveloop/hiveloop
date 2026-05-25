package testdb

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	DefaultDatabaseURL      = "postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable"
	DefaultNangoDatabaseURL = "postgres://hivy:localdev@localhost:5433/nango?sslmode=disable"
	DefaultRedisAddr        = "localhost:16279"
)

func DatabaseURL(keys ...string) string {
	if len(keys) == 0 {
		keys = []string{"DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL"}
	}
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return hostReachableDatabaseURL(value, "hivy_test")
		}
	}
	return DefaultDatabaseURL
}

func NangoDatabaseURL(keys ...string) string {
	if len(keys) == 0 {
		keys = []string{"HIVY_NANGO_DATABASE_URL", "NANGO_DATABASE_URL"}
	}
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return hostReachableDatabaseURL(value, "nango")
		}
	}
	return DefaultNangoDatabaseURL
}

func RedisAddr(keys ...string) string {
	if len(keys) == 0 {
		keys = []string{"HIVY_REDIS_ADDR", "TEST_REDIS_ADDR"}
	}
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return hostReachableRedisAddr(value)
		}
	}
	return DefaultRedisAddr
}

func hostReachableDatabaseURL(dsn string, databaseName string) string {
	if !strings.Contains(dsn, "@postgres:") {
		return dsn
	}
	for _, port := range candidatePostgresPorts() {
		if canDial("localhost:" + port) {
			return fmt.Sprintf("postgres://hivy:localdev@localhost:%s/%s?sslmode=disable", port, databaseName)
		}
	}
	return defaultDatabaseURL(databaseName)
}

func hostReachableRedisAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "redis" {
		return addr
	}
	if canDial(DefaultRedisAddr) {
		return DefaultRedisAddr
	}
	return addr
}

func candidatePostgresPorts() []string {
	return []string{
		strings.TrimSpace(readFileString("/tmp/agent-test/pg.port")),
		strings.TrimSpace(os.Getenv("HIVY_COMPOSE_POSTGRES_PORT")),
		"5433",
		"15432",
		"5432",
	}
}

func defaultDatabaseURL(databaseName string) string {
	if databaseName == "nango" {
		return DefaultNangoDatabaseURL
	}
	return DefaultDatabaseURL
}

func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func canDial(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
