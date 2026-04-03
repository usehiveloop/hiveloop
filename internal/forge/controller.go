package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/llmvault/llmvault/internal/bridge"
	"github.com/llmvault/llmvault/internal/config"
	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/streaming"
	"github.com/llmvault/llmvault/internal/token"
)

const (
	defaultMaxConcurrent = 20
	forgeTokenTTL        = 24 * time.Hour
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

// SandboxCreator abstracts sandbox provisioning so tests can inject a mock.
// The real *sandbox.Orchestrator satisfies this interface.
type SandboxCreator interface {
	CreateForgeSandbox(ctx context.Context, org *model.Org, identityID, forgeRunID uuid.UUID) (*model.Sandbox, error)
	GetBridgeClient(ctx context.Context, sb *model.Sandbox) (*bridgepkg.BridgeClient, error)
}

// ForgeController orchestrates forge runs — a persistent state machine
// that manages the design→eval→judge iteration loop.
type ForgeController struct {
	db           *gorm.DB
	orchestrator SandboxCreator
	catalog      *catalog.Catalog
	signingKey   []byte
	cfg          *config.Config
	eventBus     *streaming.EventBus
	reader       *BridgeReader
	events       *eventEmitter
	sem          chan struct{} // bounded concurrency semaphore
	mu           sync.RWMutex
	activeRuns   map[uuid.UUID]context.CancelFunc
}

// NewForgeController creates a forge controller.
func NewForgeController(
	db *gorm.DB,
	orchestrator SandboxCreator,
	signingKey []byte,
	cfg *config.Config,
	eventBus *streaming.EventBus,
	cat *catalog.Catalog,
) *ForgeController {
	return &ForgeController{
		db:           db,
		orchestrator: orchestrator,
		catalog:      cat,
		signingKey:   signingKey,
		cfg:          cfg,
		eventBus:     eventBus,
		reader:       &BridgeReader{},
		events:       &eventEmitter{db: db, eventBus: eventBus},
		sem:          make(chan struct{}, defaultMaxConcurrent),
		activeRuns:   make(map[uuid.UUID]context.CancelFunc),
	}
}

// Start submits a forge run to the worker pool. Non-blocking.
func (fc *ForgeController) Start(runID uuid.UUID) {
	go func() {
		fc.sem <- struct{}{}        // acquire slot (blocks if pool full)
		defer func() { <-fc.sem }() // release slot

		ctx, cancel := context.WithCancel(context.Background())
		fc.mu.Lock()
		fc.activeRuns[runID] = cancel
		fc.mu.Unlock()
		defer func() {
			cancel()
			fc.mu.Lock()
			delete(fc.activeRuns, runID)
			fc.mu.Unlock()
		}()

		fc.run(ctx, runID)
	}()
}

// Cancel cancels a running forge.
func (fc *ForgeController) Cancel(runID uuid.UUID) bool {
	fc.mu.RLock()
	cancel, ok := fc.activeRuns[runID]
	fc.mu.RUnlock()
	if ok {
		cancel()
		return true
	}
	return false
}

// run is the main forge orchestration loop.
func (fc *ForgeController) run(ctx context.Context, runID uuid.UUID) {
	log := slog.With("forge_run_id", runID)

	// Recover from panics.
	defer func() {
		if r := recover(); r != nil {
			log.Error("forge run panicked", "panic", r)
			fc.failRun(runID, fmt.Sprintf("panic: %v", r))
		}
	}()

	// Load the forge run.
	var run model.ForgeRun
	if err := fc.db.Preload("Agent").Where("id = ?", runID).First(&run).Error; err != nil {
		log.Error("failed to load forge run", "error", err)
		return
	}

	// Load the 3 credentials.
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
	if err := fc.db.Where("id = ?", run.Agent.CredentialID).First(&targetCred).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading target credential: %v", err))
		return
	}

	// Phase: PROVISIONING
	fc.updateRunStatus(&run, model.ForgeStatusProvisioning)

	// Load org for sandbox creation.
	var org model.Org
	if err := fc.db.Where("id = ?", run.OrgID).First(&org).Error; err != nil {
		fc.failRun(runID, fmt.Sprintf("loading org: %v", err))
		return
	}

	// Create forge sandbox.
	sb, err := fc.orchestrator.CreateForgeSandbox(ctx, &org, run.Agent.IdentityID, run.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("creating forge sandbox: %v", err))
		return
	}
	run.SandboxID = &sb.ID
	fc.db.Model(&run).Update("sandbox_id", sb.ID)

	// Get Bridge client.
	client, err := fc.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("getting bridge client: %v", err))
		return
	}

	// Mint proxy tokens for forge agents.
	archToken, archJTI, err := fc.mintToken(run.OrgID, archCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting architect token: %v", err))
		return
	}
	evalToken, evalJTI, err := fc.mintToken(run.OrgID, evalCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting eval designer token: %v", err))
		return
	}
	judgeToken, judgeJTI, err := fc.mintToken(run.OrgID, judgeCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting judge token: %v", err))
		return
	}

	// Mint eval target token (for direct proxy calls).
	evalTargetToken, evalTargetJTI, err := fc.mintToken(run.OrgID, targetCred.ID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("minting eval target token: %v", err))
		return
	}

	// Store token JTIs and agent IDs for resume.
	targetProviderID := targetCred.ProviderID
	archAgentID := uuid.New().String()
	evalAgentID := uuid.New().String()
	judgeAgentID := uuid.New().String()

	fc.db.Model(&run).Updates(map[string]any{
		"architect_token_jti":     archJTI,
		"eval_designer_token_jti": evalJTI,
		"judge_token_jti":         judgeJTI,
		"eval_target_token_jti":   evalTargetJTI,
		"architect_agent_id":      archAgentID,
		"eval_designer_agent_id":  evalAgentID,
		"judge_agent_id":          judgeAgentID,
	})

	// Build and push forge agent definitions.
	if err := fc.pushForgeAgents(ctx, client, targetProviderID, &run,
		archAgentID, evalAgentID, judgeAgentID,
		archToken, evalToken, judgeToken,
		archCred, evalCred, judgeCred,
	); err != nil {
		fc.failRun(runID, fmt.Sprintf("pushing forge agents: %v", err))
		return
	}

	now := time.Now()
	fc.db.Model(&run).Updates(map[string]any{
		"status":     model.ForgeStatusRunning,
		"started_at": now,
	})
	run.Status = model.ForgeStatusRunning
	run.StartedAt = &now
	fc.events.emit(ctx, runID, EventProvisioned, map[string]any{
		"sandbox_id": sb.ID,
	})

	// Create architect conversation (reused across iterations).
	archConv, err := client.CreateConversation(ctx, archAgentID)
	if err != nil {
		fc.failRun(runID, fmt.Sprintf("creating architect conversation: %v", err))
		return
	}
	run.ArchitectConversationID = archConv.ConversationId
	fc.db.Model(&run).Update("architect_conversation_id", archConv.ConversationId)

	// ITERATION LOOP
	var bestScore float64 = -1
	var bestIteration *model.ForgeIteration
	for i := 1; i <= run.MaxIterations; i++ {
		if ctx.Err() != nil {
			fc.cancelRun(runID)
			return
		}

		run.CurrentIteration = i
		fc.db.Model(&run).Update("current_iteration", i)
		fc.events.emit(ctx, runID, EventIterationStarted, map[string]any{
			"iteration": i,
		})

		iter, err := fc.runIteration(ctx, &run, i, client,
			evalAgentID, judgeAgentID,
			targetProviderID, evalTargetToken,
		)
		if err != nil {
			log.Error("iteration failed", "iteration", i, "error", err)
			// Continue to next iteration on non-fatal errors.
			if ctx.Err() != nil {
				fc.cancelRun(runID)
				return
			}
			continue
		}

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
	fc.completeRun(ctx, &run, bestIteration)
}

// runIteration executes a single design→eval→judge cycle.
func (fc *ForgeController) runIteration(
	ctx context.Context, run *model.ForgeRun, iteration int,
	client *bridgepkg.BridgeClient,
	evalAgentID, judgeAgentID string,
	targetProviderID, evalTargetToken string,
) (*model.ForgeIteration, error) {
	log := slog.With("forge_run_id", run.ID, "iteration", iteration)

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
	fc.events.emit(ctx, run.ID, EventArchitectStarted, map[string]any{"iteration": iteration})

	archMessage := fc.buildArchitectMessage(run, iteration)
	archResponse, err := fc.reader.ReadFullResponse(ctx, client, run.ArchitectConversationID, archMessage)
	if err != nil {
		fc.updateIterPhase(&iter, model.ForgePhaseFailed)
		return nil, fmt.Errorf("architect response: %w", err)
	}

	archOutput, err := ParseArchitectOutput(archResponse)
	if err != nil {
		log.Warn("architect returned invalid JSON, retrying", "error", err)
		// Retry with corrective message.
		archResponse, err = fc.reader.ReadFullResponse(ctx, client, run.ArchitectConversationID,
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

	fc.events.emit(ctx, run.ID, EventArchitectCompleted, map[string]any{
		"iteration":         iteration,
		"system_prompt_len": len(archOutput.SystemPrompt),
		"tools_count":       len(archOutput.Tools),
	})

	// PHASE: EVAL_DESIGNING — only in iteration 1.
	// In subsequent iterations, reuse ForgeEvalCase records from the run.
	var evalCases []model.ForgeEvalCase

	if iteration == 1 {
		fc.events.emit(ctx, run.ID, EventEvalDesignStarted, map[string]any{"iteration": iteration})

		evalConv, err := client.CreateConversation(ctx, evalAgentID)
		if err != nil {
			fc.updateIterPhase(&iter, model.ForgePhaseFailed)
			return nil, fmt.Errorf("creating eval designer conversation: %w", err)
		}

		evalMessage := fc.buildEvalDesignerMessage(archOutput, &run.Agent)
		evalResponse, err := fc.reader.ReadFullResponse(ctx, client, evalConv.ConversationId, evalMessage)
		if err != nil {
			fc.updateIterPhase(&iter, model.ForgePhaseFailed)
			return nil, fmt.Errorf("eval designer response: %w", err)
		}

		evalOutput, err := ParseEvalDesignerOutput(evalResponse)
		if err != nil {
			log.Warn("eval designer returned invalid JSON, retrying", "error", err)
			evalResponse, err = fc.reader.ReadFullResponse(ctx, client, evalConv.ConversationId,
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

		fc.events.emit(ctx, run.ID, EventEvalsGenerated, map[string]any{
			"iteration": iteration,
			"count":     len(evalOutput.Evals),
		})

		// End eval designer conversation (no longer needed).
		_ = client.EndConversation(ctx, evalConv.ConversationId)
	} else {
		// Iterations 2+: load existing ForgeEvalCase records from the run.
		fc.db.Where("forge_run_id = ?", run.ID).Find(&evalCases)

		fc.db.Model(&iter).Update("phase", model.ForgePhaseEvaluating)
		iter.Phase = model.ForgePhaseEvaluating
	}

	// PHASE: EVALUATING — use Bridge with forge MCP server for tool mocking.
	// Push a temporary eval-target agent to Bridge with the architect's system prompt
	// and the forge MCP server for tool call mocking.
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
	}

	if err := client.UpsertAgent(ctx, evalTargetAgentID, evalTargetDef); err != nil {
		fc.updateIterPhase(&iter, model.ForgePhaseFailed)
		return nil, fmt.Errorf("pushing eval target agent: %w", err)
	}
	defer func() {
		_ = client.RemoveAgentDefinition(ctx, evalTargetAgentID)
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
			evalConvResp, err := client.CreateConversation(ctx, evalTargetAgentID)
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

			bridgeResp, err := fc.reader.ReadFullResponseWithTools(ctx, client, evalConvResp.ConversationId, evalCase.TestPrompt)
			_ = client.EndConversation(ctx, evalConvResp.ConversationId)

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

	fc.db.Model(&iter).Update("phase", model.ForgePhaseJudging)
	iter.Phase = model.ForgePhaseJudging

	// PHASE: JUDGING
	// Reload eval results with judging status.
	var judgingResults []model.ForgeEvalResult
	fc.db.Where("forge_iteration_id = ? AND status = ?", iter.ID, model.ForgeEvalJudging).Find(&judgingResults)

	// Create a judge conversation for this iteration.
	judgeConv, err := client.CreateConversation(ctx, judgeAgentID)
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
		judgeResponse, err := fc.reader.ReadFullResponse(ctx, client, judgeConv.ConversationId, judgeMessage)
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

	_ = client.EndConversation(ctx, judgeConv.ConversationId)

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
	fc.db.Where("forge_run_id = ?", run.ID).Find(&evalCases)

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

// pushForgeAgents builds and pushes 3 ephemeral agent definitions to Bridge.
func (fc *ForgeController) pushForgeAgents(
	ctx context.Context, client *bridgepkg.BridgeClient,
	targetProviderID string, run *model.ForgeRun,
	archAgentID, evalAgentID, judgeAgentID string,
	archToken, evalToken, judgeToken string,
	archCred, evalCred, judgeCred model.Credential,
) error {
	proxyBaseURL := fmt.Sprintf("https://%s/v1/proxy", fc.cfg.BridgeHost)

	// Helper to build an agent definition for a forge agent.
	buildDef := func(agentID, name, systemPrompt, model, proxyToken string, cred model.Credential, schema map[string]any) bridgepkg.AgentDefinition {
		providerType := bridgepkg.Custom
		if pt, ok := providerTypeMap[cred.ProviderID]; ok {
			providerType = pt
		}
		def := bridgepkg.AgentDefinition{
			Id:           agentID,
			Name:         name,
			SystemPrompt: systemPrompt,
			Provider: bridgepkg.ProviderConfig{
				ProviderType: providerType,
				Model:        model,
				ApiKey:       proxyToken,
				BaseUrl:      &proxyBaseURL,
			},
		}
		if schema != nil {
			cfg := &bridgepkg.AgentConfig{
				JsonSchema: schema,
			}
			def.Config = cfg
		}
		return def
	}

	// Load system prompts.
	archPrompt, err := LoadSystemPrompt(RoleArchitect, targetProviderID)
	if err != nil {
		return fmt.Errorf("loading architect prompt: %w", err)
	}
	evalPrompt, err := LoadSystemPrompt(RoleEvalDesigner, targetProviderID)
	if err != nil {
		return fmt.Errorf("loading eval designer prompt: %w", err)
	}
	judgePrompt, err := LoadSystemPrompt(RoleJudge, "")
	if err != nil {
		return fmt.Errorf("loading judge prompt: %w", err)
	}

	// Build and push all 3 agent definitions.
	agents := []bridgepkg.AgentDefinition{
		buildDef(archAgentID, "forge-architect", archPrompt, run.ArchitectModel, archToken, archCred, ArchitectSchema()),
		buildDef(evalAgentID, "forge-eval-designer", evalPrompt, run.EvalDesignerModel, evalToken, evalCred, EvalDesignerSchema()),
		buildDef(judgeAgentID, "forge-judge", judgePrompt, run.JudgeModel, judgeToken, judgeCred, JudgeSchema()),
	}

	if err := client.PushAgents(ctx, agents); err != nil {
		return fmt.Errorf("pushing agents to bridge: %w", err)
	}
	return nil
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

	if best != nil {
		score := best.Score
		updates["final_score"] = score
		updates["result_system_prompt"] = best.SystemPrompt
		updates["result_tools"] = best.Tools
		updates["result_agent_config"] = best.AgentConfig
	}

	fc.db.Model(run).Updates(updates)

	var finalScore float64
	if best != nil {
		finalScore = best.Score
	}
	fc.events.emit(ctx, run.ID, EventRunCompleted, map[string]any{
		"final_score":      finalScore,
		"total_iterations": run.CurrentIteration,
		"stop_reason":      run.StopReason,
	})
}

// failRun marks a forge run as failed.
func (fc *ForgeController) failRun(runID uuid.UUID, errMsg string) {
	slog.Error("forge run failed", "forge_run_id", runID, "error", errMsg)
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
	fc.db.Where("status IN ?", []string{model.ForgeStatusRunning, model.ForgeStatusProvisioning}).Find(&staleRuns)
	for _, run := range staleRuns {
		slog.Warn("marking stale forge run as failed",
			"forge_run_id", run.ID,
			"status", run.Status,
		)
		fc.failRun(run.ID, "server restarted while forge was running")
	}
}
