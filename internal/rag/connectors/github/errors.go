// Thin constructors over interfaces.ConnectorFailure.
//
// Onyx analog: the inline NewDocumentFailure / NewEntityFailure calls
// scattered through connector.py:654-668 + :741-754. We centralise the
// shape so the fetch loops stay readable.
package github

import (
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// entityFailure builds a non-document ConnectorFailure for a repo or
// poll-window-level miss. The scheduler logs the entity ID + the miss
// window so retries can target the same scope.
func entityFailure(entityID, msg string, cause error) *interfaces.ConnectorFailure {
	return interfaces.NewEntityFailure(entityID, msg, nil, nil, cause)
}
