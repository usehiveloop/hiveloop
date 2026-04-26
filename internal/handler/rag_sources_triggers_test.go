package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

func TestSyncTrigger_EnqueuesIngestTask(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)

	rr := post(t, h, "/v1/rag/sources/"+src.ID.String()+"/sync", map[string]any{})
	mustStatus(t, rr, http.StatusAccepted)

	if len(h.Enq.tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(h.Enq.tasks))
	}
	task := h.Enq.tasks[0]
	if task.Type() != ragtasks.TypeRagIngest {
		t.Fatalf("expected %s, got %s", ragtasks.TypeRagIngest, task.Type())
	}
	var payload ragtasks.IngestPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.RAGSourceID != src.ID {
		t.Fatalf("payload source id mismatch")
	}
	if payload.FromBeginning {
		t.Fatalf("expected from_beginning=false")
	}
}

func TestSyncTrigger_FromBeginning(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)

	rr := post(t, h, "/v1/rag/sources/"+src.ID.String()+"/sync", map[string]any{
		"from_beginning": true,
	})
	mustStatus(t, rr, http.StatusAccepted)
	var payload ragtasks.IngestPayload
	_ = json.Unmarshal(h.Enq.tasks[0].Payload(), &payload)
	if !payload.FromBeginning {
		t.Fatalf("expected from_beginning=true")
	}
}

func TestSyncTrigger_DedupesRapidClicks(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)

	rr1 := post(t, h, "/v1/rag/sources/"+src.ID.String()+"/sync", map[string]any{})
	mustStatus(t, rr1, http.StatusAccepted)
	rr2 := post(t, h, "/v1/rag/sources/"+src.ID.String()+"/sync", map[string]any{})
	mustStatus(t, rr2, http.StatusAccepted)

	var second map[string]any
	decodeJSON(t, rr2, &second)
	if second["deduplicated"] != true {
		t.Fatalf("expected deduplicated=true on second click; got %v", second)
	}
	if len(h.Enq.tasks) != 1 {
		t.Fatalf("expected 1 task on the queue, got %d", len(h.Enq.tasks))
	}
}

func TestPermSyncTrigger_CapabilityGate(t *testing.T) {
	h := newRAGHarness(t)
	// WEBSITE kind sidesteps the integration-requires-connection CHECK
	// while still being a legal target for the perm-sync gate test.
	src := h.createSource(t, func(s *ragmodel.RAGSource) {
		s.KindValue = ragmodel.RAGSourceKindWebsite
		s.InConnectionID = nil
	})
	h.CapsAllowAll = false

	rr := post(t, h, "/v1/rag/sources/"+src.ID.String()+"/perm-sync", nil)
	mustStatus(t, rr, http.StatusUnprocessableEntity)
	if !bodyContains(rr, "permission sync") {
		t.Fatalf("expected capability-gate message; got %s", rr.Body.String())
	}
}

func TestPruneTrigger_EnqueuesPruneTask(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)

	rr := post(t, h, "/v1/rag/sources/"+src.ID.String()+"/prune", nil)
	mustStatus(t, rr, http.StatusAccepted)

	if len(h.Enq.tasks) != 1 || h.Enq.tasks[0].Type() != ragtasks.TypeRagPrune {
		t.Fatalf("expected 1 prune task, got %#v", h.Enq.tasks)
	}
}
