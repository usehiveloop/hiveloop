package github

import (
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func entityFailure(entityID, msg string, cause error) *interfaces.ConnectorFailure {
	return interfaces.NewEntityFailure(entityID, msg, nil, nil, cause)
}
