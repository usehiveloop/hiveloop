package tasks

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/system"
)

func parseUUIDs(rawIDs []string, label, code string) ([]uuid.UUID, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}
	seen := make(map[uuid.UUID]struct{}, len(rawIDs))
	out := make([]uuid.UUID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, &system.ResolveError{
				Code:    code,
				Message: fmt.Sprintf("invalid %s %q", label, raw),
			}
		}
		if _, dup := seen[parsed]; dup {
			continue
		}
		seen[parsed] = struct{}{}
		out = append(out, parsed)
	}
	return out, nil
}

func missingMessage(label string, requested, found []uuid.UUID) string {
	foundSet := make(map[uuid.UUID]struct{}, len(found))
	for _, id := range found {
		foundSet[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := foundSet[id]; !ok {
			return fmt.Sprintf("%s %s not found in this workspace", label, id)
		}
	}
	return fmt.Sprintf("one or more %s ids not found in this workspace", label)
}
