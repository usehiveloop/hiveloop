package storage_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/storage"
)

const (
	testMinioEndpoint = "http://localhost:9000"
	testMinioAccess   = "minioadmin"
	testMinioSecret   = "minioadmin"
	testMinioBucket   = "public-files-test"
)

func newPresigner(t *testing.T, ttl time.Duration) *storage.S3Presigner {
	t.Helper()
	endpoint := os.Getenv("PUBLIC_ASSETS_S3_ENDPOINT")
	if endpoint == "" {
		endpoint = testMinioEndpoint
	}
	if resp, err := http.Get(endpoint + "/minio/health/ready"); err != nil || resp.StatusCode >= 400 {
		t.Skipf("MinIO not reachable at %s: %v", endpoint, err)
	}
	cfg := storage.PublicAssetsConfig{
		Bucket:     testMinioBucket,
		Region:     "auto",
		Endpoint:   endpoint,
		AccessKey:  testMinioAccess,
		SecretKey:  testMinioSecret,
		PublicBase: endpoint + "/" + testMinioBucket,
		SignTTL:    ttl,
	}
	p, err := storage.NewS3Presigner(cfg)
	if err != nil {
		t.Fatalf("create presigner: %v", err)
	}
	return p
}

func TestSign_AvatarHappyPath(t *testing.T) {
	p := newPresigner(t, 5*time.Minute)
	body := []byte("\x89PNG\r\n\x1a\nfake-bytes-for-test")

	out, err := p.Sign(context.Background(), storage.SignRequest{
		AssetType:   storage.AssetTypeAvatar,
		UserID:      uuid.New(),
		ContentType: "image/png",
		SizeBytes:   int64(len(body)),
		Filename:    "me.png",
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if !strings.HasPrefix(out.Key, "avatars/") {
		t.Fatalf("expected avatars/ prefix, got %q", out.Key)
	}

	req, _ := http.NewRequest(http.MethodPut, out.UploadURL, bytes.NewReader(body))
	for k, v := range out.RequiredHeaders {
		req.Header.Set(k, v)
	}
	req.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from PUT, got %d", resp.StatusCode)
	}

	getResp, err := http.Get(out.PublicURL)
	if err != nil {
		t.Fatalf("public GET: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("public GET: expected 200, got %d", getResp.StatusCode)
	}
	got, _ := io.ReadAll(getResp.Body)
	if !bytes.Equal(got, body) {
		t.Fatalf("public GET returned different bytes")
	}
}

func TestSign_RejectsContentTypeMismatch(t *testing.T) {
	p := newPresigner(t, 5*time.Minute)
	body := []byte("hello")

	out, err := p.Sign(context.Background(), storage.SignRequest{
		AssetType:   storage.AssetTypeAvatar,
		UserID:      uuid.New(),
		ContentType: "image/png",
		SizeBytes:   int64(len(body)),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPut, out.UploadURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("expected 4xx for content-type mismatch, got %d", resp.StatusCode)
	}
}

func TestSign_RejectsOversizeFile(t *testing.T) {
	p := newPresigner(t, 5*time.Minute)
	declared := int64(4 * 1024)
	actual := bytes.Repeat([]byte{0xAB}, int(declared)+1024)

	out, err := p.Sign(context.Background(), storage.SignRequest{
		AssetType:   storage.AssetTypeAvatar,
		UserID:      uuid.New(),
		ContentType: "image/png",
		SizeBytes:   declared,
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPut, out.UploadURL, bytes.NewReader(actual))
	req.Header.Set("Content-Type", "image/png")
	req.ContentLength = int64(len(actual))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("expected 4xx for oversize, got %d", resp.StatusCode)
	}
}

func TestSign_KeyPrefix_PerAssetType(t *testing.T) {
	p := newPresigner(t, time.Minute)
	userID := uuid.New()
	orgID := uuid.New()

	cases := []struct {
		name      string
		req       storage.SignRequest
		wantStart string
	}{
		{
			name: "avatar",
			req: storage.SignRequest{
				AssetType:   storage.AssetTypeAvatar,
				UserID:      userID,
				ContentType: "image/png",
				SizeBytes:   100,
			},
			wantStart: "avatars/" + userID.String() + "/",
		},
		{
			name: "org_logo",
			req: storage.SignRequest{
				AssetType:   storage.AssetTypeOrgLogo,
				UserID:      userID,
				OrgID:       &orgID,
				ContentType: "image/png",
				SizeBytes:   100,
			},
			wantStart: "pub/o/" + orgID.String() + "/",
		},
		{
			name: "generic",
			req: storage.SignRequest{
				AssetType:   storage.AssetTypeGeneric,
				UserID:      userID,
				ContentType: "application/pdf",
				SizeBytes:   100,
			},
			wantStart: "pub/u/" + userID.String() + "/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := p.Sign(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			if !strings.HasPrefix(out.Key, tc.wantStart) {
				t.Fatalf("key %q does not start with %q", out.Key, tc.wantStart)
			}
		})
	}
}

func TestSign_ExpiredURL(t *testing.T) {
	p := newPresigner(t, 1*time.Second)
	body := []byte("expired-test")

	out, err := p.Sign(context.Background(), storage.SignRequest{
		AssetType:   storage.AssetTypeAvatar,
		UserID:      uuid.New(),
		ContentType: "image/png",
		SizeBytes:   int64(len(body)),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	time.Sleep(2 * time.Second)

	req, _ := http.NewRequest(http.MethodPut, out.UploadURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/png")
	req.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("expected 4xx for expired URL, got %d", resp.StatusCode)
	}
}
