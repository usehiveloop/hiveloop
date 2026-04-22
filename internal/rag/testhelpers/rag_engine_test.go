package testhelpers_test

// Integration tests for the rag-engine testhelpers themselves.
//
// These 4 tests match the Phase 2 plan's "Tranche 2J — Tests" list.
// Each one pins a behavior that, if it regresses, would leave every
// downstream tranche unable to trust the helper.
//
// Every test speaks to a real process + real MinIO. No mocks.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// TestStartRagEngineInTestMode_Boots verifies the full happy path.
// Business value: every other integration test in the subsystem builds
// on this — a silent regression here stops all downstream testing.
func TestStartRagEngineInTestMode_Boots(t *testing.T) {
	inst := testhelpers.StartRagEngineInTestMode(t, testhelpers.RagEngineConfig{})
	if inst.Addr == "" {
		t.Fatal("no addr")
	}
	if inst.Client == nil {
		t.Fatal("no client")
	}
	// Health roundtrip via ragclient — proves shared-secret auth wires up.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := inst.Client.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if resp == nil {
		t.Fatal("nil health response")
	}
}

// TestStartRagEngineInTestMode_ReusesBinaryAcrossTwoCalls verifies the
// sync.Once-backed build cache: the binary's mtime is identical between
// two helper invocations inside the same `go test` process.
// Business value: CI pays the ~40s Rust build exactly once per test
// binary, not per-test.
func TestStartRagEngineInTestMode_ReusesBinaryAcrossTwoCalls(t *testing.T) {
	// We start ONE real server (to populate the sync.Once-backed
	// cache), then stat the binary and invoke the build helper a
	// second time — which must short-circuit on the cache. This avoids
	// a second process binding another ephemeral port, which has a
	// race under parallel TIME_WAIT.
	inst := testhelpers.StartRagEngineInTestMode(t, testhelpers.RagEngineConfig{})
	statOne, err := testhelpers.StatBinary(inst.BinaryPath)
	if err != nil {
		t.Fatalf("stat after first start: %v", err)
	}

	// Resolve through the same path the helper internally uses — this
	// tests the second-call codepath in BuildRagEngineBinary. The
	// override-or-default logic must resolve to the same path.
	override := os.Getenv("RAG_ENGINE_BRANCH")
	bin2, err := testhelpers.BuildRagEngineBinary(override)
	if err != nil {
		t.Fatalf("second BuildRagEngineBinary: %v", err)
	}
	statTwo, err := testhelpers.StatBinary(bin2)
	if err != nil {
		t.Fatalf("stat after second build call: %v", err)
	}

	if inst.BinaryPath != bin2 {
		t.Fatalf("binary path changed across calls: %s vs %s", inst.BinaryPath, bin2)
	}
	if !statOne.ModTime.Equal(statTwo.ModTime) {
		t.Fatalf("binary mtime changed (build ran again): %s → %s", statOne.ModTime, statTwo.ModTime)
	}
	if statOne.Size != statTwo.Size {
		t.Fatalf("binary size changed across calls: %d → %d", statOne.Size, statTwo.Size)
	}
}

// TestStartRagEngineInTestMode_CleanupRegisteredRemovesPrefix verifies
// the t.Cleanup handler wipes the per-instance S3 prefix.
// Business value: tests MUST NOT leak MinIO state between runs.
func TestStartRagEngineInTestMode_CleanupRegisteredRemovesPrefix(t *testing.T) {
	cfg := testhelpers.DefaultMinIOConfig()
	bucket := testhelpers.DefaultMinIOBucket
	testhelpers.AssertMinIOUp(t, cfg)

	prefix := "tests/cleanup-" + strings.ReplaceAll(t.Name(), "/", "-") + "/"
	ensureCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := testhelpers.EnsureBucket(ensureCtx, cfg, bucket); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}

	// Seed a marker object so the cleanup has something concrete to
	// delete. Even when the rag-engine itself never touches its prefix
	// (e.g. because handlers return Unimplemented in 2J alone), cleanup
	// must still sweep what was there.
	seedObject(t, cfg, bucket, prefix+"seed.txt", "hello")

	t.Run("instance-owned-prefix-is-removed-on-cleanup", func(sub *testing.T) {
		inst := testhelpers.StartRagEngineInTestMode(sub, testhelpers.RagEngineConfig{
			LancePrefix: prefix,
		})
		if inst.Prefix != prefix {
			sub.Fatalf("prefix not threaded through: got %q want %q", inst.Prefix, prefix)
		}
	})

	countCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	n, err := testhelpers.CountS3Prefix(countCtx, cfg, bucket, prefix)
	if err != nil {
		t.Fatalf("count prefix after cleanup: %v", err)
	}
	if n != 0 {
		t.Fatalf("cleanup left %d objects under %s/%s", n, bucket, prefix)
	}
}

// TestStartRagEngineInTestMode_FailsLoudlyWhenMinIODown verifies Hard
// Rule #7 of TESTING.md: if MinIO isn't reachable, the helper produces
// a clear remediation message and fails — no silent skip.
func TestStartRagEngineInTestMode_FailsLoudlyWhenMinIODown(t *testing.T) {
	bogus := testhelpers.MinIOConfig{
		// Port 1 ("tcpmux") is assigned by IANA but never bound by user
		// services; kernel immediately returns ECONNREFUSED.
		Endpoint:  "http://127.0.0.1:1",
		AccessKey: "anyone",
		SecretKey: "anyone",
		Region:    "us-east-1",
	}
	rec := &fatalRecorder{}
	fakeT := &recordingT{T: t, recorder: rec}
	testhelpers.AssertMinIOUp(fakeT, bogus)
	msg := rec.msg
	if msg == "" {
		t.Fatal("expected AssertMinIOUp to fail when MinIO is unreachable")
	}
	const want = "run `make test-services-up` first"
	if !strings.Contains(msg, want) {
		t.Fatalf("missing remediation string in error message.\nwant substring: %q\ngot: %q", want, msg)
	}
}

// -------- helpers --------

// seedObject uploads one small object under key. Uses the public S3
// client constructor so we're exercising the same code path the
// helpers do.
func seedObject(t *testing.T, cfg testhelpers.MinIOConfig, bucket, key, body string) {
	t.Helper()
	client := testhelpers.NewMinIOS3Client(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(body),
	}); err != nil {
		t.Fatalf("seed object %s/%s: %v", bucket, key, err)
	}
}

// fatalRecorder captures the FIRST formatted Fatalf message.
type fatalRecorder struct {
	msg string
}

// recordingT adapts *testing.T so AssertMinIOUp's Fatalf goes into a
// recorder instead of killing the test. Only the two methods
// AssertMinIOUp touches (`Helper` + `Fatalf`) are overridden — every
// other testing.TB call delegates to the embedded *testing.T.
type recordingT struct {
	*testing.T
	recorder *fatalRecorder
}

func (r *recordingT) Fatalf(format string, args ...any) {
	if r.recorder != nil && r.recorder.msg == "" {
		r.recorder.msg = fmt.Sprintf(format, args...)
		return
	}
	r.T.Fatalf(format, args...)
}
