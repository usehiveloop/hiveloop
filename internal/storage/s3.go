package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client wraps the AWS S3 client for agent drive storage.
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client creates a new S3 client configured for the given bucket.
// Pass a non-empty endpoint for S3-compatible stores (MinIO, R2, etc.).
func NewS3Client(bucket, region, endpoint, accessKey, secretKey string) (*S3Client, error) {
	if bucket == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	opts := []func(*s3.Options){
		func(options *s3.Options) {
			options.Region = region
			options.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
		},
	}

	if endpoint != "" {
		opts = append(opts, func(options *s3.Options) {
			options.BaseEndpoint = aws.String(endpoint)
			options.UsePathStyle = true // required for MinIO / local dev
		})
	}

	client := s3.New(s3.Options{}, opts...)

	return &S3Client{client: client, bucket: bucket}, nil
}

// Upload puts an object into the bucket at the given key.
func (sc *S3Client) Upload(ctx context.Context, key string, body io.Reader, contentType string, size int64) error {
	input := &s3.PutObjectInput{
		Bucket:        aws.String(sc.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	}
	_, err := sc.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3 upload %q: %w", key, err)
	}
	return nil
}

// Delete removes an object from the bucket.
func (sc *S3Client) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(sc.bucket),
		Key:    aws.String(key),
	}
	_, err := sc.client.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3 delete %q: %w", key, err)
	}
	return nil
}

// PresignedURL generates a time-limited GET URL for downloading an object.
// Forces Content-Disposition: attachment and Content-Type:
// application/octet-stream to mitigate stored-XSS from any text/html or
// SVG content still sitting in the bucket from before the upload allowlist
// was tightened.
func (sc *S3Client) PresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	presigner := s3.NewPresignClient(sc.client)

	filename := key
	if idx := strings.LastIndex(key, "/"); idx >= 0 && idx < len(key)-1 {
		filename = key[idx+1:]
	}
	disposition := fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilenameForHeader(filename))

	input := &s3.GetObjectInput{
		Bucket:                     aws.String(sc.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(disposition),
		ResponseContentType:        aws.String("application/octet-stream"),
	}
	result, err := presigner.PresignGetObject(ctx, input, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("s3 presign %q: %w", key, err)
	}
	return result.URL, nil
}

// sanitizeFilenameForHeader replaces characters that would break a
// Content-Disposition header value with underscores.
func sanitizeFilenameForHeader(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' || r == '\r' || r == '\n' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
