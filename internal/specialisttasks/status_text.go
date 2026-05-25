package specialisttasks

import (
	"fmt"
	"strings"
	"time"
)

func (r TaskStatusResponse) Text() string {
	lines := []string{
		fmt.Sprintf("Specialist task %s", r.TaskID),
		"Specialist: " + r.SpecialistSlug,
		"Status: " + r.Status,
		"Created: " + r.CreatedAt.UTC().Format(time.RFC3339),
	}
	if r.EndedAt != nil {
		lines = append(lines, "Ended: "+r.EndedAt.UTC().Format(time.RFC3339))
	}
	if r.LastActivityAt != nil {
		lines = append(lines, "Last activity: "+r.LastActivityAt.UTC().Format(time.RFC3339))
	}
	if r.ActivitySummary != "" {
		lines = append(lines, "Recent activity: "+r.ActivitySummary)
	}
	if r.LatestMessage != "" {
		lines = append(lines, "Latest specialist message: "+r.LatestMessage)
	}
	if r.LatestError != "" {
		lines = append(lines, "Latest error: "+r.LatestError)
	}
	lines = append(lines, "Next action: "+r.NextAction)
	return strings.Join(lines, "\n")
}
