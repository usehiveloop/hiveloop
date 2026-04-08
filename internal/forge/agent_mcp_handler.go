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

const submitEvalCaseDescription = `Submit a single test case for the eval suite. Call this tool once per test case.

## When to Use
Call this tool for each test case you design. Submit test cases one at a time — each call saves one test case. After submitting all test cases, call finalize_evals.

## Required Fields
- name: Unique snake_case identifier (e.g. "basic_triage_bug_report")
- description: 1-2 sentences explaining what this test validates and why it matters. Read by the architect to understand optimization targets.
- category: One of happy_path, edge_case, adversarial, tool_error
- tier: One of basic (fundamental), standard (real-world), adversarial (designed to break)
- requirement_type: hard (binary pass/fail for safety/accuracy) or soft (0.0-1.0 partial credit)
- sample_count: 1 for deterministic-only, 3 for standard (default), 5 for adversarial
- test_prompt: The user message to send to the agent. Write it like a real user — not robotic.
- expected_behavior: What the agent should do. Be specific — include tool calls, response content, and sequencing.
- rubric: Array of scoring criteria. Each needs criterion (specific text), requirement_type (hard/soft), weight (0.0-1.0, sum to ~1.0).

## Optional Fields
- tool_mocks: Mock responses keyed by tool name. Each tool has an array of {match, response} pairs. Use {} for match to match all calls.
- deterministic_checks: Automated checks. Each needs type (tool_called/tool_not_called/tool_order/argument_contains/response_contains/response_not_contains) and config.

## Tips
- Submit test cases in order: basic first, then standard, then adversarial
- Each call returns a confirmation with the eval name and index
- If a call returns an error, fix the issue and resubmit that single test case`

const finalizeEvalsDescription = `Finalize the eval suite after submitting all test cases with submit_eval_case.

## When to Use
Call this once after you have submitted all your test cases. This transitions the forge run to review status.

## Requirements
- At least 10 test cases must have been submitted via submit_eval_case
- The forge run must still be in designing_evals status

## What Happens
- Validates the minimum test case count
- Transitions the forge run to reviewing_evals status
- The user will review the test cases before optimization begins`

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
			Name:        "submit_eval_case",
			Description: submitEvalCaseDescription,
			InputSchema: submitEvalCaseSchema(),
		},
		h.handleSubmitOne(runID),
	)

	server.AddTool(
		&mcpsdk.Tool{
			Name:        "finalize_evals",
			Description: finalizeEvalsDescription,
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		h.handleFinalize(runID),
	)

	return server
}

// submitEvalCaseSchema returns the JSON Schema for the submit_eval_case tool (singular).
func submitEvalCaseSchema() map[string]any {
	rubricSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"criterion":        map[string]any{"type": "string"},
			"requirement_type": map[string]any{"type": "string", "enum": []string{"hard", "soft"}},
			"weight":           map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0},
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

	mockSampleSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"match":    map[string]any{"type": "object"},
			"response": map[string]any{"type": "object"},
		},
		"required": []string{"match", "response"},
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":              map[string]any{"type": "string"},
			"description":       map[string]any{"type": "string"},
			"category":          map[string]any{"type": "string", "enum": []string{"happy_path", "edge_case", "adversarial", "tool_error"}},
			"tier":              map[string]any{"type": "string", "enum": []string{"basic", "standard", "adversarial"}},
			"requirement_type":  map[string]any{"type": "string", "enum": []string{"hard", "soft"}},
			"sample_count":      map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
			"test_prompt":       map[string]any{"type": "string"},
			"expected_behavior": map[string]any{"type": "string"},
			"tool_mocks": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type":  "array",
					"items": mockSampleSchema,
				},
			},
			"rubric": map[string]any{
				"type":     "array",
				"items":    rubricSchema,
				"minItems": 1,
			},
			"deterministic_checks": map[string]any{
				"type":  "array",
				"items": deterministicCheckSchema,
			},
		},
		"required": []string{"name", "description", "category", "tier", "requirement_type", "sample_count", "test_prompt", "expected_behavior", "rubric"},
	}
}

// handleSubmitOne saves a single eval case to the DB.
func (h *ForgeEvalDesignerMCPHandler) handleSubmitOne(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var run model.ForgeRun
		if err := h.db.Select("id, status").Where("id = ?", runID).First(&run).Error; err != nil {
			return toolError("forge run not found: %s", err)
		}
		if run.Status != model.ForgeStatusDesigningEvals {
			return toolError("evals already finalized (status: %s)", run.Status)
		}

		var evalCase EvalCase
		if err := json.Unmarshal(req.Params.Arguments, &evalCase); err != nil {
			return toolError("invalid arguments: %s", err)
		}

		// ── Strict validation ────────────────────────────────────────────
		validCategories := map[string]bool{"happy_path": true, "edge_case": true, "adversarial": true, "tool_error": true}
		validTiers := map[string]bool{"basic": true, "standard": true, "adversarial": true}
		validReqTypes := map[string]bool{"hard": true, "soft": true}
		validCheckTypes := map[string]bool{
			"tool_called": true, "tool_not_called": true, "tool_order": true,
			"argument_contains": true, "response_contains": true, "response_not_contains": true,
		}

		var errors []string

		if evalCase.Name == "" {
			errors = append(errors, "'name' is required. Use a unique snake_case identifier.")
		}
		if evalCase.Description == "" {
			errors = append(errors, "'description' is required. Explain what this test validates and why.")
		} else if len(evalCase.Description) < 20 {
			errors = append(errors, fmt.Sprintf("description is too short (%d chars). Write 1-2 sentences.", len(evalCase.Description)))
		}
		if !validCategories[evalCase.Category] {
			errors = append(errors, fmt.Sprintf("invalid category '%s'. Must be: happy_path, edge_case, adversarial, tool_error.", evalCase.Category))
		}
		if !validTiers[evalCase.Tier] {
			errors = append(errors, fmt.Sprintf("invalid tier '%s'. Must be: basic, standard, adversarial.", evalCase.Tier))
		}
		if !validReqTypes[evalCase.RequirementType] {
			errors = append(errors, fmt.Sprintf("invalid requirement_type '%s'. Must be: hard, soft.", evalCase.RequirementType))
		}
		if evalCase.SampleCount < 1 || evalCase.SampleCount > 5 {
			errors = append(errors, "sample_count must be 1-5.")
		}
		if evalCase.TestPrompt == "" {
			errors = append(errors, "'test_prompt' is required.")
		} else if len(evalCase.TestPrompt) < 10 {
			errors = append(errors, "test_prompt is too short. Write a realistic user message.")
		}
		if evalCase.ExpectedBehavior == "" {
			errors = append(errors, "'expected_behavior' is required.")
		} else if len(evalCase.ExpectedBehavior) < 15 {
			errors = append(errors, "expected_behavior is too short. Include tool calls and response details.")
		}
		if len(evalCase.Rubric) == 0 {
			errors = append(errors, "'rubric' must have at least one criterion.")
		}
		for idx, rubric := range evalCase.Rubric {
			if rubric.Criterion == "" {
				errors = append(errors, fmt.Sprintf("rubric[%d]: 'criterion' is required.", idx))
			}
			if !validReqTypes[rubric.RequirementType] {
				errors = append(errors, fmt.Sprintf("rubric[%d]: requirement_type must be 'hard' or 'soft', got '%s'.", idx, rubric.RequirementType))
			}
			if rubric.Weight <= 0 || rubric.Weight > 1 {
				errors = append(errors, fmt.Sprintf("rubric[%d]: weight must be between 0.0 and 1.0.", idx))
			}
		}
		for idx, check := range evalCase.DeterministicChecks {
			if !validCheckTypes[check.Type] {
				errors = append(errors, fmt.Sprintf("deterministic_checks[%d]: invalid type '%s'.", idx, check.Type))
			}
			if len(check.Config) == 0 {
				errors = append(errors, fmt.Sprintf("deterministic_checks[%d]: 'config' must not be empty.", idx))
			}
		}
		for toolName, mocks := range evalCase.ToolMocks {
			for idx, mock := range mocks {
				if len(mock.Response) == 0 {
					errors = append(errors, fmt.Sprintf("tool_mocks[%s][%d]: 'response' must not be empty.", toolName, idx))
				}
			}
		}

		if len(errors) > 0 {
			detail := fmt.Sprintf("Validation failed for eval '%s' with %d errors:\n", evalCase.Name, len(errors))
			for idx, errMsg := range errors {
				detail += fmt.Sprintf("  %d. %s\n", idx+1, errMsg)
			}
			detail += "\nFix these errors and resubmit this eval case."
			return toolError("%s", detail)
		}

		// Check for duplicate name
		var existingCount int64
		h.db.Model(&model.ForgeEvalCase{}).Where("forge_run_id = ? AND test_name = ?", runID, evalCase.Name).Count(&existingCount)
		if existingCount > 0 {
			return toolError("eval case with name '%s' already exists. Use a unique name.", evalCase.Name)
		}

		// Count existing evals to set order_index
		var count int64
		h.db.Model(&model.ForgeEvalCase{}).Where("forge_run_id = ?", runID).Count(&count)

		parsedRunID := mustParseUUID(runID)
		sampleCount := evalCase.SampleCount
		if sampleCount < 1 {
			sampleCount = 3
		}
		if sampleCount > 5 {
			sampleCount = 5
		}

		mocksJSON, _ := json.Marshal(evalCase.ToolMocks)
		rubricJSON, _ := json.Marshal(evalCase.Rubric)
		checksJSON, _ := json.Marshal(evalCase.DeterministicChecks)

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
			OrderIndex:          int(count),
		}
		if err := h.db.Create(&record).Error; err != nil {
			return toolError("failed to save eval case: %s", err)
		}

		slog.Info("forge eval designer mcp: eval case saved",
			"forge_run_id", runID,
			"eval_name", evalCase.Name,
			"order_index", count,
		)

		return toolSuccess(fmt.Sprintf("eval_case_saved: %s (index %d)", evalCase.Name, count))
	}
}

// handleFinalize transitions the forge run from designing_evals to reviewing_evals.
func (h *ForgeEvalDesignerMCPHandler) handleFinalize(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var run model.ForgeRun
		if err := h.db.Select("id, status").Where("id = ?", runID).First(&run).Error; err != nil {
			return toolError("forge run not found: %s", err)
		}
		if run.Status != model.ForgeStatusDesigningEvals {
			return toolError("evals already finalized (status: %s)", run.Status)
		}

		var count int64
		h.db.Model(&model.ForgeEvalCase{}).Where("forge_run_id = ?", runID).Count(&count)
		if count < 10 {
			return toolError("only %d eval cases submitted. Submit at least 10 before finalizing. Include evals for runtime tool usage (memory, journal, todo, subagents) if not already included.", count)
		}

		h.db.Model(&model.ForgeRun{}).
			Where("id = ? AND status = ?", runID, model.ForgeStatusDesigningEvals).
			Update("status", model.ForgeStatusReviewingEvals)

		slog.Info("forge eval designer mcp: evals finalized",
			"forge_run_id", runID,
			"count", count,
		)

		if h.eventBus != nil {
			payload, _ := json.Marshal(map[string]any{"count": count})
			h.eventBus.Publish(ctx, "forge:"+runID, EventEvalsDesigned, payload)
		}

		return toolSuccess(fmt.Sprintf("finalized: %d eval cases ready for review", count))
	}
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
