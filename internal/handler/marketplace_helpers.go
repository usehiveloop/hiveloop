package handler

import (
	"sort"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func extractRequiredIntegrations(integrations model.JSON) []string {
	if len(integrations) == 0 {
		return []string{}
	}

	var result []string
	for key := range integrations {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
