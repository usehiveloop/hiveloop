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

func main() {
	ctx := context.Background()

	endpoint, bucket, accessKey, secretKey, region, runID, uri := setupS3Env()

	fmt.Println("LanceDB Phase 0 spike")
	fmt.Println("=====================")
	fmt.Printf("endpoint=%s bucket=%s prefix=spike-%s region=%s\n", endpoint, bucket, runID, region)
	fmt.Println()

	results := make([]opResult, 0, 7)

	allowHTTP := true
	forcePath := true
	regionStr := region
	accessKeyStr := accessKey
	secretKeyStr := secretKey
	endpointStr := endpoint

	start := time.Now()
	conn, err := lancedb.Connect(ctx, uri, &contracts.ConnectionOptions{
		Region: &regionStr,
		StorageOptions: &contracts.StorageOptions{
			AllowHTTP: &allowHTTP,
			S3Config: &contracts.S3Config{
				AccessKeyID:     &accessKeyStr,
				SecretAccessKey: &secretKeyStr,
				Region:          &regionStr,
				Endpoint:        &endpointStr,
				ForcePathStyle:  &forcePath,
			},
		},
	})
	connLatency := time.Since(start)
	if err != nil {
		results = append(results, opResult{name: "1. Connect (S3/MinIO)", ok: false, latency: connLatency, err: err})
		reportAndExit(results)
	}
	defer conn.Close()
	results = append(results, opResult{name: "1. Connect (S3/MinIO)", ok: true, latency: connLatency, detail: "lancedb.Connect"})

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "org_id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "vector", Type: arrow.FixedSizeListOf(embeddingDim, arrow.PrimitiveTypes.Float32), Nullable: false},
		{Name: "acl", Type: arrow.ListOf(arrow.BinaryTypes.String), Nullable: true},
		{Name: "content", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "is_public", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
		{Name: "doc_updated_at", Type: &arrow.TimestampType{Unit: arrow.Microsecond}, Nullable: false},
	}, nil)

	ldbSchema, err := lancedb.NewSchema(schema)
	if err != nil {
		results = append(results, opResult{name: "2. Create dataset", ok: false, err: fmt.Errorf("NewSchema: %w", err)})
		reportAndExit(results)
	}

	start = time.Now()
	table, err := conn.CreateTable(ctx, tableName, ldbSchema)
	createLatency := time.Since(start)
	if err != nil {
		results = append(results, opResult{name: "2. Create dataset", ok: false, latency: createLatency, err: err})
		reportAndExit(results)
	}
	defer table.Close()
	results = append(results, opResult{name: "2. Create dataset", ok: true, latency: createLatency, detail: "conn.CreateTable"})

	start = time.Now()
	record, err := buildSampleRecord(schema, sampleRows)
	if err != nil {
		results = append(results, opResult{name: "3. Upsert 100 rows", ok: false, err: fmt.Errorf("buildSampleRecord: %w", err)})
		reportAndExit(results)
	}
	defer record.Release()

	if err := table.Add(ctx, record, nil); err != nil {
		results = append(results, opResult{name: "3. Upsert 100 rows", ok: false, latency: time.Since(start), err: err})
		reportAndExit(results)
	}
	insertLatency := time.Since(start)
	count, err := table.Count(ctx)
	if err != nil {
		results = append(results, opResult{name: "3. Upsert 100 rows", ok: false, err: fmt.Errorf("Count: %w", err)})
		reportAndExit(results)
	}
	if count != sampleRows {
		results = append(results, opResult{name: "3. Upsert 100 rows", ok: false, err: fmt.Errorf("expected %d rows, got %d", sampleRows, count)})
		reportAndExit(results)
	}
	results = append(results, opResult{name: "3. Upsert 100 rows", ok: true, latency: insertLatency, detail: fmt.Sprintf("table.Add → %d rows", count)})

	// Filter: org_id = 'org-A' AND (array_has(acl, 'user_email:alice@x.com') OR is_public = true).
	queryVec := generateUnitVector(embeddingDim)
	filter := "org_id = 'org-A' AND (array_has(acl, 'user_email:alice@x.com') OR is_public = true)"

	start = time.Now()
	vectorResults, err := table.VectorSearchWithFilter(ctx, "vector", queryVec, 10, filter)
	vectorLatency := time.Since(start)
	if err != nil {
		results = append(results, opResult{name: "4. Vector search + filter", ok: false, latency: vectorLatency, err: err})
		reportAndExit(results)
	}
	latencyOK := vectorLatency < 100*time.Millisecond
	results = append(results, opResult{
		name:    "4. Vector search + filter (<100ms)",
		ok:     latencyOK,
		latency: vectorLatency,
		detail:  fmt.Sprintf("VectorSearchWithFilter → %d hits", len(vectorResults)),
	})
	if !latencyOK {
		// Not fatal — record and continue to exercise the remaining ops.
		fmt.Printf("  [warn] vector search latency %s exceeds 100ms target\n", vectorLatency)
	}

	// Non-fatal on failure so we can still exercise ops 6 and 7, which are
	// independent of FTS. The overall outcome still reflects this failure.
	ftsErr := table.CreateIndex(ctx, []string{"content"}, contracts.IndexTypeFts)
	var ftsResults []map[string]interface{}
	var ftsLatency time.Duration
	start = time.Now()
	if ftsErr == nil {
		ftsResults, ftsErr = table.FullTextSearchWithFilter(ctx, "content", "doc", filter)
	}
	ftsLatency = time.Since(start)
	if ftsErr != nil {
		results = append(results, opResult{name: "5. FTS with filter", ok: false, latency: ftsLatency, err: ftsErr})
	} else {
		results = append(results, opResult{
			name:    "5. FTS with filter",
			ok:      true,
			latency: ftsLatency,
			detail:  fmt.Sprintf("FullTextSearchWithFilter → %d hits", len(ftsResults)),
		})
	}

	// Find an existing id by selecting one row, then update only its ACL.
	existing, err := table.SelectWithLimit(ctx, 1, 0)
	if err != nil || len(existing) == 0 {
		results = append(results, opResult{name: "6. Metadata-only update", ok: false, err: fmt.Errorf("SelectWithLimit: %v (len=%d)", err, len(existing))})
		reportAndExit(results)
	}
	targetID, ok := existing[0]["id"].(string)
	if !ok {
		results = append(results, opResult{name: "6. Metadata-only update", ok: false, err: fmt.Errorf("unexpected id type: %T", existing[0]["id"])})
		reportAndExit(results)
	}
	originalVector := existing[0]["vector"]

	// Attempt metadata-only ACL update. v0.1.2's Go binding only accepts
	// scalar update values (string/number/bool/null). A `list<string>` column
	// therefore cannot be updated via this API. We try three paths and
	// record whichever succeeds — or all-fail if none do.
	newACLArraySQL := "make_array('user_email:bob@x.com', 'external_group:github_org_x_team_y', 'PUBLIC')"

	attempts := []struct {
		name  string
		value interface{}
	}{
		{name: "[]string literal", value: []string{"user_email:bob@x.com", "external_group:github_org_x_team_y", "PUBLIC"}},
		{name: "SQL make_array as string", value: newACLArraySQL},
		{name: "JSON-array-as-string", value: `["user_email:bob@x.com","external_group:github_org_x_team_y","PUBLIC"]`},
	}
	var updateLatency time.Duration
	var updateDetail string
	var updateErr error
	var verify []map[string]interface{}
	var postACL interface{}
	aclLen := -1
	for _, a := range attempts {
		start = time.Now()
		err = table.Update(ctx, fmt.Sprintf("id = '%s'", targetID), map[string]interface{}{
			"acl": a.value,
		})
		updateLatency = time.Since(start)
		if err == nil {
			updateDetail = "update value mode: " + a.name
			updateErr = nil
			break
		}
		updateErr = fmt.Errorf("%s → %v", a.name, err)
	}
	if updateErr != nil {
		results = append(results, opResult{
			name:    "6. Metadata-only update (no vector rewrite)",
			ok:      false,
			latency: updateLatency,
			err:     updateErr,
		})
		// Continue — op 7 is independent.
		goto op7
	}

	// Verify the vector was untouched.
	verify, err = table.SelectWithFilter(ctx, fmt.Sprintf("id = '%s'", targetID))
	if err != nil || len(verify) == 0 {
		results = append(results, opResult{name: "6. Metadata-only update (no vector rewrite)", ok: false, err: fmt.Errorf("verify select: %v (len=%d)", err, len(verify))})
		reportAndExit(results)
	}
	if !sameVector(originalVector, verify[0]["vector"]) {
		results = append(results, opResult{name: "6. Metadata-only update (no vector rewrite)", ok: false, err: fmt.Errorf("vector CHANGED after metadata-only Update — perm-sync is not safe")})
		reportAndExit(results)
	}
	// Sanity-check the acl column was actually replaced with an array of
	// length 3 (not a JSON-encoded string stuffed into a single cell).
	postACL = verify[0]["acl"]
	switch v := postACL.(type) {
	case []interface{}:
		aclLen = len(v)
	case []string:
		aclLen = len(v)
	}
	if aclLen != 3 {
		results = append(results, opResult{
			name:    "6. Metadata-only update (no vector rewrite)",
			ok:      false,
			latency: updateLatency,
			err:     fmt.Errorf("acl update did not produce a 3-element array — got %T value %v (length=%d)", postACL, postACL, aclLen),
		})
		goto op7
	}
	results = append(results, opResult{
		name:    "6. Metadata-only update (no vector rewrite)",
		ok:      true,
		latency: updateLatency,
		detail:  fmt.Sprintf("table.Update on id=%s; vector bytes unchanged, acl→3 elements (%s)", targetID, updateDetail),
	})

op7:
	start = time.Now()
	if err := table.Delete(ctx, fmt.Sprintf("id = '%s'", targetID)); err != nil {
		results = append(results, opResult{name: "7. Delete by id", ok: false, latency: time.Since(start), err: err})
		reportAndExit(results)
	}
	deleteLatency := time.Since(start)
	postCount, err := table.Count(ctx)
	if err != nil {
		results = append(results, opResult{name: "7. Delete by id", ok: false, err: fmt.Errorf("Count post-delete: %w", err)})
		reportAndExit(results)
	}
	if postCount != sampleRows-1 {
		results = append(results, opResult{name: "7. Delete by id", ok: false, err: fmt.Errorf("expected %d rows after delete, got %d", sampleRows-1, postCount)})
		reportAndExit(results)
	}
	results = append(results, opResult{
		name:    "7. Delete by id",
		ok:      true,
		latency: deleteLatency,
		detail:  fmt.Sprintf("table.Delete → %d → %d rows", sampleRows, postCount),
	})

	reportAndExit(results)
}

// envDefault returns os.Getenv(key), or the fallback if unset.
