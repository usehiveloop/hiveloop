package employeeruntime

import (
	"encoding/json"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

const runtimeConfigSpecialistsKey = "specialists"

func SpecialistModelOverride(runtimeConfig model.JSON, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" || len(runtimeConfig) == 0 {
		return ""
	}
	specialistsConfig, ok := runtimeConfig[runtimeConfigSpecialistsKey].(map[string]any)
	if !ok {
		return ""
	}
	entry, ok := specialistsConfig[slug].(map[string]any)
	if !ok {
		return ""
	}
	modelID, _ := entry["model"].(string)
	return strings.TrimSpace(modelID)
}

func SetSpecialistModelOverride(runtimeConfig model.JSON, slug string, modelID string) model.JSON {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return cloneRuntimeConfig(runtimeConfig)
	}
	next := cloneRuntimeConfig(runtimeConfig)
	specialistsConfig, ok := next[runtimeConfigSpecialistsKey].(map[string]any)
	if !ok {
		specialistsConfig = map[string]any{}
	}
	entry, ok := specialistsConfig[slug].(map[string]any)
	if !ok {
		entry = map[string]any{}
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		delete(entry, "model")
		if len(entry) == 0 {
			delete(specialistsConfig, slug)
		} else {
			specialistsConfig[slug] = entry
		}
	} else {
		entry["model"] = modelID
		specialistsConfig[slug] = entry
	}
	if len(specialistsConfig) == 0 {
		delete(next, runtimeConfigSpecialistsKey)
	} else {
		next[runtimeConfigSpecialistsKey] = specialistsConfig
	}
	return next
}

func EffectiveSpecialistModel(runtimeConfig model.JSON, slug string, defaultModel string) string {
	if override := SpecialistModelOverride(runtimeConfig, slug); override != "" {
		return override
	}
	return strings.TrimSpace(defaultModel)
}

func cloneRuntimeConfig(raw model.JSON) model.JSON {
	if len(raw) == 0 {
		return model.JSON{}
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return model.JSON{}
	}
	var out model.JSON
	if err := json.Unmarshal(bytes, &out); err != nil {
		return model.JSON{}
	}
	if out == nil {
		return model.JSON{}
	}
	return out
}
