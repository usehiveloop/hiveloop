// Side-effect import target. Importing this package registers the
// "github" factory with the connector registry. The cmd/server binary
// pulls it in via internal/rag/connectors/all.go.
package github

import (
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func init() {
	interfaces.Register(Kind, Build)
}
