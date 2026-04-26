package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type IngestPayload struct {
	RAGSourceID   uuid.UUID `json:"rag_source_id"`
	FromBeginning bool      `json:"from_beginning,omitempty"`
}

type PermSyncPayload struct {
	RAGSourceID uuid.UUID `json:"rag_source_id"`
}

type PrunePayload struct {
	RAGSourceID uuid.UUID `json:"rag_source_id"`
}

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
