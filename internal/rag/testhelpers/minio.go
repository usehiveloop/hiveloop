package testhelpers

// MinIO bucket lifecycle for RAG integration tests.
//
// Docker Compose provisions a MinIO instance on 127.0.0.1:9000 with
// credentials minioadmin/minioadmin. `make test-services-up` also
// pre-creates `hiveloop-rag-test`. This file exists so tests that use a
// custom bucket (or want to cleanly isolate state per-test via an S3
// prefix) can create it on the fly, and so a loud clean failure is
// produced when MinIO isn't up — per Hard Rule #7 of TESTING.md.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// MinIO defaults used by docker-compose. Overridable per-test via
// RagEngineConfig / MinIOConfig.
const (
	DefaultMinIOEndpoint  = "http://127.0.0.1:9000"
	DefaultMinIOAccessKey = "minioadmin"
	DefaultMinIOSecretKey = "minioadmin"
	DefaultMinIORegion    = "us-east-1"
	DefaultMinIOBucket    = "hiveloop-rag-test"
)

// MinIOConfig carries everything an S3-compatible client needs to talk
// to the docker-compose MinIO instance.
type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Region    string
}

// DefaultMinIOConfig returns a config pointing at the docker-compose
// MinIO container.
func DefaultMinIOConfig() MinIOConfig {
	return MinIOConfig{
		Endpoint:  DefaultMinIOEndpoint,
		AccessKey: DefaultMinIOAccessKey,
		SecretKey: DefaultMinIOSecretKey,
		Region:    DefaultMinIORegion,
	}
}

// AssertMinIOUp probes the MinIO readiness endpoint. On failure it
// calls Fatalf on the supplied TB with a message that names the
// remediation command — the exact phrasing
// `run `+"`make test-services-up`"+` first` is asserted by
// `TestStartRagEngineInTestMode_FailsLoudlyWhenMinIODown`.
//
// We take `testing.TB` (not `*testing.T`) so both integration tests
// and a recording wrapper used by the MinIO-down test can call us.
func AssertMinIOUp(t testing.TB, cfg MinIOConfig) {
	t.Helper()
	if err := probeMinIO(cfg.Endpoint, 2*time.Second); err != nil {
		t.Fatalf("MinIO not reachable at %s (run `make test-services-up` first): %v", cfg.Endpoint, err)
	}
}

func probeMinIO(endpoint string, timeout time.Duration) error {
	if endpoint == "" {
		return errors.New("empty endpoint")
	}
	url := strings.TrimRight(endpoint, "/") + "/minio/health/ready"
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("health probe returned HTTP %d", resp.StatusCode)
}

// NewMinIOS3Client builds an AWS SDK v2 S3 client configured for the
// supplied MinIO endpoint (path-style addressing, custom endpoint
// resolver, static creds).
func NewMinIOS3Client(cfg MinIOConfig) *s3.Client {
	return s3.NewFromConfig(aws.Config{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})
}

// EnsureBucket creates `bucket` on the MinIO endpoint if it doesn't
// already exist. Safe to call repeatedly.
func EnsureBucket(ctx context.Context, cfg MinIOConfig, bucket string) error {
	if bucket == "" {
		return errors.New("bucket name required")
	}
	client := NewMinIOS3Client(cfg)
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return nil
	}
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return nil
	}
	// If the bucket was just created concurrently, accept it.
	var owned *types.BucketAlreadyOwnedByYou
	var exists *types.BucketAlreadyExists
	if errors.As(err, &owned) || errors.As(err, &exists) {
		return nil
	}
	return fmt.Errorf("create bucket %q: %w", bucket, err)
}

// DeleteS3Prefix removes every object under `bucket/prefix`. Used to
// clean up per-test S3 state. Errors are returned, not fatal — the
// caller decides whether to log or fail.
func DeleteS3Prefix(ctx context.Context, cfg MinIOConfig, bucket, prefix string) error {
	client := NewMinIOS3Client(cfg)
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects under %s/%s: %w", bucket, prefix, err)
		}
		if len(page.Contents) == 0 {
			continue
		}
		objs := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			objs = append(objs, types.ObjectIdentifier{Key: obj.Key})
		}
		_, err = client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: objs, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("delete objects under %s/%s: %w", bucket, prefix, err)
		}
	}
	return nil
}

// CountS3Prefix returns the number of objects currently under
// `bucket/prefix`. Used by tests that assert cleanup left the prefix
// empty after `t.Cleanup` ran.
func CountS3Prefix(ctx context.Context, cfg MinIOConfig, bucket, prefix string) (int, error) {
	client := NewMinIOS3Client(cfg)
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	count := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		count += len(page.Contents)
	}
	return count, nil
}
