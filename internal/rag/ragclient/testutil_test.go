package ragclient

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// ragEngineBuildOnce caches the Rust binary build across every test in
// the package. The first test pays the ~40s build; subsequent tests
// reuse the artifact at services/rag-engine/target/release/rag-engine-server.
var (
	ragEngineBuildOnce    sync.Once
	ragEngineBuildErr     error
	ragEngineBinaryPath   string
	ragEngineRepoRootOnce sync.Once
	ragEngineRepoRoot     string
	ragEngineRepoRootErr  error
)

// repoRoot walks up from this file's location until it finds a directory
// that contains go.mod + services/rag-engine. Cached.
func repoRoot() (string, error) {
	ragEngineRepoRootOnce.Do(func() {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			ragEngineRepoRootErr = fmt.Errorf("runtime.Caller failed")
			return
		}
		dir := filepath.Dir(thisFile)
		for i := 0; i < 10; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				if _, err := os.Stat(filepath.Join(dir, "services", "rag-engine", "Cargo.toml")); err == nil {
					ragEngineRepoRoot = dir
					return
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		ragEngineRepoRootErr = fmt.Errorf("could not locate repo root with services/rag-engine")
	})
	return ragEngineRepoRoot, ragEngineRepoRootErr
}

// buildRagEngineBinary compiles `rag-engine-server` once per test run.
// Fails fast with a clear message when cargo is absent — we never
// substitute a mock.
func buildRagEngineBinary() (string, error) {
	ragEngineBuildOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			ragEngineBuildErr = err
			return
		}
		if _, err := exec.LookPath("cargo"); err != nil {
			ragEngineBuildErr = fmt.Errorf(
				"cargo is required to run ragclient tests (builds services/rag-engine). " +
					"Install Rust toolchain (https://rustup.rs) and retry")
			return
		}
		cmd := exec.Command("cargo", "build", "--release", "--bin", "rag-engine-server")
		cmd.Dir = filepath.Join(root, "services", "rag-engine")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			ragEngineBuildErr = fmt.Errorf("cargo build failed: %w", err)
			return
		}
		ragEngineBinaryPath = filepath.Join(root, "services", "rag-engine", "target", "release", "rag-engine-server")
		if _, err := os.Stat(ragEngineBinaryPath); err != nil {
			ragEngineBuildErr = fmt.Errorf("built binary missing at %s: %w", ragEngineBinaryPath, err)
		}
	})
	return ragEngineBinaryPath, ragEngineBuildErr
}

// pickFreePort grabs a free TCP port on loopback and returns its string
// form ("127.0.0.1:PORT"). There is a micro-race between releasing the
// listener and the Rust server binding; in practice this is fine on
// test machines.
func pickFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}
	return addr
}

// startRagEngine launches the Rust binary on a fresh free port with the
// supplied shared secret. It waits (up to 10s) for the gRPC health
// service to report SERVING before returning.
//
// Returned addr is "host:port"; shutdown is a func that SIGTERMs the
// process and waits for exit. Cleanup is also registered on t.Cleanup.
func startRagEngine(t *testing.T, secret string) (addr string, shutdown func()) {
	t.Helper()
	bin, err := buildRagEngineBinary()
	if err != nil {
		t.Fatalf("build rag-engine: %v", err)
	}

	addr = pickFreePort(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"RAG_ENGINE_LISTEN_ADDR="+addr,
		"RAG_ENGINE_SHARED_SECRET="+secret,
		"RAG_ENGINE_LOG_LEVEL=warn",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	// Put the child in its own process group so we can signal it
	// cleanly without racing the go test harness.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start rag-engine: %v", err)
	}

	done := make(chan struct{})
	var once sync.Once
	shutdown = func() {
		once.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
			// Best-effort wait; tests don't care about exit code.
			waited := make(chan error, 1)
			go func() { waited <- cmd.Wait() }()
			select {
			case <-waited:
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
				<-waited
			}
			close(done)
		})
	}
	t.Cleanup(shutdown)

	if err := waitForServing(t, addr, 10*time.Second); err != nil {
		shutdown()
		t.Fatalf("rag-engine never became SERVING on %s: %v", addr, err)
	}
	return addr, shutdown
}

// waitForServing polls the gRPC health endpoint until it reports
// SERVING or the timeout elapses. Health is unauthenticated so no
// bearer token is required.
func waitForServing(t *testing.T, addr string, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for SERVING")
		}
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			hc := grpc_health_v1.NewHealthClient(conn)
			resp, hcErr := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
			cancel()
			_ = conn.Close()
			if hcErr == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}
