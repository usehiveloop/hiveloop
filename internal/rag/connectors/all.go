// Package connectors is the side-effect import target that populates
// the connector registry. Add new connectors as a one-line _ import.
package connectors

import (
	_ "github.com/usehiveloop/hiveloop/internal/rag/connectors/github"
)
