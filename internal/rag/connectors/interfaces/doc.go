// Package interfaces defines the connector contract every data-source
// integration implements. It is the single source of truth for:
//
//   - The trait hierarchy (Connector, CheckpointedConnector[T],
//     PermSyncConnector, SlimConnector).
//   - The neutral document shapes (Document, Section, SlimDocument).
//   - The external-access shapes (ExternalAccess, DocExternalAccess,
//     ExternalGroup).
//   - The failure-propagation union types
//     (ConnectorFailure, DocumentOrFailure, SlimDocOrFailure,
//     DocExternalAccessOrFailure, ExternalGroupOrFailure).
//   - The factory registry (Register, Lookup, RegisteredKinds).
//
// This package is pure — no external service dependencies, no database,
// no network. It is consumed by the scheduler (Tranche 3C) and the
// concrete connector packages (github in Tranche 3D; notion / slack in
// later phases).
//
// Ports backend/onyx/connectors/interfaces.py — specifically the
// CheckpointedConnector, SlimConnector, and related abstract-base
// protocols from Onyx. The Hiveloop port is Go-idiomatic (channels +
// generics + interface constraints rather than Python generators +
// ABCs) while preserving the exact semantic contract so downstream
// behavior matches upstream Onyx where we've deliberately ported it.
package interfaces
