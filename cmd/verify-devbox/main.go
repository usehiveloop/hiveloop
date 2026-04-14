// Command verify-devbox spins up a sandbox from a dev-box snapshot and runs
// HTTP-based verification against the services that should be listening
// inside it: Bridge on :8080.
//
// Verification uses Daytona preview URLs (sandbox.GetPreviewLink) instead of
// the toolbox proxy, because this Daytona instance returns a toolbox proxy
// hostname (preview.ziraloop.com) that has no public DNS.
//
// Usage:
//
//	go run ./cmd/verify-devbox -snapshot zira-dev-box-medium-v0.17.2
//	go run ./cmd/verify-devbox -snapshot zira-dev-box-medium-v0.17.2 -keep
//	go run ./cmd/verify-devbox -cleanup <sandbox-id>
//
// Requires SANDBOX_PROVIDER_KEY, SANDBOX_PROVIDER_URL, and SANDBOX_TARGET in
// the environment (load .env via the verify-devbox make target).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"
)

func mustEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("error: %s is required in the environment", name)
	}
	return value
}

func newClient(ctx context.Context) (*daytona.Client, error) {
	return daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: mustEnv("SANDBOX_PROVIDER_KEY"),
		APIUrl: mustEnv("SANDBOX_PROVIDER_URL"),
		Target: os.Getenv("SANDBOX_TARGET"),
	})
}

func main() {
	snapshot := flag.String("snapshot", "", "Snapshot name to verify (e.g. zira-dev-box-medium-v0.17.2)")
	keep := flag.Bool("keep", false, "Keep the sandbox after verification (for manual debugging)")
	cleanup := flag.String("cleanup", "", "Delete a sandbox by ID and exit (no verification)")
	flag.Parse()

	if *cleanup != "" {
		if err := runCleanup(*cleanup); err != nil {
			log.Fatalf("cleanup failed: %v", err)
		}
		return
	}

	if *snapshot == "" {
		fmt.Fprintln(os.Stderr, "error: -snapshot is required (or pass -cleanup <sandbox-id>)")
		flag.Usage()
		os.Exit(1)
	}

	if err := runVerify(*snapshot, *keep); err != nil {
		log.Fatalf("verification failed: %v", err)
	}
}

func runCleanup(sandboxID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := newClient(ctx)
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	log.Printf("Looking up sandbox %s...", sandboxID)
	sandbox, err := client.Get(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("getting sandbox: %w", err)
	}
	log.Printf("Found sandbox: id=%s name=%s state=%s", sandbox.ID, sandbox.Name, sandbox.State)

	log.Printf("Deleting sandbox %s...", sandbox.ID)
	if err := sandbox.Delete(ctx); err != nil {
		return fmt.Errorf("deleting sandbox: %w", err)
	}
	log.Printf("Sandbox deleted.")
	return nil
}

func runVerify(snapshot string, keep bool) (retErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	client, err := newClient(ctx)
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	log.Printf("Creating sandbox from snapshot %q...", snapshot)
	sandbox, err := client.Create(ctx, types.SnapshotParams{Snapshot: snapshot})
	if err != nil {
		return fmt.Errorf("creating sandbox: %w", err)
	}
	log.Printf("Sandbox created: id=%s name=%s state=%s", sandbox.ID, sandbox.Name, sandbox.State)

	if !keep {
		defer func() {
			log.Printf("Deleting sandbox %s...", sandbox.ID)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cleanupCancel()
			if err := sandbox.Delete(cleanupCtx); err != nil {
				log.Printf("warning: failed to delete sandbox: %v", err)
			} else {
				log.Printf("Sandbox deleted.")
			}
		}()
	}

	log.Printf("Waiting for sandbox to start...")
	if err := sandbox.WaitForStart(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("waiting for sandbox start: %w", err)
	}
	log.Printf("Sandbox is running.")

	type portCheck struct {
		name     string
		port     int
		path     string
		required bool // false = diagnostic only, doesn't fail the run
	}
	checks := []portCheck{
		{name: "Bridge /health", port: 8080, path: "/health", required: true},
		{name: "sentinel (definitely unbound)", port: 9999, path: "/", required: false},
	}

	failed := 0
	for _, item := range checks {
		log.Printf("\n[%s] requesting preview link for port %d...", item.name, item.port)
		preview, err := sandbox.GetPreviewLink(ctx, item.port)
		if err != nil {
			log.Printf("    ERROR getting preview link: %v", err)
			failed++
			continue
		}
		log.Printf("    URL:   %s", preview.URL)
		if preview.Token != "" {
			log.Printf("    Token: %s…", maskToken(preview.Token))
		}

		fullURL := strings.TrimRight(preview.URL, "/") + item.path

		// Retry with backoff — services may take a few seconds to bind.
		var lastErr error
		var lastStatus int
		var lastBody string
		ok := false
		startedAt := time.Now()
		attempts := 1
		if item.required {
			attempts = 15
		}
		for attempt := 1; attempt <= attempts; attempt++ {
			status, body, err := httpGet(ctx, fullURL, preview.Token)
			lastErr = err
			lastStatus = status
			lastBody = body
			if err == nil && status >= 200 && status < 300 {
				log.Printf("    OK (HTTP %d, attempt %d, %s) -- body: %s",
					status, attempt, time.Since(startedAt).Round(time.Millisecond), truncate(body, 200))
				ok = true
				break
			}
			if attempts > 1 {
				time.Sleep(2 * time.Second)
			}
		}
		if !ok {
			label := "FAIL"
			if !item.required {
				label = "DIAGNOSTIC"
			}
			if lastErr != nil {
				log.Printf("    %s: %v", label, lastErr)
			} else {
				log.Printf("    %s: HTTP %d (after %s) -- body: %q",
					label, lastStatus, time.Since(startedAt).Round(time.Millisecond), truncate(lastBody, 400))
			}
			if item.required {
				failed++
			}
		}
	}

	log.Println()
	if failed > 0 {
		return fmt.Errorf("%d check(s) failed out of %d", failed, len(checks))
	}
	log.Printf("All %d checks passed for snapshot %q.", len(checks), snapshot)
	return nil
}

func httpGet(ctx context.Context, url, token string) (int, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	if token != "" {
		req.Header.Set("x-daytona-preview-token", token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, string(body), nil
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "…" + token[len(token)-4:]
}

func truncate(text string, limit int) string {
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
