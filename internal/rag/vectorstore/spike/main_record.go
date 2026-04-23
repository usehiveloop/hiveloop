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
func buildSampleRecord(schema *arrow.Schema, count int) (arrow.Record, error) {
	pool := memory.NewGoAllocator()

	ids := make([]string, count)
	orgIDs := make([]string, count)
	contents := make([]string, count)
	isPublic := make([]bool, count)
	updatedAt := make([]arrow.Timestamp, count)
	allVectors := make([]float32, count*embeddingDim)
	acls := make([][]string, count)

	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		ids[i] = fmt.Sprintf("doc-%04d", i)
		if i%2 == 0 {
			orgIDs[i] = "org-A"
		} else {
			orgIDs[i] = "org-B"
		}
		contents[i] = fmt.Sprintf("sample doc number %d about widgets and processes", i)
		isPublic[i] = i%5 == 0
		updatedAt[i] = arrow.Timestamp(now.Add(time.Duration(i) * time.Minute).UnixMicro())

		vec := generateUnitVector(embeddingDim)
		copy(allVectors[i*embeddingDim:(i+1)*embeddingDim], vec)

		switch i % 3 {
		case 0:
			acls[i] = []string{"user_email:alice@x.com", "PUBLIC"}
		case 1:
			acls[i] = []string{"user_email:bob@x.com", "external_group:github_org_x_team_y"}
		case 2:
			acls[i] = []string{"external_group:github_org_x_team_y"}
		}
	}

	idBuilder := array.NewStringBuilder(pool)
	idBuilder.AppendValues(ids, nil)
	idArr := idBuilder.NewArray()
	defer idArr.Release()

	orgBuilder := array.NewStringBuilder(pool)
	orgBuilder.AppendValues(orgIDs, nil)
	orgArr := orgBuilder.NewArray()
	defer orgArr.Release()

	contentBuilder := array.NewStringBuilder(pool)
	contentBuilder.AppendValues(contents, nil)
	contentArr := contentBuilder.NewArray()
	defer contentArr.Release()

	boolBuilder := array.NewBooleanBuilder(pool)
	boolBuilder.AppendValues(isPublic, nil)
	boolArr := boolBuilder.NewArray()
	defer boolArr.Release()

	tsBuilder := array.NewTimestampBuilder(pool, &arrow.TimestampType{Unit: arrow.Microsecond})
	tsBuilder.AppendValues(updatedAt, nil)
	tsArr := tsBuilder.NewArray()
	defer tsArr.Release()

	// Vector column — FixedSizeList<Float32, embeddingDim>.
	vecValuesBuilder := array.NewFloat32Builder(pool)
	vecValuesBuilder.AppendValues(allVectors, nil)
	vecValuesArr := vecValuesBuilder.NewArray()
	defer vecValuesArr.Release()

	vecListType := arrow.FixedSizeListOf(embeddingDim, arrow.PrimitiveTypes.Float32)
	vecData := array.NewData(vecListType, count, []*memory.Buffer{nil}, []arrow.ArrayData{vecValuesArr.Data()}, 0, 0)
	vecArr := array.NewFixedSizeListData(vecData)
	defer vecArr.Release()

	// ACL column — List<Utf8>.
	aclBuilder := array.NewListBuilder(pool, arrow.BinaryTypes.String)
	aclStrBuilder := aclBuilder.ValueBuilder().(*array.StringBuilder)
	for _, a := range acls {
		aclBuilder.Append(true)
		for _, tok := range a {
			aclStrBuilder.Append(tok)
		}
	}
	aclArr := aclBuilder.NewArray()
	defer aclArr.Release()

	columns := []arrow.Array{idArr, orgArr, vecArr, aclArr, contentArr, boolArr, tsArr}
	rec := array.NewRecord(schema, columns, int64(count))
	return rec, nil
}

// generateUnitVector returns a random unit-norm []float32 of length dim.
func generateUnitVector(dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = mrand.Float32()*2 - 1
	}
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range v {
			v[i] /= norm
		}
	}
	return v
}

// sameVector compares two vector column values (as returned in Select result
// maps) byte-for-byte. Both sides may be []float32, []interface{}, or a
// FixedSizeList representation depending on the binding's decoding.
func sameVector(a, b interface{}) bool {
	as := toFloat32Slice(a)
	bs := toFloat32Slice(b)
	if as == nil || bs == nil {
		return false
	}
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func toFloat32Slice(x interface{}) []float32 {
	switch v := x.(type) {
	case []float32:
		return v
	case []interface{}:
		out := make([]float32, len(v))
		for i, raw := range v {
			switch r := raw.(type) {
			case float32:
				out[i] = r
			case float64:
				out[i] = float32(r)
			default:
				return nil
			}
		}
		return out
	default:
		return nil
	}
}

// reportAndExit prints a PASS/FAIL line per op and exits with 0 if every op
// passed, non-zero otherwise.
func reportAndExit(results []opResult) {
	fmt.Println()
	fmt.Println("Results")
	fmt.Println("-------")
	allOK := true
	for _, r := range results {
		status := "PASS"
		if !r.ok {
			status = "FAIL"
			allOK = false
		}
		line := fmt.Sprintf("  [%s] %s (%s)", status, r.name, r.latency)
		if r.detail != "" {
			line += " — " + r.detail
		}
		if r.err != nil {
			line += " — err: " + r.err.Error()
		}
		fmt.Println(line)
	}

	fmt.Println()
	if allOK && len(results) == 7 {
		fmt.Println("OVERALL: PASS (all 7 primitives verified)")
		os.Exit(0)
	}
	fmt.Printf("OVERALL: FAIL (%d/%d primitives verified)\n", countOK(results), 7)
	os.Exit(1)
}

func countOK(results []opResult) int {
	n := 0
	for _, r := range results {
		if r.ok {
			n++
		}
	}
	return n
}
