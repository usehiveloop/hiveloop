package storage

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

type AssetType string

const (
	AssetTypeAvatar  AssetType = "avatar"
	AssetTypeOrgLogo AssetType = "org_logo"
	AssetTypeGeneric AssetType = "generic"
)

type SignRequest struct {
	AssetType   AssetType
	UserID      uuid.UUID
	OrgID       *uuid.UUID
	ContentType string
	SizeBytes   int64
	Filename    string
}

type SignedUpload struct {
	UploadURL       string            `json:"upload_url"`
	UploadMethod    string            `json:"upload_method"`
	RequiredHeaders map[string]string `json:"required_headers"`
	Key             string            `json:"key"`
	PublicURL       string            `json:"public_url"`
	ExpiresAt       time.Time         `json:"expires_at"`
	MaxSizeBytes    int64             `json:"max_size_bytes"`
}

type Presigner interface {
	Sign(ctx context.Context, req SignRequest) (*SignedUpload, error)
	Policy(t AssetType) (AssetPolicy, bool)
}

type AssetPolicy struct {
	MaxBytes     int64
	AllowedTypes map[string]string
	KeyPrefix    func(req SignRequest) (string, error)
}

func defaultPolicies() map[AssetType]AssetPolicy {
	imageTypes := map[string]string{
		"image/png":  "png",
		"image/jpeg": "jpg",
		"image/webp": "webp",
		"image/gif":  "gif",
	}
	genericTypes := map[string]string{
		"image/png":       "png",
		"image/jpeg":      "jpg",
		"image/webp":      "webp",
		"image/gif":       "gif",
		"application/pdf": "pdf",
		"text/plain":      "txt",
	}
	return map[AssetType]AssetPolicy{
		AssetTypeAvatar: {
			MaxBytes:     5 * 1024 * 1024,
			AllowedTypes: imageTypes,
			KeyPrefix: func(req SignRequest) (string, error) {
				return fmt.Sprintf("avatars/%s/", req.UserID), nil
			},
		},
		AssetTypeOrgLogo: {
			MaxBytes:     5 * 1024 * 1024,
			AllowedTypes: imageTypes,
			KeyPrefix: func(req SignRequest) (string, error) {
				if req.OrgID == nil {
					return "", fmt.Errorf("org_id is required for org_logo")
				}
				return fmt.Sprintf("pub/o/%s/", *req.OrgID), nil
			},
		},
		AssetTypeGeneric: {
			MaxBytes:     25 * 1024 * 1024,
			AllowedTypes: genericTypes,
			KeyPrefix: func(req SignRequest) (string, error) {
				return fmt.Sprintf("pub/u/%s/", req.UserID), nil
			},
		},
	}
}

type PublicAssetsConfig struct {
	Bucket      string
	Region      string
	Endpoint    string
	AccessKey   string
	SecretKey   string
	PublicBase  string
	SignTTL     time.Duration
	UsePublicACL bool
}

type S3Presigner struct {
	cfg      PublicAssetsConfig
	client   *s3.Client
	presign  *s3.PresignClient
	policies map[AssetType]AssetPolicy
}

func NewS3Presigner(cfg PublicAssetsConfig) (*S3Presigner, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("public assets bucket is required")
	}
	if cfg.PublicBase == "" {
		return nil, fmt.Errorf("public assets base URL is required")
	}
	if cfg.SignTTL <= 0 {
		cfg.SignTTL = 15 * time.Minute
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.Region = cfg.Region
			if cfg.Region == "" {
				o.Region = "auto"
			}
			o.Credentials = credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
		},
	}
	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.New(s3.Options{}, opts...)
	return &S3Presigner{
		cfg:      cfg,
		client:   client,
		presign:  s3.NewPresignClient(client),
		policies: defaultPolicies(),
	}, nil
}

func (p *S3Presigner) Policy(t AssetType) (AssetPolicy, bool) {
	pol, ok := p.policies[t]
	return pol, ok
}

func (p *S3Presigner) Sign(ctx context.Context, req SignRequest) (*SignedUpload, error) {
	pol, ok := p.policies[req.AssetType]
	if !ok {
		return nil, fmt.Errorf("unknown asset_type %q", req.AssetType)
	}
	ext, ok := pol.AllowedTypes[strings.ToLower(req.ContentType)]
	if !ok {
		return nil, fmt.Errorf("content_type %q not allowed for asset_type %q", req.ContentType, req.AssetType)
	}
	if req.SizeBytes <= 0 {
		return nil, fmt.Errorf("size_bytes must be positive")
	}
	if req.SizeBytes > pol.MaxBytes {
		return nil, fmt.Errorf("size_bytes %d exceeds limit %d for asset_type %q", req.SizeBytes, pol.MaxBytes, req.AssetType)
	}

	prefix, err := pol.KeyPrefix(req)
	if err != nil {
		return nil, err
	}
	key := prefix + buildLeaf(req.Filename, ext)

	put := &s3.PutObjectInput{
		Bucket:        aws.String(p.cfg.Bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(req.ContentType),
		ContentLength: aws.Int64(req.SizeBytes),
	}
	if p.cfg.UsePublicACL {
		put.ACL = "public-read"
	}

	signed, err := p.presign.PresignPutObject(ctx, put, s3.WithPresignExpires(p.cfg.SignTTL))
	if err != nil {
		return nil, fmt.Errorf("presign put: %w", err)
	}

	headers := map[string]string{
		"Content-Type": req.ContentType,
	}
	if p.cfg.UsePublicACL {
		headers["x-amz-acl"] = "public-read"
	}

	return &SignedUpload{
		UploadURL:       signed.URL,
		UploadMethod:    "PUT",
		RequiredHeaders: headers,
		Key:             key,
		PublicURL:       strings.TrimRight(p.cfg.PublicBase, "/") + "/" + key,
		ExpiresAt:       time.Now().Add(p.cfg.SignTTL).UTC(),
		MaxSizeBytes:    pol.MaxBytes,
	}, nil
}

var slugInvalid = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func buildLeaf(filename, ext string) string {
	id := uuid.New().String()
	slug := sanitizeSlug(filename)
	if slug == "" {
		return id + "." + ext
	}
	return id + "-" + slug + "." + ext
}

func sanitizeSlug(filename string) string {
	if filename == "" {
		return ""
	}
	base := path.Base(filename)
	stem := strings.TrimSuffix(base, path.Ext(base))
	stem = slugInvalid.ReplaceAllString(stem, "-")
	stem = strings.Trim(stem, "-.")
	if len(stem) > 40 {
		stem = stem[:40]
	}
	return url.PathEscape(stem)
}
