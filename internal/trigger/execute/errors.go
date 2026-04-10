package execute

import "errors"

var (
	// ErrNilNango means the executor was constructed without a NangoProxy.
	// Always a programmer error.
	ErrNilNango = errors.New("execute: nango proxy is required")

	// ErrNilCatalog means the executor was constructed without a catalog
	// reference. Always a programmer error.
	ErrNilCatalog = errors.New("execute: catalog is required")

	// ErrRequiredContextFailed is returned when a non-optional context action
	// fails. The caller should not proceed to create a conversation.
	ErrRequiredContextFailed = errors.New("execute: required context action failed")
)
