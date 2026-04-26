package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// IngestPayload is the per-source payload for TypeRagIngest tasks. Only
// the source ID is carried — the handler reloads the row by primary key
// so it always sees the freshest configuration. FromBeginning maps to
// RAGIndexAttempt.FromBeginning at backend/onyx/db/models.py:2206-2209
// and is set by the run-once API only.
type IngestPayload struct {
	RAGSourceID   uuid.UUID `json:"rag_source_id"`
	FromBeginning bool      `json:"from_beginning,omitempty"`
}

// PermSyncPayload is the per-source payload for TypeRagPermSync tasks.
type PermSyncPayload struct {
	RAGSourceID uuid.UUID `json:"rag_source_id"`
}

// PrunePayload is the per-source payload for TypeRagPrune tasks.
type PrunePayload struct {
	RAGSourceID uuid.UUID `json:"rag_source_id"`
}

// NewIngestTask builds an asynq.Task for TypeRagIngest. The opts argument
// is appended to the default queue + retry settings; callers (the
// scheduler scan) pass asynq.Unique(ttl) to suppress duplicates within
// the scan-tick window.
func NewIngestTask(p IngestPayload, opts ...asynq.Option) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal ingest payload: %w", err)
	}
	full := append([]asynq.Option{
		asynq.Queue(QueueRagWork),
		asynq.MaxRetry(0),
	}, opts...)
	return asynq.NewTask(TypeRagIngest, body, full...), nil
}

// NewPermSyncTask builds an asynq.Task for TypeRagPermSync.
func NewPermSyncTask(p PermSyncPayload, opts ...asynq.Option) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal perm_sync payload: %w", err)
	}
	full := append([]asynq.Option{
		asynq.Queue(QueueRagWork),
		asynq.MaxRetry(0),
	}, opts...)
	return asynq.NewTask(TypeRagPermSync, body, full...), nil
}

// NewPruneTask builds an asynq.Task for TypeRagPrune.
func NewPruneTask(p PrunePayload, opts ...asynq.Option) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal prune payload: %w", err)
	}
	full := append([]asynq.Option{
		asynq.Queue(QueueRagWork),
		asynq.MaxRetry(0),
	}, opts...)
	return asynq.NewTask(TypeRagPrune, body, full...), nil
}

// UnmarshalIngest parses an IngestPayload from a task body. Returned
// errors are wrapped with the task type so logs identify the bad task
// shape unambiguously.
func UnmarshalIngest(body []byte) (IngestPayload, error) {
	var p IngestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return IngestPayload{}, fmt.Errorf("unmarshal %s payload: %w", TypeRagIngest, err)
	}
	if p.RAGSourceID == uuid.Nil {
		return IngestPayload{}, fmt.Errorf("%s: rag_source_id required", TypeRagIngest)
	}
	return p, nil
}

// UnmarshalPermSync parses a PermSyncPayload from a task body.
func UnmarshalPermSync(body []byte) (PermSyncPayload, error) {
	var p PermSyncPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return PermSyncPayload{}, fmt.Errorf("unmarshal %s payload: %w", TypeRagPermSync, err)
	}
	if p.RAGSourceID == uuid.Nil {
		return PermSyncPayload{}, fmt.Errorf("%s: rag_source_id required", TypeRagPermSync)
	}
	return p, nil
}

// UnmarshalPrune parses a PrunePayload from a task body.
func UnmarshalPrune(body []byte) (PrunePayload, error) {
	var p PrunePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return PrunePayload{}, fmt.Errorf("unmarshal %s payload: %w", TypeRagPrune, err)
	}
	if p.RAGSourceID == uuid.Nil {
		return PrunePayload{}, fmt.Errorf("%s: rag_source_id required", TypeRagPrune)
	}
	return p, nil
}
