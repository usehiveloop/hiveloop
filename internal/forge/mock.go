package forge

import "encoding/json"

// findBestMock picks the mock sample whose match pattern best fits the given arguments.
// Priority: exact match on all specified keys > partial match > first sample (wildcard).
// Used by the Forge MCP handler to select which mock response to return.
func findBestMock(args map[string]any, samples []MockSample) MockSample {
	var bestSample MockSample
	bestScore := -1

	for _, s := range samples {
		if len(s.Match) == 0 {
			// Wildcard match — use as fallback if no better match found.
			if bestScore < 0 {
				bestSample = s
				bestScore = 0
			}
			continue
		}

		score := matchScore(args, s.Match)
		if score > bestScore {
			bestScore = score
			bestSample = s
		}
	}

	// If no match at all, use the first sample.
	if bestScore < 0 && len(samples) > 0 {
		return samples[0]
	}
	return bestSample
}

// matchScore returns how many match keys are present and equal in args.
// Returns -1 if any specified key doesn't match.
func matchScore(args map[string]any, match map[string]any) int {
	score := 0
	for k, expected := range match {
		actual, ok := args[k]
		if !ok {
			return -1 // required key missing
		}
		// Compare via JSON serialization for deep equality.
		expectedJSON, _ := json.Marshal(expected)
		actualJSON, _ := json.Marshal(actual)
		if string(expectedJSON) != string(actualJSON) {
			return -1 // value mismatch
		}
		score++
	}
	return score
}
