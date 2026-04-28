package website

import "github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"

func init() {
	interfaces.Register(Kind, Build)
}
