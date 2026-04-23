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
// opResult captures the PASS/FAIL outcome of a single primitive.
type opResult struct {
	name    string
	ok      bool
	latency time.Duration
	detail  string
	err     error
}
