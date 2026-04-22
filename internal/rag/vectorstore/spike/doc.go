//go:build !lancedb_spike
// +build !lancedb_spike

// Package main is the Phase 0 LanceDB Go-binding verification spike.
//
// The real implementation is in main.go behind the `lancedb_spike` build
// tag — run via `make rag-spike`. That build tag keeps the CGO
// dependency on github.com/lancedb/lancedb-go out of normal `go build`
// and `go test` invocations, so developers without the native library
// installed can still build and test the rest of the tree. See
// internal/rag/doc/SPIKE_RESULT.md for the outcome.
package main

import "fmt"

// main exists only so `go build ./...` succeeds without the
// lancedb_spike build tag. The real work lives in main.go, gated by
// //go:build lancedb_spike.
func main() {
	fmt.Println("rag vectorstore spike: rebuild with `-tags lancedb_spike` to run (or use `make rag-spike`)")
}
