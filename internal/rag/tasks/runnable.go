package tasks

import (
	"context"
	"encoding/json"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// RunnableCheckpointed is the non-generic adapter interface the ingest
// handler drives. Real connectors implement this alongside their
// generic CheckpointedConnector[T] surface — the handler can't call
// the generic method directly because T is erased at the factory
// boundary.
//
// The contract is identical to CheckpointedConnector.LoadFromCheckpoint
// modulo the checkpoint argument: callers pass the persisted bytes (or
// nil for "from beginning") and the connector handles unmarshalling
// internally.
type RunnableCheckpointed interface {
	interfaces.Connector

	// Run starts (or resumes) an ingest pass. checkpointJSON is the
	// raw bytes from the previous attempt's CheckpointPointer or nil
	// for a fresh run. The returned channel is closed when the run
	// completes; the connector may emit a final checkpoint via
	// FinalCheckpoint() (also non-generic).
	Run(
		ctx context.Context,
		src interfaces.Source,
		checkpointJSON json.RawMessage,
		start, end time.Time,
	) (<-chan interfaces.DocumentOrFailure, error)

	// FinalCheckpoint returns the marshalled checkpoint produced after
	// the most recent Run completes. Called once after the channel
	// closes; the bytes are persisted in RAGIndexAttempt.CheckpointPointer.
	FinalCheckpoint() (json.RawMessage, error)
}

// RunnablePermSync is the non-generic adapter for PermSyncConnector.
type RunnablePermSync interface {
	interfaces.Connector
	interfaces.PermSyncConnector
}

// RunnableSlim is the non-generic adapter for SlimConnector.
type RunnableSlim interface {
	interfaces.Connector
	interfaces.SlimConnector
}
