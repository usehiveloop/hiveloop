package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	bridgepkg "github.com/ziraloop/ziraloop/internal/bridge"
	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/streaming"
	systemagents "github.com/ziraloop/ziraloop/internal/system-agents"
	"github.com/ziraloop/ziraloop/internal/token"
)

const (
	forgeTokenTTL = 24 * time.Hour
)

// providerTypeMap maps provider IDs to Bridge ProviderType values.
var providerTypeMap = map[string]bridgepkg.ProviderType{
	"openai":    bridgepkg.OpenAi,
	"anthropic": bridgepkg.Anthropic,
	"google":    bridgepkg.Google,
	"cohere":    bridgepkg.Cohere,
	"groq":      bridgepkg.Groq,
	"deepseek":  bridgepkg.DeepSeek,
	"mistral":   bridgepkg.Mistral,
	"fireworks": bridgepkg.Fireworks,
	"together":  bridgepkg.Together,
	"xai":       bridgepkg.XAi,
	"ollama":    bridgepkg.Ollama,
}

// SampleResult captures a single sample execution within an eval case.
type SampleResult struct {
	SampleIndex int            `json:"sample_index"`
	Response    string         `json:"response"`
	ToolCalls   []ToolCallInfo `json:"tool_calls,omitempty"`
	Passed      bool           `json:"passed"`
	Score       float64        `json:"score"`
}

// EvalScoreEntry records per-eval score history across iterations.
type EvalScoreEntry struct {
	EvalName string    `json:"eval_name"`
	Scores   []float64 `json:"scores"`
}

// ForgeOrchestrator abstracts sandbox operations so tests can inject mocks.
type ForgeOrchestrator interface {
	GetBridgeClient(ctx context.Context, sb *model.Sandbox) (*bridgepkg.BridgeClient, error)
	WakeSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error)
}

// ForgePusher abstracts agent push operations so tests can inject mocks.
type ForgePusher interface {
	PushAgent(ctx context.Context, agent *model.Agent) error
	PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error
}

// ForgeController orchestrates forge runs — a persistent state machine
// that manages the design→eval→judge iteration loop.
// Uses system agents (seeded to DB) for architect, eval-designer, and judge roles.
type ForgeController struct {
	db           *gorm.DB
	orchestrator ForgeOrchestrator
	pusher       ForgePusher
	catalog      *catalog.Catalog
	signingKey   []byte
	cfg          *config.Config
	eventBus     *streaming.EventBus
	reader       *BridgeReader
	events       *eventEmitter
	inspector    *asynq.Inspector // for cancelling tasks
}

func defaultAgentConfig() *bridgepkg.AgentConfig {
	maxTokens := int32(8192)
	maxTurns := int32(250)
	temperature := 0.7
	maxTasks := int32(50)
	maxConcurrent := int32(100)
	return &bridgepkg.AgentConfig{
		MaxTokens:                  &maxTokens,
		MaxTurns:                   &maxTurns,
		Temperature:                &temperature,
		MaxTasksPerConversation:    &maxTasks,
		MaxConcurrentConversations: &maxConcurrent,
	}
}

// NewForgeController creates a forge controller.
func NewForgeController(
	db *gorm.DB,
	orchestrator ForgeOrchestrator,
	pusher ForgePusher,
	signingKey []byte,
	cfg *config.Config,
	eventBus *streaming.EventBus,
	cat *catalog.Catalog,
	redisOpt ...asynq.RedisConnOpt,
) *ForgeController {
	fc := &ForgeController{
		db:           db,
		orchestrator: orchestrator,
		pusher:       pusher,
		catalog:      cat,
		signingKey:   signingKey,
		cfg:          cfg,
		eventBus:     eventBus,
		reader:       &BridgeReader{},
		events:       &eventEmitter{db: db, eventBus: eventBus},
	}
	if len(redisOpt) > 0 && redisOpt[0] != nil {
		fc.inspector = asynq.NewInspector(redisOpt[0])
	}
	return fc
}

// Execute runs the forge orchestration loop for a given run ID.
// Called directly by the Asynq task handler. The context carries Asynq's
// deadline and cancellation signal.
func (fc *ForgeController) Execute(ctx context.Context, runID uuid.UUID) {
	fc.run(ctx, runID)
}

// Cancel cancels a running forge via the Asynq inspector.
func (fc *ForgeController) Cancel(runID uuid.UUID) bool {
	// Look up the Asynq task ID from the forge run record.
	var run model.ForgeRun
	if err := fc.db.Select("asynq_task_id").Where("id = ?", runID).First(&run).Error; err != nil {
		return false
	}
	if run.AsynqTaskID == "" || fc.inspector == nil {
		// No task ID or no inspector — mark as cancelled directly.
		fc.cancelRun(runID)
		return true
	}
	if err := fc.inspector.CancelProcessing(run.AsynqTaskID); err != nil {
		slog.Warn("failed to cancel asynq task, marking run as cancelled directly",
			"forge_run_id", runID,
			"asynq_task_id", run.AsynqTaskID,
			"error", err,
		)
		fc.cancelRun(runID)
	}
	return true
}

// ContextGatheringResult is returned by SetupContextGathering with the IDs
// the frontend needs to connect to the conversation.
type ContextGatheringResult struct {
	ForgeRunID     uuid.UUID
	ConversationID string // Bridge conversation ID
}

// SetupContextGathering creates a ForgeRun in gathering_context status,
// provisions the forge-context-gatherer system agent, creates a Bridge
// conversation, and sends an initial message with the target agent's details.
//
// Called by the agent handler when an agent is created with forge=true.
func (fc *ForgeController) SetupContextGathering(ctx context.Context, agent *model.Agent, cred *model.Credential, forgeRun *model.ForgeRun) (*ContextGatheringResult, error) {
	log := slog.With("agent_id", agent.ID, "forge_run_id", forgeRun.ID)

	// Determine provider group and load the context-gatherer system agent.
	providerGroup := systemagents.MapProviderToGroup(cred.ProviderID)
	agentName := fmt.Sprintf("forge-context-gatherer-%s", providerGroup)
	gathererAgent, err := fc.loadSystemAgent(agentName)
	if err != nil {
		return nil, fmt.Errorf("loading context gatherer agent: %w", err)
	}

	// Set require_approval on start_forge so the user must approve before forge begins.
	gathererAgent.Permissions = model.JSON{"start_forge": "require_approval"}

	// Provision the system agent (sandbox + push to Bridge).
	client, err := fc.ensureSystemAgentReady(ctx, gathererAgent)
	if err != nil {
		return nil, fmt.Errorf("provisioning context gatherer: %w", err)
	}

	// Mint a proxy token using the agent's credential.
	proxyToken, jti, err := fc.mintToken(forgeRun.OrgID, cred.ID)
	if err != nil {
		return nil, fmt.Errorf("minting context gatherer token: %w", err)
	}
	_ = proxyToken // TODO: pass as per-conversation auth override when Bridge supports it

	// Create Bridge conversation.
	convResp, err := client.CreateConversation(ctx, gathererAgent.ID.String())
	if err != nil {
		return nil, fmt.Errorf("creating context conversation: %w", err)
	}
	convID := convResp.ConversationId

	// Send the initial message with agent details and resolved tools.
	initialMsg := fc.buildContextGatheringMessage(agent)
	if err := client.SendMessage(ctx, convID, initialMsg); err != nil {
		log.Warn("forge: failed to send initial context message", "error", err)
		// Non-fatal — the conversation exists, the user can still chat.
	}

	// Save conversation record so the frontend can stream from it.
	agentConv := model.AgentConversation{
		OrgID:                 forgeRun.OrgID,
		AgentID:               gathererAgent.ID,
		SandboxID:             *gathererAgent.SandboxID,
		BridgeConversationID:  convID,
		Status:                "active",
	}
	if err := fc.db.Create(&agentConv).Error; err != nil {
		return nil, fmt.Errorf("saving context conversation record: %w", err)
	}

	// Update ForgeRun with context gathering state.
	fc.db.Model(forgeRun).Updates(map[string]any{
		"context_conversation_id":    agentConv.ID,
		"context_gatherer_agent_id":  gathererAgent.ID.String(),
		"context_gatherer_token_jti": jti,
	})

	log.Info("forge: context gathering conversation created",
		"conversation_id", convID,
		"gatherer_agent", agentName,
	)

	return &ContextGatheringResult{
		ForgeRunID:     forgeRun.ID,
		ConversationID: agentConv.ID.String(),
	}, nil
}

// buildContextGatheringMessage creates the initial message sent to the context
// gatherer agent with the target agent's full details — including tools exactly
// as they'll appear at runtime via MCP (provider_actionKey naming, descriptions
// from the catalog).
func (fc *ForgeController) buildContextGatheringMessage(agent *model.Agent) string {
	var msg strings.Builder

	msg.WriteString(fmt.Sprintf("Here is the agent you'll be helping to optimize:\n\n**Agent Name:** %s", agent.Name))

	if agent.Description != nil && *agent.Description != "" {
		msg.WriteString(fmt.Sprintf("\n**Description:** %s", *agent.Description))
	}

	if agent.Model != "" {
		msg.WriteString(fmt.Sprintf("\n**Model:** %s", agent.Model))
	}

	// Current system prompt (may be empty for new forge agents).
	if agent.SystemPrompt != "" {
		msg.WriteString(fmt.Sprintf("\n\n## Current System Prompt\n\n%s", agent.SystemPrompt))
	}

	// Resolve integration actions into the exact tool names and descriptions
	// that the agent will have at runtime via MCP.
	var resolvedActions []ResolvedAction
	if fc.catalog != nil {
		actions, err := resolveAgentActions(fc.db, fc.catalog, agent)
		if err == nil && len(actions) > 0 {
			resolvedActions = actions
		}
	}

	// Custom tools defined directly on the agent.
	var customTools []ToolDefinition
	if len(agent.Tools) > 0 {
		toolsBytes, _ := json.Marshal(agent.Tools)
		json.Unmarshal(toolsBytes, &customTools)
	}

	// Show all available tools in a unified list.
	hasTools := len(resolvedActions) > 0 || len(customTools) > 0
	if hasTools {
		msg.WriteString("\n\n## Available Tools\n\n")
		msg.WriteString("These are the exact tools the agent will have at runtime:\n\n")

		for _, action := range resolvedActions {
			msg.WriteString(fmt.Sprintf("### `%s`", action.ToolName))
			if action.Access != "" {
				msg.WriteString(fmt.Sprintf(" (%s)", action.Access))
			}
			msg.WriteString("\n")
			if action.Description != "" {
				msg.WriteString(fmt.Sprintf("%s\n", action.Description))
			}
			if len(action.Parameters) > 0 {
				var pretty json.RawMessage
				if json.Unmarshal(action.Parameters, &pretty) == nil {
					prettyBytes, _ := json.MarshalIndent(pretty, "", "  ")
					msg.WriteString(fmt.Sprintf("```json\n%s\n```\n", string(prettyBytes)))
				}
			}
			msg.WriteString("\n")
		}

		for _, tool := range customTools {
			msg.WriteString(fmt.Sprintf("### `%s`\n", tool.Name))
			if tool.Description != "" {
				msg.WriteString(fmt.Sprintf("%s\n", tool.Description))
			}
			if tool.Parameters != nil {
				paramsJSON, _ := json.MarshalIndent(tool.Parameters, "", "  ")
				msg.WriteString(fmt.Sprintf("```json\n%s\n```\n", string(paramsJSON)))
			}
			msg.WriteString("\n")
		}
	}

	// Instructions if present.
	if agent.Instructions != nil && *agent.Instructions != "" {
		msg.WriteString(fmt.Sprintf("\n\n## Additional Instructions\n\n%s", *agent.Instructions))
	}

	msg.WriteString("\n\nPlease greet the user and begin gathering requirements for this agent.")

	return msg.String()
}

// DesignEvals generates eval cases for a forge run using the eval designer agent.
// Called by the forge:design_evals Asynq task after context gathering is approved.
// On completion, transitions the run to reviewing_evals so the user can review.
func (fc *ForgeController) DesignEvals(ctx context.Context, runID uuid.UUID) {
	log := slog.With("forge_run_id", runID)
	log.Info("forge: designing evals")

	var run model.ForgeRun
	if err := fc.db.Preload("Agent").Where("id = ?", runID).First(&run).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading forge run: %v", err))
		return
	}

	if run.Status != model.ForgeStatusDesigningEvals {
		log.Warn("forge: run not in designing_evals status", "status", run.Status)
		return
	}

	// Load eval designer credential.
	var evalCred model.Credential
	if err := fc.db.Where("id = ?", run.EvalDesignerCredentialID).First(&evalCred).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading eval designer credential: %v", err))
		return
	}

	// Load and provision the eval designer system agent.
	providerGroup := systemagents.MapProviderToGroup(evalCred.ProviderID)
	evalDesignerAgent, err := fc.loadSystemAgent(fmt.Sprintf("forge-eval-designer-%s", providerGroup))
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("loading eval designer agent: %v", err))
		return
	}

	evalClient, err := fc.ensureSystemAgentReady(ctx, evalDesignerAgent)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("provisioning eval designer: %v", err))
		return
	}

	// Mint proxy token.
	_, _, err = fc.mintToken(run.OrgID, evalCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting eval designer token: %v", err))
		return
	}

	// Create conversation and send eval designer prompt.
	evalConv, err := evalClient.CreateConversation(ctx, evalDesignerAgent.ID.String())
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("creating eval designer conversation: %v", err))
		return
	}

	evalMessage := fc.buildEvalDesignerMessageFromContext(&run)
	evalResponse, err := fc.reader.ReadFullResponse(ctx, evalClient, evalConv.ConversationId, evalMessage)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("eval designer response: %v", err))
		return
	}

	// Parse output, retry once on invalid JSON.
	evalOutput, err := ParseEvalDesignerOutput(evalResponse)
	if err != nil {
		log.Warn("eval designer returned invalid JSON, retrying", "error", err)
		evalResponse, err = fc.reader.ReadFullResponse(ctx, evalClient, evalConv.ConversationId,
			"Your previous response was not valid JSON. Please respond with valid JSON matching the required schema.")
		if err != nil {
			fc.failRun(runID, fmt.Sprintf("eval designer retry: %v", err))
			return
		}
		evalOutput, err = ParseEvalDesignerOutput(evalResponse)
		if err != nil {
			fc.failRun(runID, fmt.Sprintf("eval designer output still invalid: %v", err))
			return
		}
	}

	// Validate mocks against real action schemas.
	if fc.catalog != nil {
		actions, resolveErr := resolveAgentActions(fc.db, fc.catalog, &run.Agent)
		if resolveErr == nil && len(actions) > 0 {
			if warnings := validateEvalMocks(evalOutput.Evals, actions); len(warnings) > 0 {
				for _, warning := range warnings {
					log.Warn("eval mock validation warning", "warning", warning)
				}
			}
		}
	}

	// Create ForgeEvalCase records.
	for index, evalCase := range evalOutput.Evals {
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
			ForgeRunID:          run.ID,
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
		fc.db.Create(&record)
	}

	// End conversation and update status.
	_ = evalClient.EndConversation(ctx, evalConv.ConversationId)

	fc.db.Model(&run).Update("status", model.ForgeStatusReviewingEvals)

	fc.events.emit(ctx, runID, EventEvalsDesigned, map[string]any{
		"count": len(evalOutput.Evals),
	})

	log.Info("forge: evals designed, awaiting user review",
		"eval_count", len(evalOutput.Evals),
	)
}

// buildEvalDesignerMessageFromContext constructs the eval designer prompt from
// gathered context and agent details (not architect output). This is used when
// evals are designed before iterations begin.
func (fc *ForgeController) buildEvalDesignerMessageFromContext(run *model.ForgeRun) string {
	msg := fmt.Sprintf("Generate a comprehensive test suite for the following agent:\n\nAgent Name: %s", run.Agent.Name)

	if run.Agent.Description != nil && *run.Agent.Description != "" {
		msg += fmt.Sprintf("\nDescription: %s", *run.Agent.Description)
	}

	if run.Agent.SystemPrompt != "" {
		msg += fmt.Sprintf("\n\nCurrent System Prompt:\n%s", run.Agent.SystemPrompt)
	}

	if len(run.Agent.Tools) > 0 {
		toolsJSON, _ := json.Marshal(run.Agent.Tools)
		if string(toolsJSON) != "{}" && string(toolsJSON) != "[]" {
			msg += fmt.Sprintf("\n\nCurrent Tools:\n%s", string(toolsJSON))
		}
	}

	// Inject gathered context.
	if len(run.Context) > 0 {
		var forgeCtx ForgeContext
		if json.Unmarshal(run.Context, &forgeCtx) == nil {
			msg += "\n\n## User-Provided Requirements\n"
			msg += fmt.Sprintf("\n**Summary:** %s", forgeCtx.RequirementsSummary)
			if len(forgeCtx.SuccessCriteria) > 0 {
				msg += "\n\n**Success Criteria:**"
				for _, criterion := range forgeCtx.SuccessCriteria {
					msg += fmt.Sprintf("\n- %s", criterion)
				}
			}
			if len(forgeCtx.EdgeCases) > 0 {
				msg += "\n\n**Edge Cases:**"
				for _, edgeCase := range forgeCtx.EdgeCases {
					msg += fmt.Sprintf("\n- %s", edgeCase)
				}
			}
			if forgeCtx.ToneAndStyle != "" {
				msg += fmt.Sprintf("\n\n**Tone & Style:** %s", forgeCtx.ToneAndStyle)
			}
			if len(forgeCtx.Constraints) > 0 {
				msg += "\n\n**Constraints:**"
				for _, constraint := range forgeCtx.Constraints {
					msg += fmt.Sprintf("\n- %s", constraint)
				}
			}
			if len(forgeCtx.ExampleInteractions) > 0 {
				msg += "\n\n**Example Interactions:**"
				for _, example := range forgeCtx.ExampleInteractions {
					msg += fmt.Sprintf("\nUser: %q\nExpected: %q\n", example.User, example.ExpectedResponse)
				}
			}
			if forgeCtx.PriorityFocus != "" {
				msg += fmt.Sprintf("\n\n**Priority Focus:** %s", forgeCtx.PriorityFocus)
			}
		}
	}

	// Inject real action schemas from the catalog.
	if fc.catalog != nil {
		actions, err := resolveAgentActions(fc.db, fc.catalog, &run.Agent)
		if err == nil && len(actions) > 0 {
			msg += "\n\n" + formatActionsForEvalDesigner(actions)
		}
	}

	msg += "\n\nGenerate at least 5 eval cases with diverse categories (happy_path, edge_case, adversarial, tool_error). " +
		"Include realistic tool mocks with multiple samples per tool. " +
		"Classify each eval as basic/standard/adversarial tier and hard/soft requirement type. " +
		"Basic tier evals test fundamental correctness and must always pass. " +
		"Hard requirement evals are pass/fail with no partial credit. " +
		"Soft requirement evals allow partial scores. " +
		"Include deterministic_checks where applicable (tool_called, tool_not_called, tool_order, response_contains, etc.). " +
		"Set sample_count (1-5) for each eval — higher for non-deterministic behaviors."

	return msg
}

// run is the main forge orchestration loop.
func (fc *ForgeController) run(ctx context.Context, runID uuid.UUID) {
	log := slog.With("forge_run_id", runID)
	started := time.Now()

	log.Info("forge: run starting")

	// Recover from panics.
	defer func() {
		if r := recover(); r != nil {
			log.Error("forge: run panicked", "panic", r, "elapsed_ms", time.Since(started).Milliseconds())
			fc.failRun(runID, fmt.Sprintf("panic: %v", r))
		}
	}()

	// Load the forge run.
	var run model.ForgeRun
	if err := fc.db.Preload("Agent").Where("id = ?", runID).First(&run).Error; err != nil {
		log.Error("forge: failed to load run", "error", err)
		return
	}
	log.Info("forge: run loaded",
		"forge_run_agent_id", run.AgentID,
		"forge_run_agent_name", run.Agent.Name,
		"forge_run_org_id", run.OrgID,
		"forge_run_max_iterations", run.MaxIterations,
		"forge_run_pass_threshold", run.PassThreshold,
		"forge_run_convergence_limit", run.ConvergenceLimit,
	)

	// Load the 3 credentials.
	log.Info("forge: loading credentials")
	var archCred, evalCred, judgeCred model.Credential
	if err := fc.db.Where("id = ?", run.ArchitectCredentialID).First(&archCred).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading architect credential: %v", err))
		return
	}
	if err := fc.db.Where("id = ?", run.EvalDesignerCredentialID).First(&evalCred).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading eval designer credential: %v", err))
		return
	}
	if err := fc.db.Where("id = ?", run.JudgeCredentialID).First(&judgeCred).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading judge credential: %v", err))
		return
	}

	// Load the target agent's credential (for eval execution).
	var targetCred model.Credential
	if run.Agent.CredentialID == nil {
		fc.failRun(runID, "target agent has no credential")
		return
	}
	if err := fc.db.Where("id = ?", *run.Agent.CredentialID).First(&targetCred).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading target credential: %v", err))
		return
	}
	log.Info("forge: credentials loaded",
		"forge_run_architect_provider", archCred.ProviderID,
		"forge_run_eval_provider", evalCred.ProviderID,
		"forge_run_judge_provider", judgeCred.ProviderID,
		"forge_run_target_provider", targetCred.ProviderID,
	)

	// Phase: PROVISIONING — load system agents and create conversations.
	log.Info("forge: provisioning — loading system agents")
	fc.updateRunStatus(&run, model.ForgeStatusProvisioning)

	targetProviderID := targetCred.ProviderID
	providerGroup := systemagents.MapProviderToGroup(targetProviderID)

	// Load the 3 system agents from DB.
	archAgent, err := fc.loadSystemAgent(fmt.Sprintf("forge-architect-%s", providerGroup))
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("loading architect system agent: %v", err))
		return
	}
	evalDesignerAgent, err := fc.loadSystemAgent(fmt.Sprintf("forge-eval-designer-%s", providerGroup))
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("loading eval designer system agent: %v", err))
		return
	}
	judgeAgent, err := fc.loadSystemAgent(fmt.Sprintf("forge-judge-%s", providerGroup))
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("loading judge system agent: %v", err))
		return
	}

	log.Info("forge: system agents loaded",
		"forge_run_architect_agent", archAgent.Name,
		"forge_run_eval_designer_agent", evalDesignerAgent.Name,
		"forge_run_judge_agent", judgeAgent.Name,
		"forge_run_provider_group", providerGroup,
	)

	// Ensure each system agent has a sandbox and is pushed to Bridge.
	log.Info("forge: preparing system agent sandboxes")
	archClient, err := fc.ensureSystemAgentReady(ctx, archAgent)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("preparing architect agent: %v", err))
		return
	}
	evalDesignerClient, err := fc.ensureSystemAgentReady(ctx, evalDesignerAgent)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("preparing eval designer agent: %v", err))
		return
	}
	judgeClient, err := fc.ensureSystemAgentReady(ctx, judgeAgent)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("preparing judge agent: %v", err))
		return
	}

	log.Info("forge: system agent sandboxes ready")

	// Mint proxy tokens from user's credentials.
	log.Info("forge: minting proxy tokens")
	archToken, archJTI, err := fc.mintToken(run.OrgID, archCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting architect token: %v", err))
		return
	}
	_, evalJTI, err := fc.mintToken(run.OrgID, evalCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting eval designer token: %v", err))
		return
	}
	_, judgeJTI, err := fc.mintToken(run.OrgID, judgeCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting judge token: %v", err))
		return
	}
	// TODO: Pass tokens as per-conversation auth override when Bridge supports it.
	_ = archToken

	// Mint eval target token (for direct proxy calls during eval execution).
	evalTargetToken, evalTargetJTI, err := fc.mintToken(run.OrgID, targetCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting eval target token: %v", err))
		return
	}

	// Store token JTIs and system agent IDs.
	fc.db.Model(&run).Updates(map[string]any{
		"architect_token_jti":     archJTI,
		"eval_designer_token_jti": evalJTI,
		"judge_token_jti":         judgeJTI,
		"eval_target_token_jti":   evalTargetJTI,
		"architect_agent_id":      archAgent.ID.String(),
		"eval_designer_agent_id":  evalDesignerAgent.ID.String(),
		"judge_agent_id":          judgeAgent.ID.String(),
	})

	now := time.Now()
	fc.db.Model(&run).Updates(map[string]any{
		"status":     model.ForgeStatusRunning,
		"started_at": now,
	})
	run.Status = model.ForgeStatusRunning
	run.StartedAt = &now
	fc.events.emit(ctx, runID, EventProvisioned, map[string]any{
		"architect_agent":      archAgent.Name,
		"eval_designer_agent":  evalDesignerAgent.Name,
		"judge_agent":          judgeAgent.Name,
	})

	log.Info("forge: provisioning complete, creating architect conversation",
		"elapsed_ms", time.Since(started).Milliseconds(),
	)

	// Create architect conversation (reused across iterations).
	archConv, err := archClient.CreateConversation(ctx, archAgent.ID.String())
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("creating architect conversation: %v", err))
		return
	}
	run.ArchitectConversationID = archConv.ConversationId
	fc.db.Model(&run).Update("architect_conversation_id", archConv.ConversationId)

	// ITERATION LOOP
	log.Info("forge: starting iteration loop",
		"forge_run_max_iterations", run.MaxIterations,
		"forge_run_architect_conv_id", run.ArchitectConversationID,
	)

	var bestScore float64 = -1
	var bestIteration *model.ForgeIteration
	for i := 1; i <= run.MaxIterations; i++ {
		if ctx.Err() != nil {
			log.Info("forge: cancelled before iteration", "iteration", i)
			fc.cancelRun(runID)
			return
		}

		iterStarted := time.Now()
		log.Info("forge: iteration starting",
			"iteration", i,
			"forge_run_best_score", bestScore,
			"forge_run_convergence_count", run.ConvergenceCount,
		)

		run.CurrentIteration = i
		fc.db.Model(&run).Update("current_iteration", i)
		fc.events.emit(ctx, runID, EventIterationStarted, map[string]any{
			"iteration": i,
		})

		iter, err := fc.runIteration(ctx, &run, i,
			archAgent, archClient,
			evalDesignerAgent, evalDesignerClient,
			judgeAgent, judgeClient,
			targetProviderID, evalTargetToken,
		)
		if err != nil {
			log.Error("forge: iteration failed", "iteration", i, "error", err, "elapsed_ms", time.Since(iterStarted).Milliseconds())
			// Continue to next iteration on non-fatal errors.
			if ctx.Err() != nil {
				fc.cancelRun(runID)
				return
			}
			continue
		}

		log.Info("forge: iteration completed",
			"iteration", i,
			"forge_run_score", iter.Score,
			"forge_run_hard_score", iter.HardScore,
			"forge_run_soft_score", iter.SoftScore,
			"forge_run_all_hard_passed", iter.AllHardPassed,
			"forge_run_passed_evals", iter.PassedEvals,
			"forge_run_total_evals", iter.TotalEvals,
			"iteration_elapsed_ms", time.Since(iterStarted).Milliseconds(),
		)

		fc.events.emit(ctx, runID, EventIterationCompleted, map[string]any{
			"iteration":       i,
			"score":           iter.Score,
			"hard_score":      iter.HardScore,
			"soft_score":      iter.SoftScore,
			"all_hard_passed": iter.AllHardPassed,
			"passed_evals":    iter.PassedEvals,
			"total_evals":     iter.TotalEvals,
		})

		// Track best iteration.
		if bestIteration == nil || iter.Score > bestIteration.Score {
			bestIteration = iter
		}

		// Convergence criteria: threshold met when all hard pass + soft above threshold.
		if iter.AllHardPassed && iter.SoftScore >= run.PassThreshold {
			run.StopReason = model.ForgeStopThresholdMet
			fc.db.Model(&run).Update("stop_reason", run.StopReason)
			log.Info("forge threshold met", "iteration", i, "score", iter.Score,
				"hard_score", iter.HardScore, "soft_score", iter.SoftScore)
			break
		}

		// Convergence: N iterations with no improvement.
		if iter.Score > bestScore {
			run.ConvergenceCount = 0
		} else {
			run.ConvergenceCount++
		}
		if iter.Score > bestScore {
			bestScore = iter.Score
		}
		fc.db.Model(&run).Update("convergence_count", run.ConvergenceCount)

		if run.ConvergenceCount >= run.ConvergenceLimit {
			run.StopReason = model.ForgeStopConverged
			fc.db.Model(&run).Update("stop_reason", run.StopReason)
			log.Info("forge converged — no improvement", "iteration", i,
				"convergence_count", run.ConvergenceCount)
			break
		}

		// Max iterations — set stop reason on last iteration.
		if i >= run.MaxIterations {
			run.StopReason = model.ForgeStopMaxIterations
			fc.db.Model(&run).Update("stop_reason", run.StopReason)
		}
	}

	// Complete the run.
	log.Info("forge: run completing",
		"forge_run_total_iterations", run.CurrentIteration,
		"forge_run_stop_reason", run.StopReason,
		"forge_run_best_score", bestScore,
		"total_elapsed_ms", time.Since(started).Milliseconds(),
	)
	fc.completeRun(ctx, &run, bestIteration)
}

// runIteration executes a single design→eval→judge cycle.
func (fc *ForgeController) runIteration(
	ctx context.Context, run *model.ForgeRun, iteration int,
	archAgent *model.Agent, archClient *bridgepkg.BridgeClient,
	evalDesignerAgent *model.Agent, evalDesignerClient *bridgepkg.BridgeClient,
	judgeAgent *model.Agent, judgeClient *bridgepkg.BridgeClient,
	targetProviderID, evalTargetToken string,
) (*model.ForgeIteration, error) {
	_ = archAgent // architect uses persistent conversation from run.ArchitectConversationID
	log := slog.With("forge_run_id", run.ID, "iteration", iteration)
	phaseStart := time.Now()

	log.Info("forge: iteration — creating record")

	// Create iteration record.
	iter := model.ForgeIteration{
		ForgeRunID: run.ID,
		Iteration:  iteration,
		Phase:      model.ForgePhaseDesigning,
	}
	if err := fc.db.Create(&iter).Error; err != nil {
		return nil, fmt.Errorf("creating iteration record: %w", err)
	}

	// PHASE: DESIGNING
	log.Info("forge: phase=designing — sending to architect")
	fc.events.emit(ctx, run.ID, EventArchitectStarted, map[string]any{"iteration": iteration})

	archMessage := fc.buildArchitectMessage(run, iteration)
	archResponse, err := fc.reader.ReadFullResponse(ctx, archClient, run.ArchitectConversationID, archMessage)
	if err != nil {
		fc.updateIterPhase(&iter, model.ForgePhaseFailed)
		return nil, fmt.Errorf("architect response: %w", err)
	}

	archOutput, err := ParseArchitectOutput(archResponse)
	if err != nil {
		log.Warn("architect returned invalid JSON, retrying", "error", err)
		// Retry with corrective message.
		archResponse, err = fc.reader.ReadFullResponse(ctx, archClient, run.ArchitectConversationID,
			"Your previous response was not valid JSON. Please respond with valid JSON matching the required schema.")
		if err != nil {
			fc.updateIterPhase(&iter, model.ForgePhaseFailed)
			return nil, fmt.Errorf("architect retry response: %w", err)
		}
		archOutput, err = ParseArchitectOutput(archResponse)
		if err != nil {
			fc.updateIterPhase(&iter, model.ForgePhaseFailed)
			return nil, fmt.Errorf("architect output still invalid: %w", err)
		}
	}

	// Persist architect output.
	toolsJSON, _ := json.Marshal(archOutput.Tools)
	configJSON, _ := json.Marshal(archOutput.AgentConfig)
	fc.db.Model(&iter).Updates(map[string]any{
		"system_prompt":       archOutput.SystemPrompt,
		"tools":               model.RawJSON(toolsJSON),
		"agent_config":        model.RawJSON(configJSON),
		"architect_reasoning": archOutput.Reasoning,
		"architect_response":  archResponse,
		"phase":               model.ForgePhaseEvalDesigning,
	})
	iter.SystemPrompt = archOutput.SystemPrompt
	iter.Phase = model.ForgePhaseEvalDesigning

	log.Info("forge: phase=designing — architect completed",
		"system_prompt_len", len(archOutput.SystemPrompt),
		"tools_count", len(archOutput.Tools),
		"has_reasoning", archOutput.Reasoning != "",
		"phase_elapsed_ms", time.Since(phaseStart).Milliseconds(),
	)

	fc.events.emit(ctx, run.ID, EventArchitectCompleted, map[string]any{
		"iteration":         iteration,
		"system_prompt_len": len(archOutput.SystemPrompt),
		"tools_count":       len(archOutput.Tools),
	})

	// PHASE: EVAL_DESIGNING — only in iteration 1.
	// In subsequent iterations, reuse ForgeEvalCase records from the run.
	phaseStart = time.Now()
	var evalCases []model.ForgeEvalCase

	if iteration == 1 {
		// Check if eval cases were already created by the DesignEvals step (new flow).
		fc.db.Where("forge_run_id = ?", run.ID).Order("order_index ASC").Find(&evalCases)
		if len(evalCases) > 0 {
			log.Info("forge: phase=eval_designing — reusing pre-designed eval cases", "eval_count", len(evalCases))
			fc.db.Model(&iter).Update("phase", model.ForgePhaseEvaluating)
			iter.Phase = model.ForgePhaseEvaluating
			goto evalPhase
		}

		// Backward compatibility: generate evals inline (for runs via direct Start endpoint).
		log.Info("forge: phase=eval_designing — generating eval cases")
		fc.events.emit(ctx, run.ID, EventEvalDesignStarted, map[string]any{"iteration": iteration})

		evalConv, err := evalDesignerClient.CreateConversation(ctx, evalDesignerAgent.ID.String())
		if err != nil {
			fc.updateIterPhase(&iter, model.ForgePhaseFailed)
			return nil, fmt.Errorf("creating eval designer conversation: %w", err)
		}

		evalMessage := fc.buildEvalDesignerMessage(archOutput, &run.Agent)
		evalResponse, err := fc.reader.ReadFullResponse(ctx, evalDesignerClient, evalConv.ConversationId, evalMessage)
		if err != nil {
			fc.updateIterPhase(&iter, model.ForgePhaseFailed)
			return nil, fmt.Errorf("eval designer response: %w", err)
		}

		evalOutput, err := ParseEvalDesignerOutput(evalResponse)
		if err != nil {
			log.Warn("eval designer returned invalid JSON, retrying", "error", err)
			evalResponse, err = fc.reader.ReadFullResponse(ctx, evalDesignerClient, evalConv.ConversationId,
				"Your previous response was not valid JSON. Please respond with valid JSON matching the required schema.")
			if err != nil {
				fc.updateIterPhase(&iter, model.ForgePhaseFailed)
				return nil, fmt.Errorf("eval designer retry: %w", err)
			}
			evalOutput, err = ParseEvalDesignerOutput(evalResponse)
			if err != nil {
				fc.updateIterPhase(&iter, model.ForgePhaseFailed)
				return nil, fmt.Errorf("eval designer output still invalid: %w", err)
			}
		}

		// Validate generated mocks against real action schemas (if integrations exist).
		if fc.catalog != nil {
			actions, _ := resolveAgentActions(fc.db, fc.catalog, &run.Agent)
			if warnings := validateEvalMocks(evalOutput.Evals, actions); len(warnings) > 0 {
				for _, w := range warnings {
					log.Warn("eval mock validation warning", "warning", w)
				}
			}
		}

		// Create ForgeEvalCase records (belong to the run, reused across iterations).
		for _, ec := range evalOutput.Evals {
			mocksJSON, _ := json.Marshal(ec.ToolMocks)
			rubricJSON, _ := json.Marshal(ec.Rubric)
			checksJSON, _ := json.Marshal(ec.DeterministicChecks)

			sampleCount := ec.SampleCount
			if sampleCount < 1 {
				sampleCount = 3
			}
			if sampleCount > 5 {
				sampleCount = 5
			}

			evalCase := model.ForgeEvalCase{
				ForgeRunID:          run.ID,
				TestName:            ec.Name,
				Category:            ec.Category,
				Tier:                ec.Tier,
				RequirementType:     ec.RequirementType,
				SampleCount:         sampleCount,
				TestPrompt:          ec.TestPrompt,
				ExpectedBehavior:    ec.ExpectedBehavior,
				ToolMocks:           model.RawJSON(mocksJSON),
				Rubric:              model.RawJSON(rubricJSON),
				DeterministicChecks: model.RawJSON(checksJSON),
			}
			fc.db.Create(&evalCase)
			evalCases = append(evalCases, evalCase)
		}

		fc.db.Model(&iter).Updates(map[string]any{
			"eval_designer_response": evalResponse,
			"phase":                  model.ForgePhaseEvaluating,
		})
		iter.Phase = model.ForgePhaseEvaluating

		log.Info("forge: phase=eval_designing — eval cases generated",
			"eval_count", len(evalOutput.Evals),
			"phase_elapsed_ms", time.Since(phaseStart).Milliseconds(),
		)

		fc.events.emit(ctx, run.ID, EventEvalsGenerated, map[string]any{
			"iteration": iteration,
			"count":     len(evalOutput.Evals),
		})

		// End eval designer conversation (no longer needed).
		_ = evalDesignerClient.EndConversation(ctx, evalConv.ConversationId)
	} else {
		// Iterations 2+: load existing ForgeEvalCase records from the run.
		fc.db.Where("forge_run_id = ?", run.ID).Order("order_index ASC").Find(&evalCases)
		log.Info("forge: phase=eval_designing — reusing existing eval cases", "eval_count", len(evalCases))

		fc.db.Model(&iter).Update("phase", model.ForgePhaseEvaluating)
		iter.Phase = model.ForgePhaseEvaluating
	}

evalPhase:
	// PHASE: EVALUATING — push eval-target agent to a pool sandbox with MCP mocks.
	phaseStart = time.Now()
	log.Info("forge: phase=evaluating — preparing eval target agent", "eval_count", len(evalCases))
	evalTargetAgentID := uuid.New().String()
	proxyBaseURL := fmt.Sprintf("https://%s/v1/proxy", fc.cfg.BridgeHost)
	mcpURL := fmt.Sprintf("%s/forge/%s", fc.cfg.MCPBaseURL, run.ID.String())

	evalTargetProviderType := bridgepkg.Custom
	if pt, ok := providerTypeMap[targetProviderID]; ok {
		evalTargetProviderType = pt
	}

	// Build MCP transport with auth headers so Bridge can reach our forge MCP server.
	headers := map[string]string{
		"Authorization": "Bearer " + evalTargetToken,
	}
	var mcpTransport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type:    bridgepkg.StreamableHttp,
		Url:     mcpURL,
		Headers: &headers,
	}
	mcpTransport.FromMcpTransport1(httpTransport)
	mcpServers := []bridgepkg.McpServerDefinition{{
		Name:      "forge-mock",
		Transport: mcpTransport,
	}}

	evalTargetDef := bridgepkg.AgentDefinition{
		Id:           evalTargetAgentID,
		Name:         "forge-eval-target",
		SystemPrompt: archOutput.SystemPrompt,
		Provider: bridgepkg.ProviderConfig{
			ProviderType: evalTargetProviderType,
			Model:        run.Agent.Model,
			ApiKey:       evalTargetToken,
			BaseUrl:      &proxyBaseURL,
		},
		McpServers: &mcpServers,
		Config:     defaultAgentConfig(),
	}

	// Get a pool sandbox for the eval-target and push the agent.
	evalTargetSb, evalTargetClient, err := fc.pushEvalTargetToPool(ctx, evalTargetAgentID, evalTargetDef)
	if err != nil {
		fc.updateIterPhase(&iter, model.ForgePhaseFailed)
		return nil, fmt.Errorf("pushing eval target to pool: %w", err)
	}
	_ = evalTargetSb // used for cleanup
	defer func() {
		_ = evalTargetClient.RemoveAgentDefinition(ctx, evalTargetAgentID)
	}()

	// Create ForgeEvalResult records for each eval case in this iteration.
	var evalResults []model.ForgeEvalResult
	for _, ec := range evalCases {
		result := model.ForgeEvalResult{
			ForgeEvalCaseID:  ec.ID,
			ForgeIterationID: iter.ID,
			Status:           model.ForgeEvalPending,
		}
		fc.db.Create(&result)
		evalResults = append(evalResults, result)
	}

	log.Info("forge: phase=evaluating — running eval cases", "eval_result_count", len(evalResults))

	// Run samples for each eval case.
	for idx := range evalResults {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result := &evalResults[idx]

		// Find the corresponding eval case.
		var evalCase model.ForgeEvalCase
		for _, ec := range evalCases {
			if ec.ID == result.ForgeEvalCaseID {
				evalCase = ec
				break
			}
		}

		fc.events.emit(ctx, run.ID, EventEvalStarted, map[string]any{
			"iteration":    iteration,
			"eval_name":    evalCase.TestName,
			"category":     evalCase.Category,
			"tier":         evalCase.Tier,
			"sample_count": evalCase.SampleCount,
		})

		// Mark result as running — the forge MCP server queries for this.
		fc.db.Model(result).Update("status", model.ForgeEvalRunning)

		sampleCount := evalCase.SampleCount
		if sampleCount < 1 {
			sampleCount = 1
		}

		var sampleResults []SampleResult
		var allToolCalls []ToolCallInfo // aggregate tool calls across samples for deterministic checks
		var lastResponse string

		for s := 0; s < sampleCount; s++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			// Create conversation, send test prompt, read response.
			evalConvResp, err := evalTargetClient.CreateConversation(ctx, evalTargetAgentID)
			if err != nil {
				log.Warn("eval conversation creation failed",
					"eval_name", evalCase.TestName, "sample", s, "error", err)
				sampleResults = append(sampleResults, SampleResult{
					SampleIndex: s,
					Passed:      false,
					Score:       0,
				})
				continue
			}

			bridgeResp, err := fc.reader.ReadFullResponseWithTools(ctx, evalTargetClient, evalConvResp.ConversationId, evalCase.TestPrompt)
			_ = evalTargetClient.EndConversation(ctx, evalConvResp.ConversationId)

			if err != nil {
				log.Warn("eval execution failed",
					"eval_name", evalCase.TestName, "sample", s, "error", err)
				sampleResults = append(sampleResults, SampleResult{
					SampleIndex: s,
					Passed:      false,
					Score:       0,
				})
				continue
			}

			sampleResults = append(sampleResults, SampleResult{
				SampleIndex: s,
				Response:    bridgeResp.Text,
				ToolCalls:   bridgeResp.ToolCalls,
			})

			// Track aggregate tool calls and last response for deterministic checks.
			allToolCalls = append(allToolCalls, bridgeResp.ToolCalls...)
			lastResponse = bridgeResp.Text
		}

		// Run deterministic checks before judge.
		var deterministicChecks []DeterministicCheck
		if len(evalCase.DeterministicChecks) > 0 {
			json.Unmarshal(evalCase.DeterministicChecks, &deterministicChecks)
		}

		var deterministicResults []DeterministicResult
		if len(deterministicChecks) > 0 {
			deterministicResults = RunDeterministicChecks(deterministicChecks, lastResponse, allToolCalls)
		}

		// Persist sample results and deterministic results, move to judging.
		sampleResultsJSON, _ := json.Marshal(sampleResults)
		deterministicJSON, _ := json.Marshal(deterministicResults)
		fc.db.Model(result).Updates(map[string]any{
			"sample_results":       model.RawJSON(sampleResultsJSON),
			"deterministic_results": model.RawJSON(deterministicJSON),
			"status":               model.ForgeEvalJudging,
		})
		result.SampleResults = model.RawJSON(sampleResultsJSON)
		result.DeterministicResults = model.RawJSON(deterministicJSON)

		fc.events.emit(ctx, run.ID, EventEvalCompleted, map[string]any{
			"iteration":       iteration,
			"eval_name":       evalCase.TestName,
			"sample_count":    sampleCount,
			"samples_run":     len(sampleResults),
			"det_checks_run":  len(deterministicResults),
		})
	}

	log.Info("forge: phase=evaluating — all evals executed",
		"phase_elapsed_ms", time.Since(phaseStart).Milliseconds(),
	)

	fc.db.Model(&iter).Update("phase", model.ForgePhaseJudging)
	iter.Phase = model.ForgePhaseJudging

	// PHASE: JUDGING
	phaseStart = time.Now()
	log.Info("forge: phase=judging — scoring eval results")

	// Reload eval results with judging status.
	var judgingResults []model.ForgeEvalResult
	fc.db.Where("forge_iteration_id = ? AND status = ?", iter.ID, model.ForgeEvalJudging).Find(&judgingResults)
	log.Info("forge: phase=judging — eval results to judge", "count", len(judgingResults))

	// Create a judge conversation for this iteration.
	judgeConv, err := judgeClient.CreateConversation(ctx, judgeAgent.ID.String())
	if err != nil {
		fc.updateIterPhase(&iter, model.ForgePhaseFailed)
		return nil, fmt.Errorf("creating judge conversation: %w", err)
	}

	for idx := range judgingResults {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result := &judgingResults[idx]

		// Load the corresponding eval case.
		var evalCase model.ForgeEvalCase
		fc.db.Where("id = ?", result.ForgeEvalCaseID).First(&evalCase)

		fc.events.emit(ctx, run.ID, EventJudgeStarted, map[string]any{
			"iteration": iteration,
			"eval_name": evalCase.TestName,
		})

		judgeMessage := fc.buildJudgeMessage(&evalCase, result)
		judgeResponse, err := fc.reader.ReadFullResponse(ctx, judgeClient, judgeConv.ConversationId, judgeMessage)
		if err != nil {
			log.Warn("judge failed", "eval_name", evalCase.TestName, "error", err)
			fc.db.Model(result).Update("status", model.ForgeEvalFailed)
			continue
		}

		judgeOutput, err := ParseJudgeOutput(judgeResponse)
		if err != nil {
			log.Warn("judge returned invalid JSON", "eval_name", evalCase.TestName, "error", err)
			fc.db.Model(result).Update("status", model.ForgeEvalFailed)
			continue
		}

		// Compute pass rate from sample results.
		var sampleResults []SampleResult
		json.Unmarshal(result.SampleResults, &sampleResults)

		samplesPassed := 0
		for si := range sampleResults {
			// Mark each sample based on judge score (passed if score >= 0.5).
			sampleResults[si].Passed = judgeOutput.Passed
			sampleResults[si].Score = judgeOutput.Score
			if judgeOutput.Passed {
				samplesPassed++
			}
		}
		var passRate float64
		if len(sampleResults) > 0 {
			passRate = float64(samplesPassed) / float64(len(sampleResults))
		}

		sampleResultsJSON, _ := json.Marshal(sampleResults)
		rubricScoresJSON, _ := json.Marshal(judgeOutput.RubricScores)
		fc.db.Model(result).Updates(map[string]any{
			"score":            judgeOutput.Score,
			"passed":           judgeOutput.Passed,
			"failure_category": judgeOutput.FailureCategory,
			"critique":         judgeOutput.Critique,
			"rubric_scores":    model.RawJSON(rubricScoresJSON),
			"pass_rate":        passRate,
			"sample_results":   model.RawJSON(sampleResultsJSON),
			"status":           model.ForgeEvalCompleted,
		})
		result.Score = judgeOutput.Score
		result.Passed = judgeOutput.Passed
		result.FailureCategory = judgeOutput.FailureCategory
		result.Critique = judgeOutput.Critique
		result.PassRate = passRate
		result.Status = model.ForgeEvalCompleted

		fc.events.emit(ctx, run.ID, EventJudgeCompleted, map[string]any{
			"iteration":        iteration,
			"eval_name":        evalCase.TestName,
			"score":            judgeOutput.Score,
			"passed":           judgeOutput.Passed,
			"pass_rate":        passRate,
			"failure_category": judgeOutput.FailureCategory,
		})
	}

	_ = judgeClient.EndConversation(ctx, judgeConv.ConversationId)

	log.Info("forge: phase=judging — all evals judged",
		"phase_elapsed_ms", time.Since(phaseStart).Milliseconds(),
	)

	// Compute tiered scoring.
	var completedResults []model.ForgeEvalResult
	fc.db.Where("forge_iteration_id = ? AND status = ?", iter.ID, model.ForgeEvalCompleted).Find(&completedResults)

	var (
		totalHard      int
		passedHard     int
		softScoreSum   float64
		totalSoft      int
		passedCount    int
		totalEvalCount = len(evalCases)
	)

	for _, r := range completedResults {
		// Look up the eval case to determine requirement type.
		var ec model.ForgeEvalCase
		fc.db.Where("id = ?", r.ForgeEvalCaseID).First(&ec)

		if r.Passed {
			passedCount++
		}

		switch ec.RequirementType {
		case model.ForgeRequirementHard:
			totalHard++
			if r.Passed {
				passedHard++
			}
		case model.ForgeRequirementSoft:
			totalSoft++
			softScoreSum += r.Score
		default:
			// Treat unknown as soft.
			totalSoft++
			softScoreSum += r.Score
		}
	}

	var hardScore float64
	if totalHard > 0 {
		hardScore = float64(passedHard) / float64(totalHard)
	} else {
		hardScore = 1.0 // no hard evals means all hard "pass"
	}
	allHardPassed := hardScore == 1.0

	var softScore float64
	if totalSoft > 0 {
		softScore = softScoreSum / float64(totalSoft)
	}

	var overallScore float64
	if totalEvalCount > 0 {
		overallScore = float64(passedCount) / float64(totalEvalCount)
	}

	// Build per-eval score history for regression detection.
	evalScoreHistory := fc.buildEvalScoreHistory(run.ID, evalCases, iter.ID, completedResults)
	evalScoreHistoryJSON, _ := json.Marshal(evalScoreHistory)

	fc.db.Model(&iter).Updates(map[string]any{
		"total_evals":       totalEvalCount,
		"passed_evals":      passedCount,
		"score":             overallScore,
		"hard_score":        hardScore,
		"soft_score":        softScore,
		"all_hard_passed":   allHardPassed,
		"eval_score_history": model.RawJSON(evalScoreHistoryJSON),
		"phase":             model.ForgePhaseCompleted,
	})
	iter.TotalEvals = totalEvalCount
	iter.PassedEvals = passedCount
	iter.Score = overallScore
	iter.HardScore = hardScore
	iter.SoftScore = softScore
	iter.AllHardPassed = allHardPassed
	iter.Phase = model.ForgePhaseCompleted

	return &iter, nil
}

// buildEvalScoreHistory queries all ForgeEvalResult records across all completed
// iterations for each eval case and returns per-eval score history.
func (fc *ForgeController) buildEvalScoreHistory(
	runID uuid.UUID,
	evalCases []model.ForgeEvalCase,
	currentIterID uuid.UUID,
	currentResults []model.ForgeEvalResult,
) []EvalScoreEntry {
	// Load all completed iterations for this run, ordered by iteration number.
	var allIterations []model.ForgeIteration
	fc.db.Where("forge_run_id = ? AND phase = ?", runID, model.ForgePhaseCompleted).
		Order("iteration ASC").Find(&allIterations)

	// Collect iteration IDs (including current, which may not yet be marked completed in DB).
	iterIDs := make([]uuid.UUID, 0, len(allIterations)+1)
	for _, it := range allIterations {
		iterIDs = append(iterIDs, it.ID)
	}
	// Add current iteration if not already included.
	found := false
	for _, id := range iterIDs {
		if id == currentIterID {
			found = true
			break
		}
	}
	if !found {
		iterIDs = append(iterIDs, currentIterID)
	}

	// Load all completed eval results for these iterations.
	var allResults []model.ForgeEvalResult
	if len(iterIDs) > 0 {
		fc.db.Where("forge_iteration_id IN ? AND status = ?", iterIDs, model.ForgeEvalCompleted).
			Find(&allResults)
	}

	// Also include current results that may not be persisted as completed yet.
	resultMap := make(map[uuid.UUID]map[uuid.UUID]float64) // iterID -> evalCaseID -> score
	for _, r := range allResults {
		if resultMap[r.ForgeIterationID] == nil {
			resultMap[r.ForgeIterationID] = make(map[uuid.UUID]float64)
		}
		resultMap[r.ForgeIterationID][r.ForgeEvalCaseID] = r.Score
	}
	// Overlay current results.
	for _, r := range currentResults {
		if resultMap[currentIterID] == nil {
			resultMap[currentIterID] = make(map[uuid.UUID]float64)
		}
		resultMap[currentIterID][r.ForgeEvalCaseID] = r.Score
	}

	// Build history per eval case.
	var history []EvalScoreEntry
	for _, ec := range evalCases {
		entry := EvalScoreEntry{EvalName: ec.TestName}
		for _, iterID := range iterIDs {
			if scores, ok := resultMap[iterID]; ok {
				if s, ok := scores[ec.ID]; ok {
					entry.Scores = append(entry.Scores, s)
				}
			}
		}
		history = append(history, entry)
	}

	return history
}

// buildArchitectMessage constructs the message to send to the architect.
// Includes ALL previous iterations' results for full context.
func (fc *ForgeController) buildArchitectMessage(run *model.ForgeRun, iteration int) string {
	if iteration == 1 {
		// First iteration: provide the target agent's requirements.
		msg := fmt.Sprintf(`Design an AI agent with the following requirements:

Agent Name: %s`, run.Agent.Name)

		if run.Agent.Description != nil && *run.Agent.Description != "" {
			msg += fmt.Sprintf("\nDescription: %s", *run.Agent.Description)
		}
		if run.Agent.SystemPrompt != "" {
			msg += fmt.Sprintf("\n\nCurrent System Prompt (to improve upon):\n%s", run.Agent.SystemPrompt)
		}

		// Include current tools if any.
		if len(run.Agent.Tools) > 0 {
			toolsJSON, _ := json.Marshal(run.Agent.Tools)
			if string(toolsJSON) != "{}" && string(toolsJSON) != "[]" {
				msg += fmt.Sprintf("\n\nCurrent Tools:\n%s", string(toolsJSON))
			}
		}

		// Inject user-provided context from context-gathering conversation.
		if len(run.Context) > 0 {
			var ctx ForgeContext
			if json.Unmarshal(run.Context, &ctx) == nil {
				msg += "\n\n## User-Provided Requirements\n"
				msg += fmt.Sprintf("\n**Summary:** %s", ctx.RequirementsSummary)
				if len(ctx.SuccessCriteria) > 0 {
					msg += "\n\n**Success Criteria:**"
					for _, criterion := range ctx.SuccessCriteria {
						msg += fmt.Sprintf("\n- %s", criterion)
					}
				}
				if len(ctx.EdgeCases) > 0 {
					msg += "\n\n**Edge Cases:**"
					for _, edgeCase := range ctx.EdgeCases {
						msg += fmt.Sprintf("\n- %s", edgeCase)
					}
				}
				if ctx.ToneAndStyle != "" {
					msg += fmt.Sprintf("\n\n**Tone & Style:** %s", ctx.ToneAndStyle)
				}
				if len(ctx.Constraints) > 0 {
					msg += "\n\n**Constraints:**"
					for _, constraint := range ctx.Constraints {
						msg += fmt.Sprintf("\n- %s", constraint)
					}
				}
				if len(ctx.ExampleInteractions) > 0 {
					msg += "\n\n**Example Interactions:**"
					for _, example := range ctx.ExampleInteractions {
						msg += fmt.Sprintf("\nUser: %q\nExpected: %q\n", example.User, example.ExpectedResponse)
					}
				}
				if ctx.PriorityFocus != "" {
					msg += fmt.Sprintf("\n\n**Priority Focus:** %s", ctx.PriorityFocus)
				}
			}
		}

		msg += "\n\nDesign the best possible system prompt, tools, and configuration for this agent."
		return msg
	}

	// Subsequent iterations: include FULL iteration history.
	var allIterations []model.ForgeIteration
	fc.db.Where("forge_run_id = ? AND phase = ? AND iteration < ?",
		run.ID, model.ForgePhaseCompleted, iteration).
		Order("iteration ASC").Find(&allIterations)

	// Load eval cases for this run (static across iterations).
	var evalCases []model.ForgeEvalCase
	fc.db.Where("forge_run_id = ?", run.ID).Order("order_index ASC").Find(&evalCases)

	// Build a map of eval case ID to eval case for quick lookup.
	evalCaseMap := make(map[uuid.UUID]model.ForgeEvalCase, len(evalCases))
	for _, ec := range evalCases {
		evalCaseMap[ec.ID] = ec
	}

	// Track per-eval pass/fail across iterations for regression detection.
	// evalCaseID -> iteration -> passed
	evalHistory := make(map[uuid.UUID]map[int]bool)

	msg := "## Iteration History\n\n"

	for _, prevIter := range allIterations {
		var results []model.ForgeEvalResult
		fc.db.Where("forge_iteration_id = ?", prevIter.ID).Find(&results)

		msg += fmt.Sprintf("### Iteration %d (score: %.0f%%, hard: %d/%d, soft: %.0f%%)\n",
			prevIter.Iteration, prevIter.Score*100,
			fc.countHardPassed(results, evalCaseMap),
			fc.countHardTotal(evalCases),
			prevIter.SoftScore*100)

		if prevIter.ArchitectReasoning != "" {
			msg += fmt.Sprintf("Change: %s\n", prevIter.ArchitectReasoning)
		}

		for _, r := range results {
			ec, ok := evalCaseMap[r.ForgeEvalCaseID]
			if !ok {
				continue
			}

			// Track history.
			if evalHistory[ec.ID] == nil {
				evalHistory[ec.ID] = make(map[int]bool)
			}
			evalHistory[ec.ID][prevIter.Iteration] = r.Passed

			// Determine status label.
			statusLabel := "PASSED"
			if !r.Passed {
				statusLabel = "FAILED"
			}

			// Check for regression: was passing in a previous iteration, now failing.
			if !r.Passed && prevIter.Iteration > 1 {
				for prevIterNum := prevIter.Iteration - 1; prevIterNum >= 1; prevIterNum-- {
					if prevPassed, exists := evalHistory[ec.ID][prevIterNum]; exists && prevPassed {
						statusLabel = "REGRESSION"
						break
					}
				}
			}

			// Check for fix: was failing, now passing.
			if r.Passed && prevIter.Iteration > 1 {
				for prevIterNum := prevIter.Iteration - 1; prevIterNum >= 1; prevIterNum-- {
					if prevPassed, exists := evalHistory[ec.ID][prevIterNum]; exists && !prevPassed {
						statusLabel += " — FIXED"
						break
					}
				}
			}

			msg += fmt.Sprintf("- [%s] [%s/%s] %s",
				statusLabel, ec.Tier, ec.RequirementType, ec.TestName)

			if r.FailureCategory != "" && r.FailureCategory != "none" {
				msg += fmt.Sprintf(" (%s)", r.FailureCategory)
			}

			if !r.Passed && r.Critique != "" {
				msg += fmt.Sprintf(": %s", r.Critique)
			}

			msg += "\n"
		}

		msg += "\n"
	}

	// Flag basic tier failures prominently.
	if len(allIterations) > 0 {
		lastIter := allIterations[len(allIterations)-1]
		var lastResults []model.ForgeEvalResult
		fc.db.Where("forge_iteration_id = ?", lastIter.ID).Find(&lastResults)

		var basicFailures []string
		for _, r := range lastResults {
			if !r.Passed {
				ec, ok := evalCaseMap[r.ForgeEvalCaseID]
				if ok && ec.Tier == model.ForgeEvalTierBasic {
					basicFailures = append(basicFailures, ec.TestName)
				}
			}
		}

		if len(basicFailures) > 0 {
			msg += "**CRITICAL: Basic tier evals are still failing. These must pass before addressing standard/adversarial evals:**\n"
			for _, name := range basicFailures {
				msg += fmt.Sprintf("- %s\n", name)
			}
			msg += "\n"
		}
	}

	msg += "Based on the complete iteration history above, revise the system prompt, tools, and configuration. " +
		"Focus on fixing failures (especially regressions and basic tier) while maintaining passing evals."
	return msg
}

// countHardPassed returns the number of hard eval results that passed.
func (fc *ForgeController) countHardPassed(results []model.ForgeEvalResult, evalCaseMap map[uuid.UUID]model.ForgeEvalCase) int {
	count := 0
	for _, r := range results {
		ec, ok := evalCaseMap[r.ForgeEvalCaseID]
		if ok && ec.RequirementType == model.ForgeRequirementHard && r.Passed {
			count++
		}
	}
	return count
}

// countHardTotal returns the total number of hard eval cases.
func (fc *ForgeController) countHardTotal(evalCases []model.ForgeEvalCase) int {
	count := 0
	for _, ec := range evalCases {
		if ec.RequirementType == model.ForgeRequirementHard {
			count++
		}
	}
	return count
}

// buildEvalDesignerMessage constructs the message to send to the eval designer.
// If the agent has integrations, the resolved action schemas are included so the
// eval designer generates mocks that match the real API schemas exactly.
func (fc *ForgeController) buildEvalDesignerMessage(archOutput *ArchitectOutput, agent *model.Agent) string {
	toolsJSON, _ := json.Marshal(archOutput.Tools)
	configJSON, _ := json.Marshal(archOutput.AgentConfig)

	msg := fmt.Sprintf(`Generate a comprehensive test suite for the following agent:

System Prompt:
%s

Tools:
%s

Configuration:
%s`, archOutput.SystemPrompt, string(toolsJSON), string(configJSON))

	// Inject real action schemas from the catalog if the agent has integrations.
	if fc.catalog != nil {
		actions, err := resolveAgentActions(fc.db, fc.catalog, agent)
		if err == nil && len(actions) > 0 {
			msg += "\n\n" + formatActionsForEvalDesigner(actions)
		}
	}

	msg += "\n\nGenerate at least 5 eval cases with diverse categories (happy_path, edge_case, adversarial, tool_error). " +
		"Include realistic tool mocks with multiple samples per tool. " +
		"Classify each eval as basic/standard/adversarial tier and hard/soft requirement type. " +
		"Basic tier evals test fundamental correctness and must always pass. " +
		"Hard requirement evals are pass/fail with no partial credit. " +
		"Soft requirement evals allow partial scores. " +
		"Include deterministic_checks where applicable (tool_called, tool_not_called, tool_order, response_contains, etc.). " +
		"Set sample_count (1-5) for each eval — higher for non-deterministic behaviors."
	return msg
}

// buildJudgeMessage constructs the message for the judge to score an eval.
// Includes deterministic check results, tiered rubric criteria, and sample results.
func (fc *ForgeController) buildJudgeMessage(evalCase *model.ForgeEvalCase, result *model.ForgeEvalResult) string {
	// Parse rubric criteria.
	var rubricCriteria []RubricCriterion
	json.Unmarshal(evalCase.Rubric, &rubricCriteria)

	// Build rubric string with hard/soft classification.
	var hardCriteria, softCriteria []string
	for _, rc := range rubricCriteria {
		entry := fmt.Sprintf("- %s (weight: %.1f)", rc.Criterion, rc.Weight)
		if rc.RequirementType == "hard" {
			hardCriteria = append(hardCriteria, entry)
		} else {
			softCriteria = append(softCriteria, entry)
		}
	}

	rubricStr := ""
	if len(hardCriteria) > 0 {
		rubricStr += "HARD CRITERIA (must all pass — binary pass/fail, no partial credit):\n"
		for _, c := range hardCriteria {
			rubricStr += c + "\n"
		}
	}
	if len(softCriteria) > 0 {
		rubricStr += "SOFT CRITERIA (scored 0.0-1.0, partial credit allowed):\n"
		for _, c := range softCriteria {
			rubricStr += c + "\n"
		}
	}

	// Parse and format sample results.
	var sampleResults []SampleResult
	json.Unmarshal(result.SampleResults, &sampleResults)

	sampleStr := ""
	for _, sr := range sampleResults {
		sampleStr += fmt.Sprintf("\n--- Sample %d ---\nResponse:\n%s\n", sr.SampleIndex, sr.Response)
		if len(sr.ToolCalls) > 0 {
			toolCallsJSON, _ := json.Marshal(sr.ToolCalls)
			sampleStr += fmt.Sprintf("Tool Calls: %s\n", string(toolCallsJSON))
		}
	}

	// Parse and format deterministic check results.
	var detResults []DeterministicResult
	json.Unmarshal(result.DeterministicResults, &detResults)

	detStr := ""
	if len(detResults) > 0 {
		detStr = "DETERMINISTIC CHECK RESULTS (already verified — do not re-evaluate these):\n"
		allDetPassed := true
		for _, dr := range detResults {
			status := "PASS"
			if !dr.Passed {
				status = "FAIL"
				allDetPassed = false
			}
			detStr += fmt.Sprintf("- [%s] %s: %s\n", status, dr.CheckName, dr.Details)
		}
		if !allDetPassed {
			detStr += "\nNote: Some deterministic checks FAILED. These failures are definitive and should be reflected in your scoring.\n"
		}
	}

	return fmt.Sprintf(`Score the following agent response:

Test Name: %s
Category: %s
Tier: %s
Requirement Type: %s

User Input:
%s

Expected Behavior:
%s

%s
%s
Scoring Rubric:
%s
Instructions:
- For HARD criteria: score 1.0 if met, 0.0 if not. The eval passes only if ALL hard criteria are met.
- For SOFT criteria: score 0.0-1.0 based on quality. Partial credit is allowed.
- Deterministic check results above are already verified — incorporate them into your scoring.
- If any deterministic check failed, the corresponding rubric criterion must fail.
- Set failure_category to the most relevant category: safety, correctness, completeness, tone, tool_usage, or none.
- Provide a specific, actionable critique explaining what went wrong and what instruction change would fix it.

Score this response against each rubric criterion.`,
		evalCase.TestName, evalCase.Category, evalCase.Tier, evalCase.RequirementType,
		evalCase.TestPrompt, evalCase.ExpectedBehavior,
		sampleStr, detStr, rubricStr)
}

// loadSystemAgent loads a system agent from the DB by name.
func (fc *ForgeController) loadSystemAgent(name string) (*model.Agent, error) {
	var agent model.Agent
	if err := fc.db.Where("name = ? AND is_system = true AND status = 'active'", name).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("system agent %q not found: %w", name, err)
	}
	return &agent, nil
}

// disableBuiltInTools configures a forge system agent so Bridge registers zero
// built-in tools. The agent can only use its MCP tool (start_forge,
// submit_system_prompt, etc.). Also sets tool_calls_only so the agent is
// forced to call tools rather than generating text.
func disableBuiltInTools(agent *model.Agent) {
	agent.DisableBuiltInTools = true
	if agent.AgentConfig == nil {
		agent.AgentConfig = model.JSON{}
	}
	agent.AgentConfig["tool_calls_only"] = true
}

// ensureSystemAgentReady ensures a system agent has a pool sandbox and is pushed to Bridge.
// Returns the Bridge client for the system agent's sandbox.
func (fc *ForgeController) ensureSystemAgentReady(ctx context.Context, agent *model.Agent) (*bridgepkg.BridgeClient, error) {
	// Disable all built-in tools — forge agents only use their MCP tool.
	disableBuiltInTools(agent)

	// Assign pool sandbox if not already assigned.
	if agent.SandboxID == nil {
		if err := fc.pusher.PushAgent(ctx, agent); err != nil {
			return nil, fmt.Errorf("assigning sandbox for %s: %w", agent.Name, err)
		}
		// Reload to get updated SandboxID.
		fc.db.Where("id = ?", agent.ID).First(agent)
	}

	if agent.SandboxID == nil {
		return nil, fmt.Errorf("system agent %s has no sandbox after assignment", agent.Name)
	}

	// Load sandbox and wake if stopped.
	var sb model.Sandbox
	if err := fc.db.Where("id = ?", *agent.SandboxID).First(&sb).Error; err != nil {
		return nil, fmt.Errorf("loading sandbox for %s: %w", agent.Name, err)
	}
	if sb.Status == "stopped" {
		woken, err := fc.orchestrator.WakeSandbox(ctx, &sb)
		if err != nil {
			return nil, fmt.Errorf("waking sandbox for %s: %w", agent.Name, err)
		}
		sb = *woken
	}

	// Ensure agent is pushed to Bridge (idempotent).
	if err := fc.pusher.PushAgentToSandbox(ctx, agent, &sb); err != nil {
		return nil, fmt.Errorf("pushing %s to bridge: %w", agent.Name, err)
	}

	// Get Bridge client.
	client, err := fc.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		return nil, fmt.Errorf("getting bridge client for %s: %w", agent.Name, err)
	}
	return client, nil
}

// pushEvalTargetToPool pushes a temporary eval-target agent to a pool sandbox.
// Returns the sandbox and Bridge client for eval execution.
func (fc *ForgeController) pushEvalTargetToPool(ctx context.Context, agentID string, def bridgepkg.AgentDefinition) (*model.Sandbox, *bridgepkg.BridgeClient, error) {
	// Find any running pool sandbox.
	var sb model.Sandbox
	if err := fc.db.Where("sandbox_type = 'shared' AND status = 'running'").
		Order("memory_used_bytes ASC").First(&sb).Error; err != nil {
		return nil, nil, fmt.Errorf("no pool sandbox available: %w", err)
	}

	client, err := fc.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		return nil, nil, fmt.Errorf("getting bridge client: %w", err)
	}

	if err := client.UpsertAgent(ctx, agentID, def); err != nil {
		return nil, nil, fmt.Errorf("pushing eval target to bridge: %w", err)
	}

	return &sb, client, nil
}

// mintToken mints a proxy token for a forge agent.
func (fc *ForgeController) mintToken(orgID, credentialID uuid.UUID) (string, string, error) {
	tokenStr, jti, err := token.Mint(fc.signingKey, orgID.String(), credentialID.String(), forgeTokenTTL)
	if err != nil {
		return "", "", err
	}
	return "ptok_" + tokenStr, jti, nil
}

// completeRun marks the forge run as completed with results from the best iteration.
func (fc *ForgeController) completeRun(ctx context.Context, run *model.ForgeRun, best *model.ForgeIteration) {
	now := time.Now()
	updates := map[string]any{
		"status":       model.ForgeStatusCompleted,
		"completed_at": now,
	}

	var finalScore float64
	if best != nil {
		finalScore = best.Score
		updates["final_score"] = finalScore
		updates["result_system_prompt"] = best.SystemPrompt
		updates["result_tools"] = best.Tools
		updates["result_agent_config"] = best.AgentConfig
	}

	fc.db.Model(run).Updates(updates)

	slog.Info("forge: run completed",
		"forge_run_id", run.ID,
		"forge_run_final_score", finalScore,
		"forge_run_total_iterations", run.CurrentIteration,
		"forge_run_stop_reason", run.StopReason,
		"forge_run_has_result", best != nil,
	)

	fc.events.emit(ctx, run.ID, EventRunCompleted, map[string]any{
		"final_score":      finalScore,
		"total_iterations": run.CurrentIteration,
		"stop_reason":      run.StopReason,
	})
}

// failRun marks a forge run as failed.
func (fc *ForgeController) failRun(runID uuid.UUID, errMsg string) {
	slog.Error("forge: run failed", "forge_run_id", runID, "error", errMsg)
	fc.db.Model(&model.ForgeRun{}).Where("id = ?", runID).Updates(map[string]any{
		"status":        model.ForgeStatusFailed,
		"error_message": errMsg,
		"completed_at":  time.Now(),
	})
	fc.events.emit(context.Background(), runID, EventRunFailed, map[string]any{
		"error": errMsg,
	})
}

// cancelRun marks a forge run as cancelled.
func (fc *ForgeController) cancelRun(runID uuid.UUID) {
	slog.Info("forge: run cancelled", "forge_run_id", runID)
	fc.db.Model(&model.ForgeRun{}).Where("id = ?", runID).Updates(map[string]any{
		"status":       model.ForgeStatusCancelled,
		"completed_at": time.Now(),
	})
	fc.events.emit(context.Background(), runID, EventRunCancelled, nil)
}

// updateRunStatus updates the status of a forge run.
func (fc *ForgeController) updateRunStatus(run *model.ForgeRun, status string) {
	fc.db.Model(run).Update("status", status)
	run.Status = status
}

// updateIterPhase updates the phase of a forge iteration.
func (fc *ForgeController) updateIterPhase(iter *model.ForgeIteration, phase string) {
	fc.db.Model(iter).Update("phase", phase)
	iter.Phase = phase
}

// ResumeStaleRuns finds forge runs that were interrupted by server restart
// and marks them as failed (full resume is a future enhancement).
func (fc *ForgeController) ResumeStaleRuns(ctx context.Context) {
	var staleRuns []model.ForgeRun
	fc.db.Where("status IN ?", []string{model.ForgeStatusRunning, model.ForgeStatusProvisioning, model.ForgeStatusDesigningEvals}).Find(&staleRuns)
	for _, run := range staleRuns {
		slog.Warn("marking stale forge run as failed",
			"forge_run_id", run.ID,
			"status", run.Status,
		)
		fc.failRun(run.ID, "server restarted while forge was running")
	}
}
