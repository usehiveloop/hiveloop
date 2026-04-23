package sandbox

import (
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func buildRepoContext(resources model.JSON) string {
	if resources == nil || len(resources) == 0 {
		return ""
	}

	type repo struct {
		id   string
		name string
	}
	var repos []repo

	for _, resourceTypes := range resources {
		typesMap, ok := resourceTypes.(map[string]any)
		if !ok {
			continue
		}
		repoList, ok := typesMap["repository"]
		if !ok {
			continue
		}
		repoSlice, ok := repoList.([]any)
		if !ok {
			continue
		}
		for _, item := range repoSlice {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			repoID, _ := itemMap["id"].(string)
			repoName, _ := itemMap["name"].(string)
			if repoID != "" && repoName != "" {
				repos = append(repos, repo{id: repoID, name: repoName})
			}
		}
	}

	if len(repos) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("── CLONED REPOSITORIES ──\n\n")
	builder.WriteString("The following GitHub repositories have been cloned into your workspace:\n\n")
	for _, repo := range repos {
		builder.WriteString(fmt.Sprintf("  - %s → /home/daytona/repos/%s\n", repo.id, repo.name))
	}
	builder.WriteString("\nYou can read, search, and modify files in these directories directly.")
	return builder.String()
}
