package tasks

import (
	"testing"
	"time"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// computeIngestWindow ports Onyx's run_docfetching.py:419-451 window
// logic. These tests pin the four interesting cases that cover the
// per-source ingest cadence.

func TestComputeIngestWindow_NoIndexingStartNoLastSuccess(t *testing.T) {
	src := &ragmodel.RAGSource{}
	start, end := computeIngestWindow(src, false)
	if !start.IsZero() {
		t.Errorf("expected zero (epoch) start, got %v", start)
	}
	if end.IsZero() {
		t.Error("expected non-zero end")
	}
}

func TestComputeIngestWindow_LastSuccessRewindsByOverlap(t *testing.T) {
	last := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	src := &ragmodel.RAGSource{LastSuccessfulIndexTime: &last}
	start, _ := computeIngestWindow(src, false)
	want := last.Add(-pollOverlap)
	if !start.Equal(want) {
		t.Errorf("expected start %v (last - 5min), got %v", want, start)
	}
}

func TestComputeIngestWindow_IndexingStartFloorsTheRewind(t *testing.T) {
	indexingStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC) // last - 5min would land before indexingStart
	src := &ragmodel.RAGSource{
		IndexingStart:           &indexingStart,
		LastSuccessfulIndexTime: &last,
	}
	start, _ := computeIngestWindow(src, false)
	if !start.Equal(indexingStart) {
		t.Errorf("expected start floored to IndexingStart %v, got %v", indexingStart, start)
	}
}

func TestComputeIngestWindow_FromBeginningIgnoresLastSuccess(t *testing.T) {
	indexingStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	src := &ragmodel.RAGSource{
		IndexingStart:           &indexingStart,
		LastSuccessfulIndexTime: &last,
	}
	start, _ := computeIngestWindow(src, true)
	if !start.Equal(indexingStart) {
		t.Errorf("expected start = IndexingStart %v on from_beginning, got %v", indexingStart, start)
	}
}
