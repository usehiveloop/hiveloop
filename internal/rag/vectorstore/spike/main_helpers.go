//go:build lancedb_spike
// +build lancedb_spike

// SPDX-License-Identifier: Apache-2.0
//
// Phase 0 LanceDB Go-binding verification spike.
//
// Gate for Phase 1: this binary must exercise the seven primitives we depend
// on and exit 0 with a PASS line for each. If any primitive fails we exit
// non-zero with specific error information.
//
// The seven primitives mirror the behaviour Onyx gets from Vespa's API at
// backend/onyx/document_index/vespa/index.py (write_chunks, update_metadata,
// etc.). See internal/rag/doc/SPIKE_RESEARCH.md for the API mapping.
//
// Run via:
//
//	make rag-spike
//
// Which ensures MinIO is running with the hiveloop-rag-test bucket, sets
// the CGO flags pointing at .lancedb-native/, and invokes:
//
//	go run ./internal/rag/vectorstore/spike
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	mrand "math/rand"
	"os"
	"time"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/lancedb/lancedb-go/pkg/contracts"
	"github.com/lancedb/lancedb-go/pkg/lancedb"
)

const (
	embeddingDim = 2560 // Qwen3-Embedding-4B dimensionality per the plan's locked decisions.
	tableName    = "rag_spike"
	sampleRows   = 100
)

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// setenvIfEmpty sets an env var only if it isn't already set.
func setenvIfEmpty(key, val string) {
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, val)
	}
}

// randomRunID returns 8 hex chars identifying this spike run.
func randomRunID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// buildSampleRecord generates an Arrow Record with `count` rows matching the
// spike schema. Rows are split across two orgs and mix public + private +
// per-user + group-based ACLs so the query filter in op 4 can exercise each
// branch.

func setupS3Env() (endpoint, bucket, accessKey, secretKey, region, runID, uri string) {
	endpoint = envDefault("MINIO_ENDPOINT", "http://localhost:9000")
	bucket = envDefault("MINIO_BUCKET", "hiveloop-rag-test")
	accessKey = envDefault("MINIO_ACCESS_KEY", "minioadmin")
	secretKey = envDefault("MINIO_SECRET_KEY", "minioadmin")
	region = envDefault("MINIO_REGION", "us-east-1")

	setenvIfEmpty("AWS_ACCESS_KEY_ID", accessKey)
	setenvIfEmpty("AWS_SECRET_ACCESS_KEY", secretKey)
	setenvIfEmpty("AWS_REGION", region)
	setenvIfEmpty("AWS_DEFAULT_REGION", region)
	setenvIfEmpty("AWS_ENDPOINT_URL", endpoint)
	setenvIfEmpty("AWS_ENDPOINT", endpoint)
	setenvIfEmpty("AWS_ALLOW_HTTP", "true")
	setenvIfEmpty("AWS_S3_ALLOW_UNSAFE_RENAME", "true")
	setenvIfEmpty("AWS_EC2_METADATA_DISABLED", "true")

	runID = randomRunID()
	uri = fmt.Sprintf("s3://%s/spike-%s", bucket, runID)
	return
}
