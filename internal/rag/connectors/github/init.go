package github

import (
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func init() {
	interfaces.Register(Kind, Build)
	// Nango exposes GitHub via two provider records: "github" (OAuth)
	// and "github-app" (GitHub App). Both speak the same REST API and
	// share this connector implementation.
	interfaces.Register("github-app", Build)
}
