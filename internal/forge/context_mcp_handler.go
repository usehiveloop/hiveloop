package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

const startForgeToolDescription = `Submit gathered requirements to begin the Forge optimization process. The user must approve before Forge starts.

ALL fields are required. If any field is missing or insufficient, the tool returns an error explaining exactly what's needed — read it carefully and go back to the user.

Requirements:
- requirements_summary: One paragraph. What the agent does, who uses it, core behavior.
- success_criteria: At least 3 specific, testable criteria. "Routes billing tickets to billing team" not "handles tickets well."
- edge_cases: At least 1 situation the agent should handle gracefully (ambiguous input, angry user, unknown answer, multiple issues).
- tone_and_style: How the agent should sound (formal/casual, concise/verbose, personality traits).
- constraints: At least 1 thing the agent must NEVER do.
- example_interactions: At least 2 {user, expected_response} pairs showing ideal agent behavior.
- priority_focus: The single most important quality (accuracy, safety, speed, personality, compliance).

Call this tool only after you have all 7 fields. If the tool returns an error, read the detail field — it tells you exactly what to fix.`

// ForgeContextMCPHandler serves the start_forge tool for forge context-gathering
// conversations. When the context-gatherer agent decides it has enough information,
// it calls start_forge with structured requirements — this handler stores them
// in the ForgeRun record.
//
// Route: /forge-context/{forgeRunID}/*
type ForgeContextMCPHandler struct {
	db *gorm.DB
}

// NewForgeContextMCPHandler creates a new forge context MCP handler.
func NewForgeContextMCPHandler(db *gorm.DB) *ForgeContextMCPHandler {
	return &ForgeContextMCPHandler{db: db}
}

// StreamableHTTPHandler returns an HTTP handler for the MCP Streamable HTTP transport.
func (h *ForgeContextMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(h.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

// serverFactory creates an MCP server with the start_forge tool if the forge
// run is in gathering_context status.
func (h *ForgeContextMCPHandler) serverFactory(r *http.Request) *mcpsdk.Server {
	runID := chi.URLParam(r, "forgeRunID")
	if runID == "" {
		slog.Error("forge context mcp: no forgeRunID in URL")
		return emptyContextServer()
	}

	var run model.ForgeRun
	if err := h.db.Select("id, status").Where("id = ?", runID).First(&run).Error; err != nil {
		slog.Error("forge context mcp: forge run not found", "forge_run_id", runID, "error", err)
		return emptyContextServer()
	}

	if run.Status != model.ForgeStatusGatheringContext {
		slog.Warn("forge context mcp: run not in gathering_context status", "forge_run_id", runID, "status", run.Status)
		return emptyContextServer()
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-context",
		Version: "v1.0.0",
	}, nil)

	server.AddTool(
		&mcpsdk.Tool{
			Name:        "start_forge",
			Description: startForgeToolDescription,
			InputSchema: StartForgeToolSchema(),
		},
		h.buildStartForgeHandler(run.ID.String()),
	)

	return server
}

// buildStartForgeHandler creates the tool handler for start_forge.
// It validates the arguments, stores them as context on the ForgeRun,
// and returns a confirmation message.
func (h *ForgeContextMCPHandler) buildStartForgeHandler(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args ForgeContext
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return contextToolError(
					"INVALID_JSON",
					"Could not parse your tool call arguments. Make sure you're passing valid JSON with all required fields.",
					err.Error(),
				), nil
			}
		}

		// Validate every field — all are required.
		var missing []string
		if args.RequirementsSummary == "" {
			missing = append(missing, "requirements_summary (a concise paragraph describing what the agent does, who uses it, and its core behavior)")
		}
		if len(args.SuccessCriteria) == 0 {
			missing = append(missing, "success_criteria (array of 4-8 specific, testable criteria — e.g. 'Routes billing tickets to billing team')")
		}
		if len(args.EdgeCases) == 0 {
			missing = append(missing, "edge_cases (array of situations the agent should handle gracefully — e.g. 'User mentions multiple issues in one message')")
		}
		if args.ToneAndStyle == "" {
			missing = append(missing, "tone_and_style (how the agent should sound — e.g. 'Friendly but professional. Use the customer's name.')")
		}
		if len(args.Constraints) == 0 {
			missing = append(missing, "constraints (array of things the agent must NEVER do — e.g. 'Never share internal ticket IDs')")
		}
		if len(args.ExampleInteractions) == 0 {
			missing = append(missing, "example_interactions (array of {user, expected_response} pairs showing ideal behavior — at least 2)")
		}
		if args.PriorityFocus == "" {
			missing = append(missing, "priority_focus (the single most important quality: accuracy, safety, speed, or personality)")
		}

		if len(missing) > 0 {
			detail := "You are missing the following required fields:\n"
			for _, field := range missing {
				detail += "  - " + field + "\n"
			}
			detail += "\nAll fields are required. Go back to the user and gather the missing information before calling start_forge again."
			return contextToolError("MISSING_FIELDS", detail, ""), nil
		}

		// Validate quality.
		if len(args.SuccessCriteria) < 3 {
			return contextToolError(
				"INSUFFICIENT_CRITERIA",
				fmt.Sprintf("You provided %d success criteria. We need at least 3 specific, testable criteria to generate meaningful test cases. Ask the user for more detail about what 'success' looks like.", len(args.SuccessCriteria)),
				"",
			), nil
		}
		if len(args.ExampleInteractions) < 2 {
			return contextToolError(
				"INSUFFICIENT_EXAMPLES",
				fmt.Sprintf("You provided %d example interactions. We need at least 2 to understand the desired behavior. Ask the user for a couple of sample conversations showing how the agent should respond.", len(args.ExampleInteractions)),
				"",
			), nil
		}

		contextJSON, err := json.Marshal(args)
		if err != nil {
			return contextToolError("SERIALIZE_ERROR", "Internal error serializing requirements.", err.Error()), nil
		}

		if err := h.db.Model(&model.ForgeRun{}).
			Where("id = ? AND status = ?", runID, model.ForgeStatusGatheringContext).
			Update("context", contextJSON).Error; err != nil {
			slog.Error("forge context mcp: failed to store context", "forge_run_id", runID, "error", err)
			return contextToolError("SAVE_ERROR", "Failed to save requirements. Try calling start_forge again.", ""), nil
		}

		slog.Info("forge context mcp: requirements captured",
			"forge_run_id", runID,
			"criteria_count", len(args.SuccessCriteria),
			"example_count", len(args.ExampleInteractions),
		)

		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: `{"status": "requirements_captured", "message": "All requirements recorded. The user will now review and approve before Forge begins."}`},
			},
		}, nil
	}
}

// contextToolError returns a structured error with a code, human-readable detail,
// and optional debug info. The detail tells the agent exactly what went wrong
// and what to do about it.
func contextToolError(code, detail, debug string) *mcpsdk.CallToolResult {
	msg := fmt.Sprintf(`{"error": "%s", "detail": "%s"`, code, detail)
	if debug != "" {
		msg += fmt.Sprintf(`, "debug": "%s"`, debug)
	}
	msg += "}"
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: msg},
		},
		IsError: true,
	}
}

// emptyContextServer returns an MCP server with no tools.
func emptyContextServer() *mcpsdk.Server {
	return mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-context",
		Version: "v1.0.0",
	}, nil)
}
