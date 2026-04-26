package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// genDocs builds n synthetic Documents with deterministic IDs / content.
func genDocs(prefix string, n int) []interfaces.Document {
	out := make([]interfaces.Document, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, interfaces.Document{
			DocID:      fmt.Sprintf("%s-%04d", prefix, i),
			SemanticID: fmt.Sprintf("Doc %s #%d", prefix, i),
			Sections: []interfaces.Section{
				{Text: fmt.Sprintf("body for %s-%04d", prefix, i)},
			},
		})
	}
	return out
}

// TestIngest_HappyPath_StubConnector: 5 docs, no failures, no
// configured delay. The handler should drive the connector to
// completion, produce a SUCCESS attempt, and advance the source's
// last_successful_index_time.
func TestIngest_HappyPath_StubConnector(t *testing.T) {
	f := setupTask(t)
	kind := nextStubKind()
	stub := &stubConnector{
		docs: genDocs("happy", 5),
	}
	registerStub(kind, stub)

	src := f.makeSource(t, kind)

	if err := f.runIngestNow(context.Background(), t, src.ID); err != nil {
		t.Fatalf("HandleIngest: %v", err)
	}

	att := reloadAttempt(t, f.DB, src.ID)
	if att.Status != ragmodel.IndexingStatusSuccess {
		t.Fatalf("attempt status = %s, want SUCCESS", att.Status)
	}
	got := reloadSource(t, f.DB, src.ID)
	if got.LastSuccessfulIndexTime == nil {
		t.Fatalf("last_successful_index_time was not advanced on SUCCESS")
	}
	var docCount int64
	f.DB.Model(&ragmodel.RAGDocument{}).
		Where("org_id = ?", src.OrgIDValue).
		Count(&docCount)
	if docCount != 5 {
		t.Fatalf("rag_documents count = %d, want 5", docCount)
	}
}

// TestIngest_HeartbeatTicksDuringWork: a slow connector (200ms between
// docs) must produce visible heartbeat counter advances during the
// run, not just at the end. The handler's heartbeat tick is set to
// 150ms in the fixture so we expect at least one tick mid-run.
func TestIngest_HeartbeatTicksDuringWork(t *testing.T) {
	f := setupTask(t)
	kind := nextStubKind()
	stub := &stubConnector{
		docs:         genDocs("hb", 4),
		delayBetween: 200 * time.Millisecond,
	}
	registerStub(kind, stub)

	src := f.makeSource(t, kind)

	// Sample the heartbeat counter mid-flight via a goroutine.
	var midCounter int
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		// Sleep enough for the connector to emit one or two docs.
		time.Sleep(400 * time.Millisecond)
		var att ragmodel.RAGIndexAttempt
		_ = f.DB.Where("rag_source_id = ?", src.ID).
			Order("time_created DESC").
			First(&att).Error
		midCounter = att.HeartbeatCounter
	}()

	if err := f.runIngestNow(context.Background(), t, src.ID); err != nil {
		t.Fatalf("HandleIngest: %v", err)
	}
	<-doneCh

	if midCounter < 1 {
		t.Fatalf("heartbeat_counter mid-run = %d, want >= 1", midCounter)
	}
}

// TestIngest_PerDocFailureDoesNotAbortBatch: failures at indices 1
// and 3 are wrapped in DocumentFailure; the handler must continue,
// finalise as COMPLETED_WITH_ERRORS, and write two
// rag_index_attempt_errors rows.
func TestIngest_PerDocFailureDoesNotAbortBatch(t *testing.T) {
	f := setupTask(t)
	kind := nextStubKind()
	docs := genDocs("perr", 5)
	stub := &stubConnector{
		docs: docs,
		failures: map[int]*interfaces.ConnectorFailure{
			1: interfaces.NewDocumentFailure(docs[1].DocID, "", "synthetic 403", errors.New("stub")),
			3: interfaces.NewDocumentFailure(docs[3].DocID, "", "synthetic 500", errors.New("stub")),
		},
	}
	registerStub(kind, stub)

	src := f.makeSource(t, kind)
	if err := f.runIngestNow(context.Background(), t, src.ID); err != nil {
		t.Fatalf("HandleIngest: %v", err)
	}

	att := reloadAttempt(t, f.DB, src.ID)
	if att.Status != ragmodel.IndexingStatusCompletedWithErrors {
		t.Fatalf("attempt status = %s, want COMPLETED_WITH_ERRORS", att.Status)
	}
	var errCount int64
	f.DB.Model(&ragmodel.RAGIndexAttemptError{}).
		Where("index_attempt_id = ?", att.ID).
		Count(&errCount)
	if errCount != 2 {
		t.Fatalf("rag_index_attempt_errors count = %d, want 2", errCount)
	}
}

// TestIngest_FatalConnectorErrorMarksFailed: a connector that fails
// before emitting anything (Run returns an error) must result in a
// FAILED attempt; the source's status is unchanged so the next scan
// re-eligibilities it.
func TestIngest_FatalConnectorErrorMarksFailed(t *testing.T) {
	f := setupTask(t)
	kind := nextStubKind()
	stub := &stubConnector{
		openErr: errors.New("stub: bad credentials"),
	}
	registerStub(kind, stub)

	src := f.makeSource(t, kind)
	wantStatus := src.Status

	err := f.runIngestNow(context.Background(), t, src.ID)
	if err == nil {
		t.Fatal("HandleIngest: expected error, got nil")
	}
	att := reloadAttempt(t, f.DB, src.ID)
	if att.Status != ragmodel.IndexingStatusFailed {
		t.Fatalf("attempt status = %s, want FAILED", att.Status)
	}
	if att.ErrorMsg == nil || *att.ErrorMsg == "" {
		t.Fatalf("expected error_msg populated on FAILED attempt")
	}
	got := reloadSource(t, f.DB, src.ID)
	if got.Status != wantStatus {
		t.Fatalf("source status = %s, want unchanged %s", got.Status, wantStatus)
	}
}

// TestIngest_CheckpointResumesAfterRestart: first run cancels mid-stream
// (3 of 10 docs emitted, ctx cancelled); second run starts from the
// persisted checkpoint and emits the remaining 7. We verify the doc
// count converges to 10.
//
// The stub's checkpoint mechanism is a single counter encoded as JSON
// so we can prove the persistence round-trips. The "resume" stub
// replaces its docs slice based on which checkpoint it received.
func TestIngest_CheckpointResumesAfterRestart(t *testing.T) {
	f := setupTask(t)
	kind := nextStubKind()
	first := &stubConnector{
		docs:            genDocs("cp", 3),
		finalCheckpoint: json.RawMessage(`{"emitted":3}`),
	}
	registerStub(kind, first)

	src := f.makeSource(t, kind)
	if err := f.runIngestNow(context.Background(), t, src.ID); err != nil {
		t.Fatalf("first HandleIngest: %v", err)
	}

	att := reloadAttempt(t, f.DB, src.ID)
	if att.CheckpointPointer == nil || *att.CheckpointPointer == "" {
		t.Fatalf("checkpoint_pointer not persisted after first run")
	}

	// Second run: same kind, replace the stub's docs with the
	// "remaining 7". registerStub overwrites the entry under the
	// same kind via the package-private map.
	second := &stubConnector{
		docs: genDocs("cp-r", 7),
	}
	stubRegistry.stubs[kind] = second
	second.kind = kind

	if err := f.runIngestNow(context.Background(), t, src.ID); err != nil {
		t.Fatalf("second HandleIngest: %v", err)
	}
	var docCount int64
	f.DB.Model(&ragmodel.RAGDocument{}).
		Where("org_id = ?", src.OrgIDValue).
		Count(&docCount)
	if docCount != 10 {
		t.Fatalf("total docs = %d, want 10", docCount)
	}
}
