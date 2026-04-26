package interfaces

import (
	"context"
	"encoding/json"
	"time"
)

// Source is the contract the connector layer needs from a RAGSource row.
//
// DEVIATION (from the Tranche 3B agent brief): the brief specifies
// concrete `*ragmodel.RAGSource` in factory/validator signatures.
// RAGSource lives in Tranche 3A (data model), which is executed in
// parallel to this tranche per the plan's Wave 1 launch ordering
// ("3A + 3B parallel; independent files, no shared code"). Depending
// on the concrete struct here would create a cross-tranche Go import
// that can't be satisfied until 3A merges.
//
// The Go-idiomatic solution is "interface at consumer": we declare the
// minimum behavior this layer needs from a RAGSource, and 3A's struct
// satisfies it structurally. Tranche 3C's scheduler will pass a real
// *ragmodel.RAGSource (which will trivially satisfy this interface via
// method additions or a thin adapter).
//
// The methods mirror the RAGSource fields documented in the Phase 3
// plan at plans/onyx-port-phase3.md (Tranche 3A schema section).
type Source interface {
	// SourceID returns the RAGSource.ID (uuid, rendered as string).
	// Used as the lock key + attempt-attribution identifier.
	SourceID() string

	// OrgID returns the owning organization's UUID as a string.
	// Every connector query must be scoped by this.
	OrgID() string

	// SourceKind returns the connector kind ("github", "notion", ...)
	// that was used to register the factory. Matches Connector.Kind().
	SourceKind() string

	// Config returns the raw JSON config blob from RAGSource.Config.
	// Connectors parse this into their own connector-specific shape
	// during ValidateConfig.
	Config() json.RawMessage
}

// Connector is the base trait every connector implements. It identifies
// the provider kind (which matches InIntegration.Provider and the
// Factory registration key) and validates the per-source configuration
// at registration time — before any ingest attempt fires.
//
// Onyx analog: BaseConnector at
// backend/onyx/connectors/interfaces.py:43-114. We don't port the full
// Onyx surface (load_credentials, parse_metadata, oauth methods, raw
// file callback, etc.) — those live above or below this layer in
// Hiveloop's architecture (Nango handles creds; parse_metadata is a
// pure helper; OAuth is handled at the InConnection layer).
type Connector interface {
	// Kind returns the connector identifier (e.g. "github"). This
	// MUST match the key used at registry.Register time.
	Kind() string

	// ValidateConfig inspects src.Config() and returns an error if
	// the configuration is malformed or references unavailable
	// resources. Called at Create time (Tranche 3E) before the first
	// ingest is enqueued.
	//
	// Onyx analog: BaseConnector.validate_connector_settings at
	// interfaces.py:71-77.
	ValidateConfig(ctx context.Context, src Source) error
}

// CheckpointedConnector yields documents in a resumable stream. It
// returns a channel that emits DocumentOrFailure items; the scheduler
// drains the channel until it closes. The connector persists its own
// state in the checkpoint T between runs — the scheduler stores the
// marshaled bytes in RAGIndexAttempt.CheckpointPointer (Phase 1B) +
// the FileStore (Phase 2B).
//
// T is constrained to Checkpoint (a marker interface) so that random
// structs can't be silently passed in. Each connector defines its own
// concrete checkpoint type (e.g. GitHubCheckpoint in Tranche 3D).
//
// Onyx analog: CheckpointedConnector at
// backend/onyx/connectors/interfaces.py:266-302. Onyx uses a generator
// that returns the checkpoint as its final value (Python's
// `yield from ... return` pattern). Go lacks that, so we return the
// checkpoint-advance separately: the connector mutates the checkpoint
// via its own state, and the scheduler reads the latest checkpoint
// before closing the channel. UnmarshalCheckpoint owns reading
// persisted bytes back into T.
type CheckpointedConnector[T Checkpoint] interface {
	Connector

	// LoadFromCheckpoint starts or resumes an ingest run. The returned
	// channel emits DocumentOrFailure items in no particular order; it
	// is closed when the run finishes (either naturally or on ctx
	// cancellation). The start and end bounds describe the poll
	// window; connectors may ignore them if their checkpoint already
	// pins a position.
	LoadFromCheckpoint(
		ctx context.Context,
		src Source,
		cp T,
		start, end time.Time,
	) (<-chan DocumentOrFailure, error)

	// DummyCheckpoint returns the zero-value checkpoint used for a
	// fresh "from-beginning" run.
	// Onyx analog: BaseConnector.build_dummy_checkpoint at
	// interfaces.py:113-114.
	DummyCheckpoint() T

	// UnmarshalCheckpoint parses persisted bytes back into T. Called
	// by the scheduler (Tranche 3C) when resuming a run.
	// Onyx analog: CheckpointedConnector.validate_checkpoint_json at
	// interfaces.py:299-302.
	UnmarshalCheckpoint(raw json.RawMessage) (T, error)
}

// PermSyncConnector refreshes per-document ACLs and syncs external
// group membership without re-ingesting document content. Called
// periodically by the scheduler (Tranche 3C) at
// Source.PermSyncFreqSeconds; results are streamed through channels so
// large result sets don't have to fit in memory.
//
// Onyx analog (split across two modules):
//   - backend/ee/onyx/external_permissions/<provider>/doc_sync.py
//   - backend/ee/onyx/external_permissions/<provider>/group_sync.py
//
// We unify them under one trait because both operations share the
// same connector instance + Nango auth and typically reuse the same
// HTTP client state.
type PermSyncConnector interface {
	Connector

	// SyncDocPermissions streams the current ACL for every document
	// this source owns. The scheduler merges results into Rust-side
	// ACL via UpdateACL (Phase 2 gRPC).
	SyncDocPermissions(ctx context.Context, src Source) (<-chan DocExternalAccessOrFailure, error)

	// SyncExternalGroups streams the current external group catalog
	// for this source (e.g. GitHub org teams). The scheduler upserts
	// into RAGExternalUserGroup + junction tables (Phase 1D).
	SyncExternalGroups(ctx context.Context, src Source) (<-chan ExternalGroupOrFailure, error)
}

// EstimatingConnector returns a pre-flight count of how many documents
// the next ingest run will fetch. Optional: implement only when the
// upstream API can answer cheaply (e.g. GitHub's `Link: rel="last"`
// pagination header on `/issues?per_page=1`). Connectors that would
// have to walk the full catalog to count should not implement this —
// the UI falls back to indeterminate progress.
type EstimatingConnector interface {
	Connector

	// EstimateTotal returns the number of documents the next run is
	// expected to ingest. Called once per attempt right after the
	// attempt row is opened and before any batches stream.
	EstimateTotal(ctx context.Context, src Source) (int, error)
}

// SlimConnector lists every current document ID available in the
// source. Used by the pruning loop (Tranche 3C) to detect source-side
// deletions: documents present in our index but no longer in the
// source are scheduled for delete.
//
// Onyx analog: SlimConnector at
// backend/onyx/connectors/interfaces.py:134-142.
type SlimConnector interface {
	Connector

	// ListAllSlim streams every current document ID. The emitted
	// SlimDocument.ExternalAccess MAY be populated if the slim
	// listing can cheaply surface permission info — the scheduler
	// will treat a non-nil ExternalAccess as an authoritative ACL
	// update and skip a separate perm_sync pass for that document.
	ListAllSlim(ctx context.Context, src Source) (<-chan SlimDocOrFailure, error)
}
