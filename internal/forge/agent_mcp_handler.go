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

const submitEvalCasesDescription = `Submit generated test cases for the forge run. Do not generate any text response — only call this tool.

Arguments:
- evals: Array of test cases. Each must have: name, category (happy_path/edge_case/adversarial/tool_error), tier (basic/standard/adversarial), requirement_type (hard/soft), sample_count (1-5), test_prompt (the exact user message), expected_behavior (what the agent should do).

Optional per eval: tool_mocks (mock responses keyed by tool name), rubric (scoring criteria array), deterministic_checks (automated checks like tool_called, response_contains).

Generate at least 5 test cases with good tier/category distribution. Test prompts should sound like real users, not robotic scripts. Expected behavior should be specific and testable.`

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
	db *gorm.DB
}

func NewForgeEvalDesignerMCPHandler(db *gorm.DB) *ForgeEvalDesignerMCPHandler {
	return &ForgeEvalDesignerMCPHandler{db: db}
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

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-eval-designer",
		Version: "v1.0.0",
	}, nil)

	server.AddTool(
		&mcpsdk.Tool{
			Name:        "submit_eval_cases",
			Description: submitEvalCasesDescription,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"evals": map[string]any{
						"type":        "array",
						"description": "Array of test cases",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name":                 map[string]any{"type": "string"},
								"category":             map[string]any{"type": "string", "enum": []string{"happy_path", "edge_case", "adversarial", "tool_error"}},
								"tier":                 map[string]any{"type": "string", "enum": []string{"basic", "standard", "adversarial"}},
								"requirement_type":     map[string]any{"type": "string", "enum": []string{"hard", "soft"}},
								"sample_count":         map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
								"test_prompt":          map[string]any{"type": "string"},
								"expected_behavior":    map[string]any{"type": "string"},
								"tool_mocks":           map[string]any{"type": "object"},
								"rubric":               map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
								"deterministic_checks": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
							},
							"required": []string{"name", "category", "tier", "requirement_type", "sample_count", "test_prompt", "expected_behavior"},
						},
					},
				},
				"required": []string{"evals"},
			},
		},
		h.handle(runID),
	)

	return server
}

func (h *ForgeEvalDesignerMCPHandler) handle(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args struct {
			Evals []EvalCase `json:"evals"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return toolError("invalid arguments: %s", err)
		}
		if len(args.Evals) == 0 {
			return toolError("at least one eval case is required")
		}

		parsedRunID := mustParseUUID(runID)

		for index, evalCase := range args.Evals {
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

		slog.Info("forge eval designer mcp: eval cases submitted",
			"forge_run_id", runID,
			"count", len(args.Evals),
		)

		return toolSuccess("eval_cases_saved")
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
