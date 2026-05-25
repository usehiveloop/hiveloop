package evals

import (
	"time"

	"github.com/google/uuid"
)

const (
	BehaviorDelegate = "delegate"
	BehaviorDirect   = "direct"
	BehaviorClarify  = "clarify"
)

type Suite struct {
	ID             string          `yaml:"id" json:"id"`
	TimeoutSeconds int             `yaml:"timeout_seconds" json:"timeout_seconds"`
	Models         []string        `yaml:"models" json:"models,omitempty"`
	Business       BusinessFixture `yaml:"business" json:"business"`
	Employee       EmployeeFixture `yaml:"employee" json:"employee"`
	Memories       []MemoryFixture `yaml:"memories" json:"memories"`
	Cases          []Case          `yaml:"cases" json:"cases"`
}

type BusinessFixture struct {
	Name     string `yaml:"name" json:"name"`
	Industry string `yaml:"industry" json:"industry"`
	Profile  string `yaml:"profile" json:"profile"`
}

type EmployeeFixture struct {
	Name         string `yaml:"name" json:"name"`
	Role         string `yaml:"role" json:"role"`
	Instructions string `yaml:"instructions" json:"instructions"`
}

type MemoryFixture struct {
	Type       string `yaml:"type" json:"type"`
	Content    string `yaml:"content" json:"content"`
	DocumentID string `yaml:"document_id" json:"document_id,omitempty"`
}

type Case struct {
	ID                 string           `yaml:"id" json:"id"`
	Message            string           `yaml:"message" json:"message"`
	ExpectedInitial    string           `yaml:"expected_initial_behavior" json:"expected_initial_behavior,omitempty"`
	ExpectedBehavior   string           `yaml:"expected_behavior" json:"expected_behavior"`
	ExpectedSpecialist string           `yaml:"expected_specialist" json:"expected_specialist,omitempty"`
	TimeoutSeconds     int              `yaml:"timeout_seconds" json:"timeout_seconds,omitempty"`
	FollowUp           *FollowUpFixture `yaml:"follow_up" json:"follow_up,omitempty"`
	Memories           []MemoryFixture  `yaml:"memories" json:"memories,omitempty"`
	Assertions         CaseAssertions   `yaml:"assertions" json:"assertions,omitempty"`
}

type FollowUpFixture struct {
	Mode    string `yaml:"mode" json:"mode"`
	Context string `yaml:"context" json:"context"`
}

type CaseAssertions struct {
	RequiredToolCalls           []string `yaml:"required_tool_calls" json:"required_tool_calls,omitempty"`
	ForbiddenToolCalls          []string `yaml:"forbidden_tool_calls" json:"forbidden_tool_calls,omitempty"`
	RequiredFinalText           []string `yaml:"required_final_text" json:"required_final_text,omitempty"`
	ForbiddenFinalText          []string `yaml:"forbidden_final_text" json:"forbidden_final_text,omitempty"`
	RequiredBriefContains       []string `yaml:"required_brief_contains" json:"required_brief_contains,omitempty"`
	ForbiddenBriefContains      []string `yaml:"forbidden_brief_contains" json:"forbidden_brief_contains,omitempty"`
	MaxStatusChecksBeforeWake   int      `yaml:"max_status_checks_before_wake" json:"max_status_checks_before_wake,omitempty"`
	ObserveAfterDelegateSeconds int      `yaml:"observe_after_delegate_seconds" json:"observe_after_delegate_seconds,omitempty"`
}

type RunOptions struct {
	SuitePath  string
	Models     []string
	Runs       int
	Parallel   int
	APIURL     string
	OutDir     string
	JudgeModel string
}

type TrialKey struct {
	SuiteID  string `json:"suite_id"`
	Model    string `json:"model"`
	CaseID   string `json:"case_id"`
	RunIndex int    `json:"run_index"`
}

type TrialFixture struct {
	Key        TrialKey  `json:"key"`
	UserID     uuid.UUID `json:"user_id"`
	OrgID      uuid.UUID `json:"org_id"`
	EmployeeID uuid.UUID `json:"employee_id"`
	RouteID    uuid.UUID `json:"route_id"`
	SandboxID  uuid.UUID `json:"sandbox_id"`
	ThreadID   string    `json:"thread_id"`
	MessageID  string    `json:"message_id"`

	JudgeProxyToken string `json:"-"`
	JudgeTokenJTI   string `json:"-"`
}

type GatewayResponse struct {
	Status            string `json:"status"`
	EventID           string `json:"event_id"`
	EmployeeSessionID string `json:"employee_session_id"`
	RuntimeSessionID  string `json:"runtime_session_id"`
	RuntimeStreamID   string `json:"runtime_stream_id"`
	RuntimeTraceID    string `json:"runtime_trace_id"`
	RuntimeTurnID     string `json:"runtime_turn_id"`
}

type TrialResult struct {
	Key       TrialKey        `json:"key"`
	Fixture   TrialFixture    `json:"fixture"`
	Case      Case            `json:"case"`
	Passed    bool            `json:"passed"`
	Reason    string          `json:"reason"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   time.Time       `json:"ended_at"`
	Gateway   GatewayResponse `json:"gateway"`
	Decision  Decision        `json:"decision"`
	Metrics   TrialMetrics    `json:"metrics"`
	Evidence  Evidence        `json:"-"`
	Error     string          `json:"error,omitempty"`
}

type TrialMetrics struct {
	TimeToDecisionMS int64   `json:"time_to_decision_ms"`
	GenerationCount  int64   `json:"generation_count"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	CreditsDebited   int64   `json:"credits_debited"`
}

type Summary struct {
	SuiteID string           `json:"suite_id"`
	Runs    []TrialResult    `json:"runs"`
	Models  []ModelSummary   `json:"models"`
	Overall AggregateSummary `json:"overall"`
}

type AggregateSummary struct {
	TotalCases             int     `json:"total_cases"`
	Passed                 int     `json:"passed"`
	PassRate               float64 `json:"pass_rate"`
	DelegationAccuracy     float64 `json:"delegation_accuracy"`
	CorrectSpecialistRate  float64 `json:"correct_specialist_rate"`
	FalseDelegationRate    float64 `json:"false_delegation_rate"`
	ClarificationAccuracy  float64 `json:"clarification_accuracy"`
	DirectAnswerAccuracy   float64 `json:"direct_answer_accuracy"`
	AverageCostUSD         float64 `json:"average_cost_usd"`
	AverageCreditsDebited  float64 `json:"average_credits_debited"`
	AverageDecisionSeconds float64 `json:"average_decision_seconds"`
}

type ModelSummary struct {
	Model string `json:"model"`
	AggregateSummary
}
