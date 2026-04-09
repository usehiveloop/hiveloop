package forge

import (
	"encoding/json"
	"strings"
)

// ArchitectOutput is the fixed schema returned by the architect agent.
// It defines the target agent's system prompt, tools, and configuration.
type ArchitectOutput struct {
	SystemPrompt string           `json:"system_prompt"`
	Tools        []ToolDefinition `json:"tools"`
	AgentConfig  map[string]any   `json:"agent_config"`
	Reasoning    string           `json:"reasoning"`
}

// ToolDefinition describes a single tool for the target agent.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// EvalDesignerOutput is the fixed schema returned by the eval designer agent.
type EvalDesignerOutput struct {
	Evals []EvalCase `json:"evals"`
}

// EvalCase is a single test case with tiered difficulty, requirement type,
// deterministic checks, tool mocks, and structured scoring rubric.
type EvalCase struct {
	Name                string               `json:"name"`
	Description         string               `json:"description"`          // what this test validates and why it matters
	Category            string               `json:"category"`             // happy_path, edge_case, adversarial, tool_error
	Tier                string               `json:"tier"`                 // basic, standard, adversarial
	RequirementType     string               `json:"requirement_type"`     // hard, soft
	SampleCount         int                  `json:"sample_count"`         // 1-5, default 3
	TestPrompt          string               `json:"test_prompt"`
	ExpectedBehavior    string               `json:"expected_behavior"`
	ToolMocks           map[string][]MockSample `json:"tool_mocks"`
	Rubric              []RubricCriterion     `json:"rubric"`
	DeterministicChecks []DeterministicCheck  `json:"deterministic_checks"`
}

// RubricCriterion is a structured scoring criterion with hard/soft classification.
type RubricCriterion struct {
	Criterion       string  `json:"criterion"`
	RequirementType string  `json:"requirement_type"` // "hard" or "soft"
	Weight          float64 `json:"weight"`           // 0.0-1.0, for weighted soft scoring
}

// DeterministicCheck defines a check that can be verified without an LLM judge.
type DeterministicCheck struct {
	Type   string         `json:"type"`   // tool_called, tool_not_called, tool_order, argument_contains, response_contains, response_not_contains
	Config map[string]any `json:"config"` // type-specific configuration
}

// MockSample defines a mock response for a tool call with optional argument matching.
type MockSample struct {
	Match    map[string]any `json:"match"`    // argument pattern to match (empty = match all)
	Response map[string]any `json:"response"` // mock response to return
}

// JudgeOutput is the fixed schema returned by the judge agent for each eval.
// Includes structured failure categories and actionable critiques.
type JudgeOutput struct {
	Score           float64       `json:"score"`            // 0.0-1.0
	Passed          bool          `json:"passed"`
	FailureCategory string        `json:"failure_category"` // safety, correctness, completeness, tone, tool_usage, none
	Critique        string        `json:"critique"`         // actionable, specific — what went wrong and what instruction would fix it
	RubricScores    []RubricScore `json:"rubric_scores"`
}

// RubricScore is a per-criterion scoring result from the judge.
type RubricScore struct {
	Criterion       string  `json:"criterion"`
	RequirementType string  `json:"requirement_type"` // hard, soft
	Met             bool    `json:"met"`
	Score           float64 `json:"score"`       // 0.0-1.0 for soft criteria (1.0 or 0.0 for hard)
	Explanation     string  `json:"explanation"`
}

// ForgeContext holds the structured requirements gathered from the user
// during the context-gathering conversation before forge begins.
// All fields are required — the context gatherer must fill every field.
type ForgeContext struct {
	RequirementsSummary string               `json:"requirements_summary"`
	SuccessCriteria     []string             `json:"success_criteria"`
	EdgeCases           []string             `json:"edge_cases"`
	ToneAndStyle        string               `json:"tone_and_style"`
	Constraints         []string             `json:"constraints"`
	ExampleInteractions []ExampleInteraction  `json:"example_interactions"`
	PriorityFocus       string               `json:"priority_focus"`
}

// ExampleInteraction is a sample user↔agent exchange provided during context gathering.
type ExampleInteraction struct {
	User             string `json:"user"`
	ExpectedResponse string `json:"expected_response"`
}

// StartForgeToolSchema returns the JSON Schema for the start_forge MCP tool.
func StartForgeToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"requirements_summary": map[string]any{
				"type":        "string",
				"description": "Concise summary of what the agent should do",
			},
			"success_criteria": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Specific, testable success criteria",
			},
			"edge_cases": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Edge cases the agent should handle",
			},
			"tone_and_style": map[string]any{
				"type":        "string",
				"description": "Desired tone, voice, and communication style",
			},
			"constraints": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Things the agent must NOT do",
			},
			"example_interactions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"user":              map[string]any{"type": "string"},
						"expected_response": map[string]any{"type": "string"},
					},
					"required": []string{"user", "expected_response"},
				},
				"description": "Example user messages and expected agent responses",
			},
			"priority_focus": map[string]any{
				"type":        "string",
				"description": "What matters most: accuracy, safety, speed, personality, etc.",
			},
		},
		"required": []string{"requirements_summary", "success_criteria", "edge_cases", "tone_and_style", "constraints", "example_interactions", "priority_focus"},
	}
}

// JSON Schema definitions for Bridge's AgentConfig.JsonSchema.

// ArchitectSchema returns the JSON Schema for ArchitectOutput.
func ArchitectSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"system_prompt": map[string]any{"type": "string"},
			"tools": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"parameters":  map[string]any{"type": "object"},
					},
					"required": []string{"name", "description", "parameters"},
				},
			},
			"agent_config": map[string]any{"type": "object"},
			"reasoning":    map[string]any{"type": "string"},
		},
		"required": []string{"system_prompt", "tools", "agent_config", "reasoning"},
	}
}

// EvalDesignerSchema returns the JSON Schema for EvalDesignerOutput.
func EvalDesignerSchema() map[string]any {
	rubricCriterionSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"criterion":        map[string]any{"type": "string"},
			"requirement_type": map[string]any{"type": "string", "enum": []string{"hard", "soft"}},
			"weight":           map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		},
		"required": []string{"criterion", "requirement_type", "weight"},
	}

	deterministicCheckSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type":   map[string]any{"type": "string", "enum": []string{"tool_called", "tool_not_called", "tool_order", "argument_contains", "response_contains", "response_not_contains"}},
			"config": map[string]any{"type": "object"},
		},
		"required": []string{"type", "config"},
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"evals": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":              map[string]any{"type": "string"},
						"category":          map[string]any{"type": "string", "enum": []string{"happy_path", "edge_case", "adversarial", "tool_error"}},
						"tier":              map[string]any{"type": "string", "enum": []string{"basic", "standard", "adversarial"}},
						"requirement_type":  map[string]any{"type": "string", "enum": []string{"hard", "soft"}},
						"sample_count":      map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
						"test_prompt":       map[string]any{"type": "string"},
						"expected_behavior": map[string]any{"type": "string"},
						"tool_mocks": map[string]any{
							"type": "object",
							"additionalProperties": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"match":    map[string]any{"type": "object"},
										"response": map[string]any{"type": "object"},
									},
									"required": []string{"match", "response"},
								},
							},
						},
						"rubric":               map[string]any{"type": "array", "items": rubricCriterionSchema},
						"deterministic_checks": map[string]any{"type": "array", "items": deterministicCheckSchema},
					},
					"required": []string{"name", "category", "tier", "requirement_type", "sample_count", "test_prompt", "expected_behavior", "tool_mocks", "rubric", "deterministic_checks"},
				},
			},
		},
		"required": []string{"evals"},
	}
}

// JudgeSchema returns the JSON Schema for JudgeOutput.
func JudgeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"score":            map[string]any{"type": "number", "minimum": 0, "maximum": 1},
			"passed":           map[string]any{"type": "boolean"},
			"failure_category": map[string]any{"type": "string", "enum": []string{"safety", "correctness", "completeness", "tone", "tool_usage", "none"}},
			"critique":         map[string]any{"type": "string"},
			"rubric_scores": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"criterion":        map[string]any{"type": "string"},
						"requirement_type": map[string]any{"type": "string", "enum": []string{"hard", "soft"}},
						"met":              map[string]any{"type": "boolean"},
						"score":            map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"explanation":      map[string]any{"type": "string"},
					},
					"required": []string{"criterion", "requirement_type", "met", "score", "explanation"},
				},
			},
		},
		"required": []string{"score", "passed", "failure_category", "critique", "rubric_scores"},
	}
}

// ParseArchitectOutput parses a JSON string into ArchitectOutput.
func ParseArchitectOutput(data string) (*ArchitectOutput, error) {
	var out ArchitectOutput
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ParseEvalDesignerOutput parses a JSON string into EvalDesignerOutput.
func ParseEvalDesignerOutput(data string) (*EvalDesignerOutput, error) {
	var out EvalDesignerOutput
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ParseJudgeOutput parses a JSON string into JudgeOutput.
// Strips markdown code fences if present (LLMs often wrap JSON in ```json ... ```).
func ParseJudgeOutput(data string) (*JudgeOutput, error) {
	cleaned := strings.TrimSpace(data)
	if strings.HasPrefix(cleaned, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}
	var out JudgeOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
