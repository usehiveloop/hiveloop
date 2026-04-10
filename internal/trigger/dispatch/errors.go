package dispatch

import "errors"

var (
	// ErrUnknownProvider means the dispatch input references a provider with no
	// trigger catalog entry. The dispatcher returns no runs (not an error from
	// the caller's perspective — webhooks for unconfigured providers are ignored).
	ErrUnknownProvider = errors.New("dispatch: unknown provider in trigger catalog")

	// ErrNilConnection means the caller forgot to resolve the connection. This is
	// always a programmer error.
	ErrNilConnection = errors.New("dispatch: connection is required")
)
