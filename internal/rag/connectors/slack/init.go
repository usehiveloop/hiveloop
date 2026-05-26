package slack

import (
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func init() {
	interfaces.Register(Kind, Build)
}
