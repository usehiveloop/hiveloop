package tasks

import (
	"context"
	"encoding/json"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// RunnableCheckpointed is the non-generic adapter the ingest handler
// drives. Connectors implement this alongside their generic
// CheckpointedConnector[T] surface; the handler can't call the generic
// method directly because T is erased at the factory boundary.
type RunnableCheckpointed interface {
	interfaces.Connector

	Run(
		ctx context.Context,
		src interfaces.Source,
		checkpointJSON json.RawMessage,
		start, end time.Time,
	) (<-chan interfaces.DocumentOrFailure, error)

	FinalCheckpoint() (json.RawMessage, error)
}

type RunnablePermSync interface {
	interfaces.Connector
	interfaces.PermSyncConnector
}

type RunnableSlim interface {
	interfaces.Connector
	interfaces.SlimConnector
}
