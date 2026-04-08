package model

import (
	"time"

	"github.com/google/uuid"
)

// Forge status constants.
// Flow: gathering_context → designing_evals → reviewing_evals → queued → provisioning → running → completed|failed|cancelled.
const (
	ForgeStatusGatheringContext = "gathering_context"
	ForgeStatusDesigningEvals   = "designing_evals"
	ForgeStatusReviewingEvals   = "reviewing_evals"
	ForgeStatusQueued           = "queued"
	ForgeStatusProvisioning     = "provisioning"
	ForgeStatusRunning          = "running"
	ForgeStatusCompleted        = "completed"
	ForgeStatusFailed           = "failed"
	ForgeStatusCancelled        = "cancelled"
)

// Forge iteration phase constants.
const (
	ForgePhaseDesigning     = "designing"
	ForgePhaseEvalDesigning = "eval_designing"
	ForgePhaseEvaluating    = "evaluating"
	ForgePhaseJudging       = "judging"
	ForgePhaseCompleted     = "completed"
	ForgePhaseFailed        = "failed"
)

// Forge eval result status constants.
const (
	ForgeEvalPending   = "pending"
	ForgeEvalRunning   = "running"
	ForgeEvalJudging   = "judging"
	ForgeEvalCompleted = "completed"
	ForgeEvalFailed    = "failed"
)

// Forge stop reason constants.
const (
	ForgeStopThresholdMet  = "threshold_met"
	ForgeStopConverged     = "converged"
	ForgeStopMaxIterations = "max_iterations"
)

// Forge eval tier constants.
const (
	ForgeEvalTierBasic       = "basic"
	ForgeEvalTierStandard    = "standard"
	ForgeEvalTierAdversarial = "adversarial"
)

// Forge requirement type constants.
const (
	ForgeRequirementHard = "hard"
	ForgeRequirementSoft = "soft"
)

// ForgeRun represents a complete forge execution — the top-level record
// and source of truth for forge state. Persisted at every phase transition
// for resumability after server restarts.
type ForgeRun struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrgID   uuid.UUID `gorm:"type:uuid;not null;index:idx_forge_run_org" json:"org_id"`
	Org     Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE" json:"-"`
	AgentID uuid.UUID `gorm:"type:uuid;not null;index:idx_forge_run_agent" json:"agent_id"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`

	// Forge agent configuration (user-selected credentials + models).
	ArchitectCredentialID    uuid.UUID  `gorm:"type:uuid;not null" json:"architect_credential_id"`
	ArchitectCredential      Credential `gorm:"foreignKey:ArchitectCredentialID;constraint:OnDelete:RESTRICT" json:"-"`
	ArchitectModel           string     `gorm:"not null" json:"architect_model"`
	EvalDesignerCredentialID uuid.UUID  `gorm:"type:uuid;not null" json:"eval_designer_credential_id"`
	EvalDesignerCredential   Credential `gorm:"foreignKey:EvalDesignerCredentialID;constraint:OnDelete:RESTRICT" json:"-"`
	EvalDesignerModel        string     `gorm:"not null" json:"eval_designer_model"`
	JudgeCredentialID        uuid.UUID  `gorm:"type:uuid;not null" json:"judge_credential_id"`
	JudgeCredential          Credential `gorm:"foreignKey:JudgeCredentialID;constraint:OnDelete:RESTRICT" json:"-"`
	JudgeModel               string     `gorm:"not null" json:"judge_model"`

	// Control parameters.
	MaxIterations    int     `gorm:"not null;default:5" json:"max_iterations"`
	PassThreshold    float64 `gorm:"type:numeric(5,2);not null;default:0.80" json:"pass_threshold"`
	ConvergenceLimit int     `gorm:"not null;default:3" json:"convergence_limit"` // stop after N stagnant iterations

	// Context gathering — conversational requirements before forge runs.
	Context                 RawJSON    `gorm:"type:jsonb" json:"context,omitempty"`
	ContextConversationID   *uuid.UUID `gorm:"type:uuid" json:"context_conversation_id,omitempty"`
	ContextGathererAgentID  string     `gorm:"default:''" json:"-"`
	ContextGathererTokenJTI string     `gorm:"default:''" json:"-"`

	// State machine: gathering_context → designing_evals → reviewing_evals → queued → provisioning → running → completed|failed|cancelled.
	Status           string   `gorm:"not null;default:'queued'" json:"status"`
	CurrentIteration int      `gorm:"not null;default:0" json:"current_iteration"`
	FinalScore       *float64 `gorm:"type:numeric(5,2)" json:"final_score,omitempty"`

	// Convergence tracking.
	ConvergenceCount int    `gorm:"not null;default:0" json:"-"`                  // consecutive iterations with no improvement
	StopReason       string `gorm:"default:''" json:"stop_reason,omitempty"` // threshold_met, converged, max_iterations

	// Result — best iteration output, applied to agent via /apply endpoint.
	ResultSystemPrompt *string `gorm:"type:text" json:"result_system_prompt,omitempty"`
	ResultTools        RawJSON `gorm:"type:jsonb" json:"result_tools,omitempty"`
	ResultAgentConfig  RawJSON `gorm:"type:jsonb" json:"result_agent_config,omitempty"`

	// Infrastructure — persisted for resumability.
	SandboxID               *uuid.UUID `gorm:"type:uuid" json:"sandbox_id,omitempty"`
	Sandbox                 *Sandbox   `gorm:"foreignKey:SandboxID;constraint:OnDelete:SET NULL" json:"-"`
	ArchitectConversationID string     `gorm:"default:''" json:"-"`
	ArchitectAgentID        string     `gorm:"default:''" json:"-"`
	EvalDesignerAgentID     string     `gorm:"default:''" json:"-"`
	JudgeAgentID            string     `gorm:"default:''" json:"-"`

	// Proxy tokens for forge agents — persisted for resume + cost attribution.
	ArchitectTokenJTI    string `gorm:"default:''" json:"-"`
	EvalDesignerTokenJTI string `gorm:"default:''" json:"-"`
	JudgeTokenJTI        string `gorm:"default:''" json:"-"`
	EvalTargetTokenJTI   string `gorm:"default:''" json:"-"` // token for target agent eval calls

	// Cost aggregated across all LLM calls in this forge run.
	TotalInputTokens  int     `gorm:"default:0" json:"total_input_tokens"`
	TotalOutputTokens int     `gorm:"default:0" json:"total_output_tokens"`
	TotalCost         float64 `gorm:"type:numeric(12,8);default:0" json:"total_cost"`

	// Asynq task ID for cancellation support.
	AsynqTaskID string `gorm:"default:''" json:"-"`

	ErrorMessage *string    `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (ForgeRun) TableName() string { return "forge_runs" }

// ForgeIteration represents one design→eval→judge cycle within a forge run.
// The Phase field tracks progress for resumability.
type ForgeIteration struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ForgeRunID uuid.UUID `gorm:"type:uuid;not null;index:idx_forge_iter_run" json:"forge_run_id"`
	ForgeRun   ForgeRun  `gorm:"foreignKey:ForgeRunID;constraint:OnDelete:CASCADE" json:"-"`
	Iteration  int       `gorm:"not null" json:"iteration"`

	// Phase within this iteration: designing → eval_designing → evaluating → judging → completed|failed.
	Phase string `gorm:"not null;default:'designing'" json:"phase"`

	// Architect output — persisted after designing phase.
	SystemPrompt       string  `gorm:"type:text" json:"system_prompt,omitempty"`
	Tools              RawJSON `gorm:"type:jsonb" json:"tools,omitempty"`
	AgentConfig        RawJSON `gorm:"type:jsonb" json:"agent_config,omitempty"`
	ArchitectReasoning string  `gorm:"type:text" json:"architect_reasoning,omitempty"`
	ArchitectResponse  string  `gorm:"type:text" json:"-"` // raw response for observability

	// Eval designer response — persisted after eval_designing phase (iteration 1 only).
	EvalDesignerResponse string `gorm:"type:text" json:"-"`

	// Results — persisted after judging phase.
	TotalEvals  int     `gorm:"default:0" json:"total_evals"`
	PassedEvals int     `gorm:"default:0" json:"passed_evals"`
	Score       float64 `gorm:"type:numeric(5,2);default:0" json:"score"`

	// Hard vs soft requirement scoring.
	HardScore     float64 `gorm:"type:numeric(5,2);default:0" json:"hard_score"`     // proportion of hard evals passed (must be 1.0)
	SoftScore     float64 `gorm:"type:numeric(5,2);default:0" json:"soft_score"`     // average score of soft evals
	AllHardPassed bool    `gorm:"default:false" json:"all_hard_passed"`              // convenience flag

	// Per-eval score tracking across iterations for regression detection.
	EvalScoreHistory RawJSON `gorm:"type:jsonb" json:"eval_score_history,omitempty"` // [{eval_name, scores: [0.6, 0.8, 1.0]}]

	// Cost for this iteration.
	InputTokens  int     `gorm:"default:0" json:"input_tokens"`
	OutputTokens int     `gorm:"default:0" json:"output_tokens"`
	Cost         float64 `gorm:"type:numeric(12,8);default:0" json:"cost"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ForgeIteration) TableName() string { return "forge_iterations" }

// ForgeEvalCase is a test case definition, created once per forge run during
// the first iteration's eval_designing phase. Reused across all iterations.
type ForgeEvalCase struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ForgeRunID uuid.UUID `gorm:"type:uuid;not null;index:idx_forge_eval_case_run" json:"forge_run_id"`
	ForgeRun   ForgeRun  `gorm:"foreignKey:ForgeRunID;constraint:OnDelete:CASCADE" json:"-"`

	// Test definition.
	TestName         string  `gorm:"not null" json:"test_name"`
	Description      string  `gorm:"type:text" json:"description"`
	Category         string  `json:"category"`                                          // happy_path, edge_case, adversarial, tool_error
	Tier             string  `gorm:"not null;default:'standard'" json:"tier"`            // basic, standard, adversarial
	RequirementType  string  `gorm:"not null;default:'soft'" json:"requirement_type"`    // hard, soft
	SampleCount      int     `gorm:"not null;default:3" json:"sample_count"`             // how many times to run (1-5)
	TestPrompt       string  `gorm:"type:text;not null" json:"test_prompt"`
	ExpectedBehavior string  `gorm:"type:text" json:"expected_behavior"`
	ToolMocks        RawJSON `gorm:"type:jsonb;default:'{}'" json:"tool_mocks"`          // {tool_name: [{match, response}]}
	Rubric           RawJSON `gorm:"type:jsonb;default:'[]'" json:"rubric"`              // []RubricCriterion
	DeterministicChecks RawJSON `gorm:"type:jsonb;default:'[]'" json:"deterministic_checks"` // []DeterministicCheck

	OrderIndex int       `gorm:"not null;default:0" json:"order_index"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ForgeEvalCase) TableName() string { return "forge_eval_cases" }

// ForgeEvalResult stores the result of running an eval case in a specific iteration.
// Created per eval case per iteration. Multiple samples may be run per eval.
type ForgeEvalResult struct {
	ID               uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ForgeEvalCaseID  uuid.UUID     `gorm:"type:uuid;not null;index:idx_forge_eval_result_case" json:"forge_eval_case_id"`
	ForgeEvalCase    ForgeEvalCase `gorm:"foreignKey:ForgeEvalCaseID;constraint:OnDelete:CASCADE" json:"-"`
	ForgeIterationID uuid.UUID     `gorm:"type:uuid;not null;index:idx_forge_eval_result_iter" json:"forge_iteration_id"`
	ForgeIteration   ForgeIteration `gorm:"foreignKey:ForgeIterationID;constraint:OnDelete:CASCADE" json:"-"`

	// Multi-sample results.
	PassRate      float64 `gorm:"type:numeric(5,2);default:0" json:"pass_rate"`    // proportion of samples that passed
	SampleResults RawJSON `gorm:"type:jsonb;default:'[]'" json:"sample_results"`   // [{sample_index, response, tool_calls, passed, score}]

	// Deterministic check results (run before LLM judge).
	DeterministicResults RawJSON `gorm:"type:jsonb;default:'[]'" json:"deterministic_results"` // [{check_name, passed, details}]

	// Judge verdict (from LLM judge, after deterministic checks).
	Score           float64 `gorm:"type:numeric(5,2);default:0" json:"score"`
	Passed          bool    `gorm:"default:false" json:"passed"`
	FailureCategory string  `gorm:"default:''" json:"failure_category,omitempty"` // safety, correctness, completeness, tone, tool_usage, none
	Critique        string  `gorm:"type:text" json:"critique,omitempty"`          // actionable, specific failure explanation
	RubricScores    RawJSON `gorm:"type:jsonb;default:'[]'" json:"rubric_scores"` // [{criterion, requirement_type, met, score, explanation}]

	// Status tracks result progress: pending → running → judging → completed|failed.
	Status string `gorm:"not null;default:'pending'" json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ForgeEvalResult) TableName() string { return "forge_eval_results" }

// ForgeEvent is an audit trail entry for a forge run. Every significant step
// is recorded here for full observability and dashboard rendering.
type ForgeEvent struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ForgeRunID uuid.UUID `gorm:"type:uuid;not null;index:idx_forge_event_run_created" json:"forge_run_id"`
	EventType  string    `gorm:"not null" json:"event_type"`
	Payload    RawJSON   `gorm:"type:jsonb" json:"payload"`
	CreatedAt  time.Time `gorm:"not null;index:idx_forge_event_run_created" json:"created_at"`
}

func (ForgeEvent) TableName() string { return "forge_events" }
