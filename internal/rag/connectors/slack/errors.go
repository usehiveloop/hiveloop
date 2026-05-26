package slack

import (
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func entityFailure(entityID, msg string, cause error) *interfaces.ConnectorFailure {
	return interfaces.NewEntityFailure(entityID, msg, nil, nil, cause)
}

func docFailure(docID, docLink, msg string, cause error) *interfaces.ConnectorFailure {
	return interfaces.NewDocumentFailure(docID, docLink, msg, cause)
}
