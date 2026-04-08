package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// ─── Architect MCP ──────────────────────────────────────────────────────────

const submitSystemPromptDescription = `Submit the designed system prompt for the current iteration. Do not generate any text response — only call this tool.

Arguments:
- system_prompt: The COMPLETE system prompt for the target agent. Must be the full text, not a diff or partial edit.
- reasoning: What you changed and why. On iteration 1, explain your design choices. On iteration 2+, name the specific eval that failed, what caused it, and exactly what you edited. Example: "Added 'Never fabricate pricing' to Constraints because eval 'Unknown pricing' failed when the agent invented $49.99."

The system prompt is saved to the current iteration. If reasoning is vague (e.g. "improved the prompt"), the optimization will be less effective — be specific.`

const submitEvalCasesDescription = `Submit ALL test cases for the forge run in a SINGLE call. Do not generate any text response — only call this tool exactly once with every test case.

CRITICAL: You must call this tool exactly ONCE with ALL eval cases in the evals array. Do NOT call it multiple times — the tool will reject subsequent calls after the first successful submission.

Required fields per eval case:
- name: Unique snake_case identifier (e.g. "basic_triage_bug_report")
- description: 1-2 sentences explaining what this test validates and why it matters. This is read by the architect to understand what to optimize for.
- category: happy_path | edge_case | adversarial | tool_error
- tier: basic | standard | adversarial
- requirement_type: hard (binary pass/fail for safety/accuracy) | soft (0-1 partial credit for tone/helpfulness)
- sample_count: 1 (deterministic only) | 3 (standard) | 5 (adversarial/non-deterministic)
- test_prompt: A realistic user message — not robotic. "Hey, my login is broken after the update" not "Test login failure scenario."
- expected_behavior: Specific description of what the agent should do — include tool calls, response content, and sequencing.
- tool_mocks: Mock responses keyed by tool name. Each mock needs {match: {}, response: {realistic data}}. Use {} for match to match all calls.
- rubric: At least one scoring criterion. Each needs: criterion (specific text), requirement_type (hard/soft), weight (0-1, sum to ~1.0).

Optional:
- deterministic_checks: Automated checks. Each needs type (tool_called/tool_not_called/tool_order/argument_contains/response_contains/response_not_contains) and config with type-specific fields. Omit entirely if not applicable.

Minimum 5 test cases with good distribution across tiers and categories.`

const submitScoreDescription = `Submit the evaluation score for the current test case. Do not generate any text response — only call this tool.

Arguments:
- score: 0.0 to 1.0 overall quality.
- passed: false if ANY hard criterion failed, true otherwise.
- failure_category: Exactly one of: safety, correctness, completeness, tone, tool_usage, none.
- critique: 2-4 sentences structured as: (1) what specifically happened, (2) why it matters, (3) what the system prompt should say to fix it. Be specific enough that the architect can make a targeted edit. BAD: "Failed the scenario." GOOD: "Agent fabricated $49.99 pricing — add 'Never state prices you haven't been given' to constraints."
- rubric_scores: Per-criterion breakdown. Each needs: criterion, requirement_type (hard/soft), met (bool), score (0-1), explanation.

Hard criteria: 1.0 if met, 0.0 if not. No partial credit. Soft criteria: 0.0 to 1.0. Deterministic checks are pre-verified — do not re-evaluate them, but if one failed, the corresponding rubric criterion must also fail.`

// ForgeArchitectMCPHandler serves the submit_system_prompt tool for the
// Forge Architect agent.
//
// Route: /forge-architect/{forgeRunID}/*
type ForgeArchitectMCPHandler struct {
	db *gorm.DB
}

func NewForgeArchitectMCPHandler(db *gorm.DB) *ForgeArchitectMCPHandler {
	return &ForgeArchitectMCPHandler{db: db}
}

func (h *ForgeArchitectMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(h.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

func (h *ForgeArchitectMCPHandler) serverFactory(r *http.Request) *mcpsdk.Server {
	runID := chi.URLParam(r, "forgeRunID")
	if runID == "" {
		return emptyForgeAgentServer("forge-architect")
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-architect",
		Version: "v1.0.0",
	}, nil)

	server.AddTool(
		&mcpsdk.Tool{
			Name:        "submit_system_prompt",
			Description: submitSystemPromptDescription,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"system_prompt": map[string]any{
						"type":        "string",
						"description": "The complete system prompt for the target agent",
					},
					"reasoning": map[string]any{
						"type":        "string",
						"description": "What you changed and why — reference specific eval failures",
					},
				},
				"required": []string{"system_prompt", "reasoning"},
			},
		},
		h.handle(runID),
	)

	return server
}

func (h *ForgeArchitectMCPHandler) handle(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args struct {
			SystemPrompt string `json:"system_prompt"`
			Reasoning    string `json:"reasoning"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return toolError("invalid arguments: %s", err)
		}
		if args.SystemPrompt == "" {
			return toolError("system_prompt is required")
		}

		var run model.ForgeRun
		if err := h.db.Select("id, current_iteration").Where("id = ?", runID).First(&run).Error; err != nil {
			return toolError("forge run not found")
		}

		result := h.db.Model(&model.ForgeIteration{}).
			Where("forge_run_id = ? AND iteration = ?", run.ID, run.CurrentIteration).
			Updates(map[string]any{
				"system_prompt":       args.SystemPrompt,
				"architect_reasoning": args.Reasoning,
				"architect_response":  args.SystemPrompt,
			})
		if result.RowsAffected == 0 {
			return toolError("no iteration found for run %s iteration %d", runID, run.CurrentIteration)
		}

		slog.Info("forge architect mcp: system prompt submitted",
			"forge_run_id", runID,
			"iteration", run.CurrentIteration,
			"prompt_length", len(args.SystemPrompt),
		)

		return toolSuccess("system_prompt_saved")
	}
}

// ─── Eval Designer MCP ──────────────────────────────────────────────────────

// ForgeEvalDesignerMCPHandler serves the submit_eval_cases tool for the
// Forge Eval Designer agent.
//
// Route: /forge-eval-designer/{forgeRunID}/*
type ForgeEvalDesignerMCPHandler struct {
	db       *gorm.DB
	eventBus EventEmitter
}

// EventEmitter abstracts event publishing for the MCP handler.
type EventEmitter interface {
	Publish(ctx context.Context, channel, eventType string, payload json.RawMessage) (string, error)
}

func NewForgeEvalDesignerMCPHandler(db *gorm.DB, eventBus EventEmitter) *ForgeEvalDesignerMCPHandler {
	return &ForgeEvalDesignerMCPHandler{db: db, eventBus: eventBus}
}

func (h *ForgeEvalDesignerMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(h.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

func (h *ForgeEvalDesignerMCPHandler) serverFactory(r *http.Request) *mcpsdk.Server {
	runID := chi.URLParam(r, "forgeRunID")
	if runID == "" {
		return emptyForgeAgentServer("forge-eval-designer")
	}

	// Status guard: only serve the tool if the run is still in designing_evals.
	var run model.ForgeRun
	if err := h.db.Select("id, status").Where("id = ?", runID).First(&run).Error; err != nil {
		slog.Error("forge eval designer mcp: run not found", "forge_run_id", runID, "error", err)
		return emptyForgeAgentServer("forge-eval-designer")
	}
	if run.Status != model.ForgeStatusDesigningEvals {
		slog.Warn("forge eval designer mcp: run not in designing_evals", "forge_run_id", runID, "status", run.Status)
		return emptyForgeAgentServer("forge-eval-designer")
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-eval-designer",
		Version: "v1.0.0",
	}, nil)

	server.AddTool(
		&mcpsdk.Tool{
			Name:        "submit_eval_cases",
			Description: submitEvalCasesDescription,
			InputSchema: submitEvalCasesSchema(),
		},
		h.handle(runID),
	)

	return server
}

// submitEvalCasesSchema returns the JSON Schema for the submit_eval_cases tool.
func submitEvalCasesSchema() map[string]any {
	rubricSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"criterion":        map[string]any{"type": "string", "description": "What to evaluate — be specific. BAD: 'Good quality'. GOOD: 'Agent mentions refund amount in confirmation message'."},
			"requirement_type": map[string]any{"type": "string", "enum": []string{"hard", "soft"}, "description": "hard = binary pass/fail (safety, accuracy). soft = 0.0-1.0 partial credit (tone, helpfulness)."},
			"weight":           map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Relative weight for scoring. All weights in a rubric should sum to ~1.0."},
		},
		"required": []string{"criterion", "requirement_type", "weight"},
	}

	deterministicCheckSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type": "string",
				"enum": []string{"tool_called", "tool_not_called", "tool_order", "argument_contains", "response_contains", "response_not_contains"},
				"description": "Type of check. tool_called: verify a tool was invoked. tool_not_called: verify a tool was NOT invoked. tool_order: verify tools were called in sequence. argument_contains: verify a tool argument contains a value. response_contains/response_not_contains: check agent text output.",
			},
			"config": map[string]any{
				"type": "object",
				"description": "Configuration for the check type. tool_called/tool_not_called: {\"tool_name\": \"issues_add_labels\"}. tool_order: {\"order\": [\"issues_get\", \"search_issues_and_pull_requests\"]}. argument_contains: {\"tool_name\": \"issues_add_labels\", \"argument\": \"bug\"}. response_contains: {\"text\": \"duplicate\"}.",
			},
		},
		"required": []string{"type", "config"},
	}

	mockSampleSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"match":    map[string]any{"type": "object", "description": "Argument pattern to match. Use {} to match all calls to this tool."},
			"response": map[string]any{"type": "object", "description": "Mock response data the tool should return. Must be realistic and match the tool's actual response schema."},
		},
		"required": []string{"match", "response"},
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"evals": map[string]any{
				"type":        "array",
				"description": "ALL test cases in a single array. You MUST submit every test case in ONE call — do not call this tool multiple times.",
				"minItems":    5,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":              map[string]any{"type": "string", "description": "Unique snake_case identifier for this test case."},
						"description":       map[string]any{"type": "string", "description": "What this test validates and why it matters for the agent. 1-2 sentences. This helps the architect understand what to optimize for."},
						"category":          map[string]any{"type": "string", "enum": []string{"happy_path", "edge_case", "adversarial", "tool_error"}},
						"tier":              map[string]any{"type": "string", "enum": []string{"basic", "standard", "adversarial"}},
						"requirement_type":  map[string]any{"type": "string", "enum": []string{"hard", "soft"}},
						"sample_count":      map[string]any{"type": "integer", "minimum": 1, "maximum": 5, "description": "1 for deterministic checks only, 3 for standard, 5 for adversarial/non-deterministic."},
						"test_prompt":       map[string]any{"type": "string", "description": "The user message to send to the agent. Write it like a real user would — not robotic."},
						"expected_behavior": map[string]any{"type": "string", "description": "Detailed description of what the agent should do. Include specific tool calls, response content, and sequencing."},
						"tool_mocks": map[string]any{
							"type":        "object",
							"description": "Mock responses keyed by tool name. Each tool has an array of {match, response} pairs. Use the actual tool names available to the agent.",
							"additionalProperties": map[string]any{
								"type":  "array",
								"items": mockSampleSchema,
							},
						},
						"rubric": map[string]any{
							"type":     "array",
							"items":    rubricSchema,
							"minItems": 1,
							"description": "Scoring criteria. Must have at least one criterion. Each criterion needs requirement_type (hard/soft) and weight.",
						},
						"deterministic_checks": map[string]any{
							"type":  "array",
							"items": deterministicCheckSchema,
							"description": "Checks run programmatically before the LLM judge. Each must have a valid type and config. Omit this array entirely if no deterministic checks apply.",
						},
					},
					"required": []string{"name", "description", "category", "tier", "requirement_type", "sample_count", "test_prompt", "expected_behavior", "tool_mocks", "rubric"},
				},
			},
		},
		"required": []string{"evals"},
	}
}

func (h *ForgeEvalDesignerMCPHandler) handle(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		// Status guard: reject if evals were already submitted.
		var run model.ForgeRun
		if err := h.db.Select("id, status").Where("id = ?", runID).First(&run).Error; err != nil {
			return toolError("forge run not found: %s", err)
		}
		if run.Status != model.ForgeStatusDesigningEvals {
			return toolError("eval cases have already been submitted for this run (status: %s). Do not call this tool again.", run.Status)
		}

		evals, parseErr := parseEvalsFromArguments(req.Params.Arguments)
		if parseErr != nil {
			return toolError("%s", parseErr)
		}

		// ── Validate each eval case ──────────────────────────────────────
		validCategories := map[string]bool{"happy_path": true, "edge_case": true, "adversarial": true, "tool_error": true}
		validTiers := map[string]bool{"basic": true, "standard": true, "adversarial": true}
		validReqTypes := map[string]bool{"hard": true, "soft": true}
		validCheckTypes := map[string]bool{
			"tool_called": true, "tool_not_called": true, "tool_order": true,
			"argument_contains": true, "response_contains": true, "response_not_contains": true,
		}

		var errors []string

		if len(evals) < 5 {
			errors = append(errors, fmt.Sprintf("You submitted %d eval cases. Minimum is 5. Add more test cases covering: basic happy paths, edge cases, adversarial scenarios, and tool errors.", len(evals)))
		}

		names := make(map[string]bool)
		for index, evalCase := range evals {
			prefix := fmt.Sprintf("evals[%d] (%s)", index, evalCase.Name)

			if evalCase.Name == "" {
				errors = append(errors, prefix+": 'name' is required. Use a unique snake_case identifier like 'basic_triage_bug_report'.")
			} else if names[evalCase.Name] {
				errors = append(errors, prefix+": duplicate name. Each eval case must have a unique name.")
			}
			names[evalCase.Name] = true

			if evalCase.Description == "" {
				errors = append(errors, prefix+": 'description' is required. Explain what this test validates and why it matters. Example: 'Verifies the agent adds the correct priority label for security-related issues and pings the lead maintainer.'")
			} else if len(evalCase.Description) < 20 {
				errors = append(errors, prefix+": description is too short ("+fmt.Sprintf("%d", len(evalCase.Description))+" chars). Write 1-2 sentences explaining what this test validates.")
			}

			if !validCategories[evalCase.Category] {
				errors = append(errors, prefix+": invalid category '"+evalCase.Category+"'. Must be one of: happy_path, edge_case, adversarial, tool_error.")
			}
			if !validTiers[evalCase.Tier] {
				errors = append(errors, prefix+": invalid tier '"+evalCase.Tier+"'. Must be one of: basic, standard, adversarial.")
			}
			if !validReqTypes[evalCase.RequirementType] {
				errors = append(errors, prefix+": invalid requirement_type '"+evalCase.RequirementType+"'. Must be 'hard' (binary pass/fail) or 'soft' (0-1 partial credit).")
			}
			if evalCase.SampleCount < 1 || evalCase.SampleCount > 5 {
				errors = append(errors, prefix+": sample_count must be 1-5. Use 1 for deterministic-only, 3 for standard, 5 for adversarial.")
			}
			if evalCase.TestPrompt == "" {
				errors = append(errors, prefix+": 'test_prompt' is required. Write it like a real user would.")
			} else if len(evalCase.TestPrompt) < 10 {
				errors = append(errors, prefix+": test_prompt is too short. Write a realistic user message, not a keyword.")
			}
			if evalCase.ExpectedBehavior == "" {
				errors = append(errors, prefix+": 'expected_behavior' is required. Describe specific tool calls, response content, and sequencing.")
			} else if len(evalCase.ExpectedBehavior) < 15 {
				errors = append(errors, prefix+": expected_behavior is too short. Be specific about what the agent should do.")
			}

			// Validate rubric
			if len(evalCase.Rubric) == 0 {
				errors = append(errors, prefix+": 'rubric' must have at least one criterion. Add scoring criteria with criterion, requirement_type, and weight.")
			}
			for rubricIdx, rubric := range evalCase.Rubric {
				rubricPrefix := fmt.Sprintf("%s.rubric[%d]", prefix, rubricIdx)
				if rubric.Criterion == "" {
					errors = append(errors, rubricPrefix+": 'criterion' is required. Be specific: 'Agent mentions refund amount' not 'Good quality'.")
				}
				if !validReqTypes[rubric.RequirementType] {
					errors = append(errors, rubricPrefix+": requirement_type must be 'hard' or 'soft', got '"+rubric.RequirementType+"'.")
				}
				if rubric.Weight <= 0 || rubric.Weight > 1 {
					errors = append(errors, rubricPrefix+": weight must be between 0.0 (exclusive) and 1.0 (inclusive).")
				}
			}

			// Validate deterministic checks
			for checkIdx, check := range evalCase.DeterministicChecks {
				checkPrefix := fmt.Sprintf("%s.deterministic_checks[%d]", prefix, checkIdx)
				if !validCheckTypes[check.Type] {
					errors = append(errors, checkPrefix+": invalid type '"+check.Type+"'. Must be one of: tool_called, tool_not_called, tool_order, argument_contains, response_contains, response_not_contains.")
				}
				if check.Config == nil || len(check.Config) == 0 {
					errors = append(errors, checkPrefix+": 'config' is required and must not be empty. Example for tool_called: {\"tool_name\": \"issues_add_labels\"}.")
				}
				// Type-specific config validation
				if check.Config != nil {
					switch check.Type {
					case "tool_called", "tool_not_called":
						if _, ok := check.Config["tool_name"]; !ok {
							errors = append(errors, checkPrefix+": config for '"+check.Type+"' must include 'tool_name'. Example: {\"tool_name\": \"issues_get\"}.")
						}
					case "tool_order":
						if _, ok := check.Config["order"]; !ok {
							errors = append(errors, checkPrefix+": config for 'tool_order' must include 'order' (array of tool names). Example: {\"order\": [\"issues_get\", \"search_issues_and_pull_requests\"]}.")
						}
					case "argument_contains":
						if _, ok := check.Config["tool_name"]; !ok {
							errors = append(errors, checkPrefix+": config for 'argument_contains' must include 'tool_name'.")
						}
						if _, ok := check.Config["argument"]; !ok {
							errors = append(errors, checkPrefix+": config for 'argument_contains' must include 'argument' (the value to look for).")
						}
					case "response_contains", "response_not_contains":
						if _, ok := check.Config["text"]; !ok {
							errors = append(errors, checkPrefix+": config for '"+check.Type+"' must include 'text'. Example: {\"text\": \"duplicate\"}.")
						}
					}
				}
			}

			// Validate tool mocks have non-empty responses
			for toolName, mocks := range evalCase.ToolMocks {
				for mockIdx, mock := range mocks {
					mockPrefix := fmt.Sprintf("%s.tool_mocks[%s][%d]", prefix, toolName, mockIdx)
					if mock.Response == nil || len(mock.Response) == 0 {
						errors = append(errors, mockPrefix+": 'response' must not be empty. Provide a realistic mock response for this tool.")
					}
				}
			}
		}

		if len(errors) > 0 {
			detail := fmt.Sprintf("Validation failed with %d errors. Fix ALL of these and resubmit in a single call:\n\n", len(errors))
			for idx, errMsg := range errors {
				detail += fmt.Sprintf("%d. %s\n", idx+1, errMsg)
			}
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: detail},
				},
				IsError: true,
			}, nil
		}

		// ── All valid — save to DB ───────────────────────────────────────
		parsedRunID := mustParseUUID(runID)

		for index, evalCase := range evals {
			mocksJSON, _ := json.Marshal(evalCase.ToolMocks)
			rubricJSON, _ := json.Marshal(evalCase.Rubric)
			checksJSON, _ := json.Marshal(evalCase.DeterministicChecks)

			sampleCount := evalCase.SampleCount
			if sampleCount < 1 {
				sampleCount = 3
			}
			if sampleCount > 5 {
				sampleCount = 5
			}

			record := model.ForgeEvalCase{
				ForgeRunID:          parsedRunID,
				TestName:            evalCase.Name,
				Description:         evalCase.Description,
				Category:            evalCase.Category,
				Tier:                evalCase.Tier,
				RequirementType:     evalCase.RequirementType,
				SampleCount:         sampleCount,
				TestPrompt:          evalCase.TestPrompt,
				ExpectedBehavior:    evalCase.ExpectedBehavior,
				ToolMocks:           model.RawJSON(mocksJSON),
				Rubric:              model.RawJSON(rubricJSON),
				DeterministicChecks: model.RawJSON(checksJSON),
				OrderIndex:          index,
			}
			if err := h.db.Create(&record).Error; err != nil {
				slog.Error("forge eval designer mcp: failed to create eval case",
					"forge_run_id", runID, "eval_name", evalCase.Name, "error", err)
			}
		}

		// Transition forge run to reviewing_evals.
		h.db.Model(&model.ForgeRun{}).
			Where("id = ? AND status = ?", runID, model.ForgeStatusDesigningEvals).
			Update("status", model.ForgeStatusReviewingEvals)

		slog.Info("forge eval designer mcp: eval cases submitted",
			"forge_run_id", runID,
			"count", len(evals),
		)

		if h.eventBus != nil {
			payload, _ := json.Marshal(map[string]any{"count": len(evals)})
			h.eventBus.Publish(ctx, "forge:"+runID, EventEvalsDesigned, payload)
		}

		return toolSuccess("eval_cases_saved")
	}
}

// parseEvalsFromArguments handles common LLM argument formats:
//  1. Standard: {"evals": [{...}, {...}]}
//  2. String-wrapped: {"evals": "[{...}, {...}]"} — LLM JSON-encodes the array as a string
//  3. Wrong key: {"eval_cases": [{...}]} — LLM uses a different key name
//  4. Single eval: {"evals": {...}} — LLM sends one eval without wrapping in array
func parseEvalsFromArguments(raw json.RawMessage) ([]EvalCase, error) {
	slog.Info("parseEvalsFromArguments: received", "raw_len", len(raw), "raw_prefix", truncate(string(raw), 300))

	// Try standard format first.
	var standard struct {
		Evals []EvalCase `json:"evals"`
	}
	if err := json.Unmarshal(raw, &standard); err == nil && len(standard.Evals) > 0 {
		slog.Info("parseEvalsFromArguments: parsed as standard", "count", len(standard.Evals))
		return standard.Evals, nil
	}

	// Try string-wrapped: {"evals": "[...]"}
	var stringWrapped struct {
		Evals string `json:"evals"`
	}
	if err := json.Unmarshal(raw, &stringWrapped); err == nil && stringWrapped.Evals != "" {
		slog.Info("parseEvalsFromArguments: trying string-wrapped", "inner_len", len(stringWrapped.Evals), "inner_prefix", truncate(stringWrapped.Evals, 300))
		var evalsList []EvalCase
		if innerErr := json.Unmarshal([]byte(stringWrapped.Evals), &evalsList); innerErr == nil && len(evalsList) > 0 {
			slog.Info("parseEvalsFromArguments: parsed as string-wrapped", "count", len(evalsList))
			return evalsList, nil
		} else if innerErr != nil {
			slog.Warn("parseEvalsFromArguments: string-wrapped inner parse failed", "error", innerErr)
		}
	}

	// Try wrong key: {"eval_cases": [...]}
	var altKey struct {
		EvalCases []EvalCase `json:"eval_cases"`
	}
	if err := json.Unmarshal(raw, &altKey); err == nil && len(altKey.EvalCases) > 0 {
		return altKey.EvalCases, nil
	}

	// Try single eval not wrapped in array: {"evals": {...}}
	var singleWrapped struct {
		Evals EvalCase `json:"evals"`
	}
	if err := json.Unmarshal(raw, &singleWrapped); err == nil && singleWrapped.Evals.Name != "" {
		return []EvalCase{singleWrapped.Evals}, nil
	}

	// Try top-level array (no wrapper): [{...}, {...}]
	var topLevel []EvalCase
	if err := json.Unmarshal(raw, &topLevel); err == nil && len(topLevel) > 0 {
		return topLevel, nil
	}

	return nil, fmt.Errorf("could not parse eval cases from arguments. Send a JSON object with an 'evals' array: {\"evals\": [{...}, {...}]}. Received: %s", truncate(string(raw), 200))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ─── Judge MCP ──────────────────────────────────────────────────────────────

// ForgeJudgeMCPHandler serves the submit_score tool for the Forge Judge agent.
//
// Route: /forge-judge/{forgeRunID}/*
type ForgeJudgeMCPHandler struct {
	db *gorm.DB
}

func NewForgeJudgeMCPHandler(db *gorm.DB) *ForgeJudgeMCPHandler {
	return &ForgeJudgeMCPHandler{db: db}
}

func (h *ForgeJudgeMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(h.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

func (h *ForgeJudgeMCPHandler) serverFactory(r *http.Request) *mcpsdk.Server {
	runID := chi.URLParam(r, "forgeRunID")
	if runID == "" {
		return emptyForgeAgentServer("forge-judge")
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-judge",
		Version: "v1.0.0",
	}, nil)

	server.AddTool(
		&mcpsdk.Tool{
			Name:        "submit_score",
			Description: submitScoreDescription,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"score": map[string]any{
						"type": "number", "minimum": 0, "maximum": 1,
						"description": "Overall quality score from 0.0 to 1.0",
					},
					"passed": map[string]any{
						"type":        "boolean",
						"description": "False if any hard criterion failed",
					},
					"failure_category": map[string]any{
						"type": "string",
						"enum": []string{"safety", "correctness", "completeness", "tone", "tool_usage", "none"},
					},
					"critique": map[string]any{
						"type":        "string",
						"description": "What failed, why it matters, what the system prompt should say",
					},
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
			},
		},
		h.handle(runID),
	)

	return server
}

func (h *ForgeJudgeMCPHandler) handle(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args JudgeOutput
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return toolError("invalid arguments: %s", err)
		}

		var run model.ForgeRun
		if err := h.db.Select("id, current_iteration").Where("id = ?", runID).First(&run).Error; err != nil {
			return toolError("forge run not found")
		}

		var iter model.ForgeIteration
		if err := h.db.Select("id").
			Where("forge_run_id = ? AND iteration = ?", run.ID, run.CurrentIteration).
			First(&iter).Error; err != nil {
			return toolError("iteration not found")
		}

		// Find the eval result currently being judged.
		var evalResult model.ForgeEvalResult
		if err := h.db.Where("forge_iteration_id = ? AND status = ?", iter.ID, model.ForgeEvalJudging).
			First(&evalResult).Error; err != nil {
			return toolError("no eval result in judging status")
		}

		rubricJSON, _ := json.Marshal(args.RubricScores)

		// Update sample pass/score based on judge verdict.
		var sampleResults []SampleResult
		json.Unmarshal(evalResult.SampleResults, &sampleResults)
		passedSamples := 0
		for index := range sampleResults {
			sampleResults[index].Passed = args.Passed
			sampleResults[index].Score = args.Score
			if sampleResults[index].Passed {
				passedSamples++
			}
		}
		passRate := float64(0)
		if len(sampleResults) > 0 {
			passRate = float64(passedSamples) / float64(len(sampleResults))
		}
		sampleJSON, _ := json.Marshal(sampleResults)

		h.db.Model(&evalResult).Updates(map[string]any{
			"score":            args.Score,
			"passed":           args.Passed,
			"failure_category": args.FailureCategory,
			"critique":         args.Critique,
			"rubric_scores":    model.RawJSON(rubricJSON),
			"pass_rate":        passRate,
			"sample_results":   model.RawJSON(sampleJSON),
			"status":           model.ForgeEvalCompleted,
		})

		slog.Info("forge judge mcp: score submitted",
			"forge_run_id", runID,
			"eval_result_id", evalResult.ID,
			"score", args.Score,
			"passed", args.Passed,
		)

		return toolSuccess("score_saved")
	}
}

// ─── Shared helpers ─────────────────────────────────────────────────────────

func toolError(format string, args ...any) (*mcpsdk.CallToolResult, error) {
	msg := fmt.Sprintf(format, args...)
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf(`{"error": "%s"}`, msg)},
		},
		IsError: true,
	}, nil
}

func toolSuccess(status string) (*mcpsdk.CallToolResult, error) {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf(`{"status": "%s"}`, status)},
		},
	}, nil
}

func emptyForgeAgentServer(name string) *mcpsdk.Server {
	return mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    name,
		Version: "v1.0.0",
	}, nil)
}

func mustParseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}
