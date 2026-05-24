package e2e

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func testDatabaseURL() string {
	dsn := envOr("HIVY_DATABASE_URL", testDBURL)
	if !strings.Contains(dsn, "@postgres:") {
		return dsn
	}
	for _, port := range []string{
		strings.TrimSpace(readFileString("/tmp/agent-test/pg.port")),
		"15432",
		"5433",
		"5432",
	} {
		if port != "" && canDial("localhost:"+port) {
			return fmt.Sprintf("postgres://hivy:localdev@localhost:%s/hivy_test?sslmode=disable", port)
		}
	}
	return testDBURL
}

func testRedisAddrOrEnv() string {
	addr := envOr("HIVY_REDIS_ADDR", testRedisAddr)
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "redis" {
		return addr
	}
	return testRedisAddr
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
