// Package connectors is the centralised side-effect-import target for
// every concrete connector implementation. Importing this package
// triggers each connector's init() function, registering its factory
// with internal/rag/connectors/interfaces.
//
// cmd/server (or any other binary that wants the connector registry
// populated) imports this package once for its side effect — see e.g.
// cmd/server/worker.go after the scheduler integration in Phase 3C.
//
// New connectors get a one-line _ import here and nothing else; never
// add behaviour to this package itself.
package connectors

import (
	// GitHub PR + Issue connector.
	_ "github.com/usehiveloop/hiveloop/internal/rag/connectors/github"
)
