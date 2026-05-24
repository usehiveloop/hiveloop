package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client wraps the AWS S3 client for agent drive storage.
type S3Client struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

type S3ObjectInfo struct {
	Key  string
	Size int64
}

// NewS3Client creates a new S3 client configured for the given bucket.
// Pass a non-empty endpoint for S3-compatible stores (MinIO, R2, etc.).
func NewS3Client(bucket, region, endpoint, accessKey, secretKey string) (*S3Client, error) {
	return NewS3ClientWithPresignEndpoint(bucket, region, endpoint, "", accessKey, secretKey)
}

func NewS3ClientWithPresignEndpoint(bucket, region, endpoint, presignEndpoint, accessKey, secretKey string) (*S3Client, error) {
	if bucket == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	client := newS3APIClient(region, endpoint, accessKey, secretKey)
	presignClient := client
	if presignEndpoint != "" && presignEndpoint != endpoint {
		presignClient = newS3APIClient(region, presignEndpoint, accessKey, secretKey)
	}

	return &S3Client{
		client:  client,
		presign: s3.NewPresignClient(presignClient),
		bucket:  bucket,
	}, nil
}

func newS3APIClient(region, endpoint, accessKey, secretKey string) *s3.Client {
	opts := []func(*s3.Options){
		func(options *s3.Options) {
			options.Region = region
			options.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
		},
	}
	if endpoint != "" {
		opts = append(opts, func(options *s3.Options) {
			options.BaseEndpoint = aws.String(endpoint)
			options.UsePathStyle = true
		})
	}
	return s3.New(s3.Options{}, opts...)
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

// Stream uploads body to the bucket without buffering the whole object.
func (sc *S3Client) Stream(ctx context.Context, key string, body io.Reader, contentType string) error {
	if key == "" {
		return fmt.Errorf("S3 key is required")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	tm := transfermanager.New(sc.client, func(o *transfermanager.Options) {
		o.PartSizeBytes = 8 * 1024 * 1024
		o.Concurrency = 5
	})
	_, err := tm.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket:      aws.String(sc.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 stream upload %q: %w", key, err)
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

// Head returns metadata for an object without downloading it.
func (sc *S3Client) Head(ctx context.Context, key string) (*S3ObjectInfo, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(sc.bucket),
		Key:    aws.String(key),
	}
	output, err := sc.client.HeadObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("s3 head %q: %w", key, err)
	}
	return &S3ObjectInfo{
		Key:  key,
		Size: aws.ToInt64(output.ContentLength),
	}, nil
}

// PresignedURL generates a time-limited GET URL for downloading an object.
func (sc *S3Client) PresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(sc.bucket),
		Key:    aws.String(key),
	}
	result, err := sc.presign.PresignGetObject(ctx, input, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("s3 presign %q: %w", key, err)
	}
	return result.URL, nil
}

// PresignedPutURL generates a time-limited PUT URL for uploading an object.
func (sc *S3Client) PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(sc.bucket),
		Key:    aws.String(key),
	}
	result, err := sc.presign.PresignPutObject(ctx, input, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("s3 presign put %q: %w", key, err)
	}
	return result.URL, nil
}
