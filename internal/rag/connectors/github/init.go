package github

import (
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func init() {
	interfaces.Register(Kind, Build)
	// Nango exposes "github" (OAuth) and "github-app" as distinct
	// providers; both share this connector.
	interfaces.Register("github-app", Build)
}
