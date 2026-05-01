package github

import (
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func init() {
	interfaces.Register(Kind, Build)
	// Nango exposes "github" (OAuth) and "github-app" as distinct
	// providers; both share this connector. github-app-code-reviews is
	// a second GitHub App used so a separate bot identity can review PRs
	// opened by the primary github-app — same connector logic.
	interfaces.Register("github-app", Build)
	interfaces.Register("github-app-code-reviews", Build)
}
