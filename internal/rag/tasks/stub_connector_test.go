package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

type stubConnector struct {
	kind string

	docs            []interfaces.Document
	failures        map[int]*interfaces.ConnectorFailure
	delayBetween    time.Duration
	openErr         error
	finalCheckpoint json.RawMessage

	permSet []interfaces.DocExternalAccess

	slimIDs []string

	emittedCount atomic.Int32
}

func (s *stubConnector) Kind() string { return s.kind }

func (s *stubConnector) ValidateConfig(_ context.Context, _ interfaces.Source) error { return nil }

func (s *stubConnector) Run(
	ctx context.Context,
	_ interfaces.Source,
	_ json.RawMessage,
	_ time.Time,
	_ time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	out := make(chan interfaces.DocumentOrFailure, 16)
	go func() {
		defer close(out)
		for i := range s.docs {
			if ctx.Err() != nil {
				return
			}
			if f, ok := s.failures[i]; ok && f != nil {
				select {
				case <-ctx.Done():
					return
				case out <- interfaces.NewDocFailure(f):
				}
			} else {
				doc := s.docs[i]
				select {
				case <-ctx.Done():
					return
				case out <- interfaces.NewDocResult(&doc):
					s.emittedCount.Add(1)
				}
			}
			if s.delayBetween > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.delayBetween):
				}
			}
		}
	}()
	return out, nil
}

func (s *stubConnector) FinalCheckpoint() (json.RawMessage, error) {
	if len(s.finalCheckpoint) == 0 {
		return nil, nil
	}
	return s.finalCheckpoint, nil
}

func (s *stubConnector) SyncDocPermissions(
	ctx context.Context,
	_ interfaces.Source,
) (<-chan interfaces.DocExternalAccessOrFailure, error) {
	out := make(chan interfaces.DocExternalAccessOrFailure, 8)
	go func() {
		defer close(out)
		for i := range s.permSet {
			access := s.permSet[i]
			select {
			case <-ctx.Done():
				return
			case out <- interfaces.NewAccessResult(&access):
			}
		}
	}()
	return out, nil
}

func (s *stubConnector) SyncExternalGroups(
	ctx context.Context,
	_ interfaces.Source,
) (<-chan interfaces.ExternalGroupOrFailure, error) {
	out := make(chan interfaces.ExternalGroupOrFailure)
	close(out)
	return out, nil
}

func (s *stubConnector) ListAllSlim(
	ctx context.Context,
	_ interfaces.Source,
) (<-chan interfaces.SlimDocOrFailure, error) {
	out := make(chan interfaces.SlimDocOrFailure, 8)
	go func() {
		defer close(out)
		for _, id := range s.slimIDs {
			slim := interfaces.SlimDocument{DocID: id}
			select {
			case <-ctx.Done():
				return
			case out <- interfaces.NewSlimResult(&slim):
			}
		}
	}()
	return out, nil
}

// stubRegistry must use a unique kind per test — the connector
// registry rejects duplicates package-globally and has no reset helper.
var stubRegistry = struct {
	stubs map[string]*stubConnector
}{stubs: map[string]*stubConnector{}}

func registerStub(kind string, s *stubConnector) {
	s.kind = kind
	stubRegistry.stubs[kind] = s
	interfaces.Register(kind, func(_ interfaces.Source, _ *nango.Client) (interfaces.Connector, error) {
		got, ok := stubRegistry.stubs[kind]
		if !ok {
			return nil, errors.New("stub: kind not registered: " + kind)
		}
		return got, nil
	})
}

var (
	_ interfaces.Connector          = (*stubConnector)(nil)
	_ ragtasks.RunnableCheckpointed = (*stubConnector)(nil)
	_ interfaces.PermSyncConnector  = (*stubConnector)(nil)
	_ interfaces.SlimConnector      = (*stubConnector)(nil)
)
