package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	tmtypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
)

// Stream uploads body to the bucket at key using the transfer manager so
// arbitrarily large objects (multi-GB videos) flow through without
// buffering in memory.
func (p *S3Presigner) Stream(ctx context.Context, key, contentType string, body io.Reader) (*StoredAsset, error) {
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	counted := &countingReader{r: body}

	tm := transfermanager.New(p.client, func(o *transfermanager.Options) {
		o.PartSizeBytes = 8 * 1024 * 1024
		o.Concurrency = 5
	})

	in := &transfermanager.UploadObjectInput{
		Bucket:      aws.String(p.cfg.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        counted,
	}
	if p.cfg.UsePublicACL {
		in.ACL = tmtypes.ObjectCannedACLPublicRead
	}

	if _, err := tm.UploadObject(ctx, in); err != nil {
		return nil, fmt.Errorf("s3 stream upload: %w", err)
	}

	return &StoredAsset{
		Key:       key,
		PublicURL: strings.TrimRight(p.cfg.PublicBase, "/") + "/" + key,
		Bytes:     counted.n,
	}, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
