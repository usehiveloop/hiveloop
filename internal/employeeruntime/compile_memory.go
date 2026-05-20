package employeeruntime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func buildMemoryContext(ctx context.Context, deps CompileDeps, agent *model.Agent) MemoryContext {
	memory := MemoryContext{Entries: []MemoryContextEntry{}, TokenBudget: 1000}
	if deps.Hindsight == nil || agent == nil || agent.OrgID == nil {
		return memory
	}
	recallCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := "Durable company, people, project, decision, policy, preference, technical, customer, and communication-behavior memories relevant to this employee's current work."
	result, err := deps.Hindsight.Recall(recallCtx, hindsight.OrgBankID(*agent.OrgID), &hindsight.RecallRequest{
		Query:     query,
		Budget:    "mid",
		TagGroups: employeeMemoryTagGroups(agent),
	})
	if err != nil || result == nil {
		return memory
	}
	memory.Entries = compactMemoryResults(result.Results, 12, memory.TokenBudget)
	return memory
}

func employeeMemoryTagGroups(agent *model.Agent) []any {
	if agent == nil || agent.OrgID == nil {
		return nil
	}
	tags := []string{"company:" + agent.OrgID.String()}
	return []any{map[string]any{"tags": tags, "match": "all_strict"}}
}

func compactMemoryResults(results []any, maxEntries int, tokenBudget int) []MemoryContextEntry {
	entries := make([]MemoryContextEntry, 0, len(results))
	remainingChars := tokenBudget * 4
	for _, raw := range results {
		if len(entries) >= maxEntries || remainingChars <= 0 {
			break
		}
		entry := memoryEntryFromResult(raw)
		entry.Content = strings.TrimSpace(entry.Content)
		if entry.Content == "" {
			continue
		}
		if len(entry.Content) > remainingChars {
			entry.Content = entry.Content[:remainingChars]
		}
		remainingChars -= len(entry.Content)
		entries = append(entries, entry)
	}
	return entries
}

func memoryEntryFromResult(raw any) MemoryContextEntry {
	switch value := raw.(type) {
	case string:
		return MemoryContextEntry{Content: value}
	case map[string]any:
		return memoryEntryFromMap(value)
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			return MemoryContextEntry{}
		}
		var m map[string]any
		if err := json.Unmarshal(bytes, &m); err != nil {
			return MemoryContextEntry{Content: string(bytes)}
		}
		return memoryEntryFromMap(m)
	}
}

func memoryEntryFromMap(m map[string]any) MemoryContextEntry {
	entry := MemoryContextEntry{
		Content:    firstString(m, "content", "text", "memory", "summary", "fact", "observation"),
		Source:     firstString(m, "source"),
		MemoryType: firstString(m, "memory_type", "type"),
	}
	if entry.Content == "" {
		if nested, ok := m["document"].(map[string]any); ok {
			entry.Content = firstString(nested, "content", "text", "summary")
		}
	}
	if tags, ok := m["tags"].([]any); ok {
		for _, raw := range tags {
			if tag, ok := raw.(string); ok {
				entry.Tags = append(entry.Tags, tag)
			}
		}
	}
	return entry
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
