package github

import (
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func init() {
	interfaces.Register(Kind, Build)

	interfaces.Register("github-app", Build)
	interfaces.Register("github-app-code-reviews", Build)
}
