package interfaces

// Checkpoint is a marker interface constraining the generic type
// parameter of CheckpointedConnector. Each connector defines its own
// concrete checkpoint struct (e.g. GitHubCheckpoint in Tranche 3D) and
// satisfies this constraint by embedding the unexported isCheckpoint()
// method.
//
// The method is deliberately unexported so that only types declared
// within the connector packages (or this package) can satisfy it —
// external callers can't accidentally pass a random struct as a
// checkpoint and have it silently type-check.
//
// Onyx analog: ConnectorCheckpoint at
// backend/onyx/connectors/models.py:462-473. Onyx uses Pydantic BaseModel
// inheritance for the same role; Go uses a marker-method constraint
// because Go lacks structural subtyping for structs.
//
// Checkpoint JSON-serialization contract:
//   - Any Checkpoint MUST round-trip cleanly through encoding/json.
//   - UnmarshalCheckpoint (on CheckpointedConnector) owns parsing the
//     raw bytes back into the concrete type.
type Checkpoint interface {
	// isCheckpoint is a marker method — it has no body semantics.
	// See the package-level comment for the rationale.
	isCheckpoint()
}

// AnyCheckpoint is the simplest possible Checkpoint implementation.
// Connectors that don't need any persisted state between runs (pure
// "start from scratch every time") can use this directly as their T.
// Unit tests in this package also use it to prove the marker-interface
// constraint compiles end-to-end.
type AnyCheckpoint struct {
	// HasMore mirrors Onyx ConnectorCheckpoint.has_more at
	// backend/onyx/connectors/models.py:464 — the one field universal
	// to every Onyx checkpoint.
	HasMore bool `json:"has_more"`
}

// isCheckpoint satisfies the Checkpoint marker interface for AnyCheckpoint.
func (AnyCheckpoint) isCheckpoint() {}
