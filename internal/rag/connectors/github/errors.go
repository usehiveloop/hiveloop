package github

import (
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func entityFailure(entityID, msg string, cause error) *interfaces.ConnectorFailure {
	return interfaces.NewEntityFailure(entityID, msg, nil, nil, cause)
}
