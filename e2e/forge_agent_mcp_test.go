package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/forge"
	"github.com/ziraloop/ziraloop/internal/model"
)

// ─── Architect MCP Tests ────────────────────────────────────────────────────

func TestForgeArchitectMCP_ToolRegistered(t *testing.T) {
	db := forgeTestDB(t)

	handler := forge.NewForgeArchitectMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-architect/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	fakeRunID := uuid.New()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-architect/%s/mcp", fakeRunID), initBody))
	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize expected 200, got %d", recorder.Code)
	}

	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-architect/%s/mcp", fakeRunID), listBody))

	var rpcResp struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(extractSSEData(t, recorder2.Body.String()), &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(rpcResp.Result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(rpcResp.Result.Tools))
	}
	if rpcResp.Result.Tools[0].Name != "submit_system_prompt" {
		t.Errorf("expected tool 'submit_system_prompt', got %q", rpcResp.Result.Tools[0].Name)
	}
}

func TestForgeArchitectMCP_SubmitSavesToDB(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	run := model.ForgeRun{
		OrgID:                    ids.orgID,
		AgentID:                  ids.agentID,
		ArchitectCredentialID:    ids.credentialID,
		ArchitectModel:           "gpt-4o",
		EvalDesignerCredentialID: ids.credentialID,
		EvalDesignerModel:        "gpt-4o",
		JudgeCredentialID:        ids.credentialID,
		JudgeModel:               "gpt-4o",
		Status:                   model.ForgeStatusRunning,
		CurrentIteration:         1,
	}
	db.Create(&run)

	iter := model.ForgeIteration{
		ForgeRunID: run.ID,
		Iteration:  1,
		Phase:      model.ForgePhaseDesigning,
	}
	db.Create(&iter)

	handler := forge.NewForgeArchitectMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-architect/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	// Initialize
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-architect/%s/mcp", run.ID), initBody))

	// Call submit_system_prompt
	callBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"submit_system_prompt","arguments":{"system_prompt":"You are a helpful assistant.","reasoning":"Initial design based on requirements."}}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-architect/%s/mcp", run.ID), callBody))

	if recorder2.Code != http.StatusOK {
		t.Fatalf("tools/call expected 200, got %d: %s", recorder2.Code, recorder2.Body.String())
	}

	// Verify saved to DB
	var updated model.ForgeIteration
	db.Where("id = ?", iter.ID).First(&updated)
	if updated.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("expected system_prompt saved, got %q", updated.SystemPrompt)
	}
	if updated.ArchitectReasoning != "Initial design based on requirements." {
		t.Errorf("expected reasoning saved, got %q", updated.ArchitectReasoning)
	}
}

// ─── Eval Designer MCP Tests ────────────────────────────────────────────────

func TestForgeEvalDesignerMCP_ToolRegistered(t *testing.T) {
	db := forgeTestDB(t)

	handler := forge.NewForgeEvalDesignerMCPHandler(db, nil)
	router := chi.NewRouter()
	router.Route("/forge-eval-designer/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	fakeRunID := uuid.New()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-eval-designer/%s/mcp", fakeRunID), initBody))

	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-eval-designer/%s/mcp", fakeRunID), listBody))

	var rpcResp struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	json.Unmarshal(extractSSEData(t, recorder2.Body.String()), &rpcResp)

	if len(rpcResp.Result.Tools) != 1 || rpcResp.Result.Tools[0].Name != "submit_eval_cases" {
		t.Errorf("expected 1 tool 'submit_eval_cases', got %v", rpcResp.Result.Tools)
	}
}

func TestForgeEvalDesignerMCP_SubmitSavesToDB(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	run := model.ForgeRun{
		OrgID:                    ids.orgID,
		AgentID:                  ids.agentID,
		ArchitectCredentialID:    ids.credentialID,
		ArchitectModel:           "gpt-4o",
		EvalDesignerCredentialID: ids.credentialID,
		EvalDesignerModel:        "gpt-4o",
		JudgeCredentialID:        ids.credentialID,
		JudgeModel:               "gpt-4o",
		Status:                   model.ForgeStatusDesigningEvals,
	}
	db.Create(&run)

	handler := forge.NewForgeEvalDesignerMCPHandler(db, nil)
	router := chi.NewRouter()
	router.Route("/forge-eval-designer/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-eval-designer/%s/mcp", run.ID), initBody))

	callBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"submit_eval_cases","arguments":{"evals":[{"name":"basic_triage","category":"happy_path","tier":"basic","requirement_type":"hard","sample_count":3,"test_prompt":"I need help with billing","expected_behavior":"Route to billing team"},{"name":"angry_customer","category":"adversarial","tier":"adversarial","requirement_type":"soft","sample_count":5,"test_prompt":"This is ridiculous fix it NOW","expected_behavior":"De-escalate and address the issue"}]}}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-eval-designer/%s/mcp", run.ID), callBody))

	if recorder2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder2.Code, recorder2.Body.String())
	}

	// Verify saved to DB
	var cases []model.ForgeEvalCase
	db.Where("forge_run_id = ?", run.ID).Order("order_index ASC").Find(&cases)
	if len(cases) != 2 {
		t.Fatalf("expected 2 eval cases, got %d", len(cases))
	}
	if cases[0].TestName != "basic_triage" {
		t.Errorf("expected first case 'basic_triage', got %q", cases[0].TestName)
	}
	if cases[0].OrderIndex != 0 {
		t.Errorf("expected order_index 0, got %d", cases[0].OrderIndex)
	}
	if cases[1].TestName != "angry_customer" {
		t.Errorf("expected second case 'angry_customer', got %q", cases[1].TestName)
	}
	if cases[1].SampleCount != 5 {
		t.Errorf("expected sample_count 5, got %d", cases[1].SampleCount)
	}
	if cases[1].Tier != "adversarial" {
		t.Errorf("expected tier 'adversarial', got %q", cases[1].Tier)
	}

	// Verify forge run transitioned to reviewing_evals.
	var updatedRun model.ForgeRun
	db.Where("id = ?", run.ID).First(&updatedRun)
	if updatedRun.Status != model.ForgeStatusReviewingEvals {
		t.Errorf("expected status reviewing_evals, got %q", updatedRun.Status)
	}
}

// ─── Judge MCP Tests ────────────────────────────────────────────────────────

func TestForgeJudgeMCP_ToolRegistered(t *testing.T) {
	db := forgeTestDB(t)

	handler := forge.NewForgeJudgeMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-judge/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	fakeRunID := uuid.New()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-judge/%s/mcp", fakeRunID), initBody))

	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-judge/%s/mcp", fakeRunID), listBody))

	var rpcResp struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	json.Unmarshal(extractSSEData(t, recorder2.Body.String()), &rpcResp)

	if len(rpcResp.Result.Tools) != 1 || rpcResp.Result.Tools[0].Name != "submit_score" {
		t.Errorf("expected 1 tool 'submit_score', got %v", rpcResp.Result.Tools)
	}
}

func TestForgeJudgeMCP_SubmitSavesToDB(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	run := model.ForgeRun{
		OrgID:                    ids.orgID,
		AgentID:                  ids.agentID,
		ArchitectCredentialID:    ids.credentialID,
		ArchitectModel:           "gpt-4o",
		EvalDesignerCredentialID: ids.credentialID,
		EvalDesignerModel:        "gpt-4o",
		JudgeCredentialID:        ids.credentialID,
		JudgeModel:               "gpt-4o",
		Status:                   model.ForgeStatusRunning,
		CurrentIteration:         1,
	}
	db.Create(&run)

	iter := model.ForgeIteration{
		ForgeRunID: run.ID,
		Iteration:  1,
		Phase:      model.ForgePhaseJudging,
	}
	db.Create(&iter)

	evalCase := model.ForgeEvalCase{
		ForgeRunID:      run.ID,
		TestName:        "basic_triage",
		TestPrompt:      "Help with billing",
		ToolMocks:       model.RawJSON("{}"),
		Rubric:          model.RawJSON("[]"),
		DeterministicChecks: model.RawJSON("[]"),
	}
	db.Create(&evalCase)

	sampleJSON, _ := json.Marshal([]map[string]any{
		{"sample_index": 0, "response": "I'll route you to billing.", "tool_calls": []any{}, "passed": false, "score": 0},
	})

	evalResult := model.ForgeEvalResult{
		ForgeEvalCaseID:  evalCase.ID,
		ForgeIterationID: iter.ID,
		Status:           model.ForgeEvalJudging,
		SampleResults:    model.RawJSON(sampleJSON),
	}
	db.Create(&evalResult)

	handler := forge.NewForgeJudgeMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-judge/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-judge/%s/mcp", run.ID), initBody))

	callBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"submit_score","arguments":{"score":0.85,"passed":true,"failure_category":"none","critique":"Agent correctly routed billing request.","rubric_scores":[{"criterion":"Routes to correct team","requirement_type":"hard","met":true,"score":1.0,"explanation":"Correctly identified billing team."}]}}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-judge/%s/mcp", run.ID), callBody))

	if recorder2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder2.Code, recorder2.Body.String())
	}

	// Verify saved to DB
	var updated model.ForgeEvalResult
	db.Where("id = ?", evalResult.ID).First(&updated)
	if updated.Score != 0.85 {
		t.Errorf("expected score 0.85, got %f", updated.Score)
	}
	if !updated.Passed {
		t.Error("expected passed=true")
	}
	if updated.FailureCategory != "none" {
		t.Errorf("expected failure_category 'none', got %q", updated.FailureCategory)
	}
	if updated.Critique != "Agent correctly routed billing request." {
		t.Errorf("expected critique saved, got %q", updated.Critique)
	}
	if updated.Status != model.ForgeEvalCompleted {
		t.Errorf("expected status '%s', got %q", model.ForgeEvalCompleted, updated.Status)
	}
	if len(updated.RubricScores) == 0 {
		t.Error("expected rubric_scores to be saved")
	}
}

func TestForgeJudgeMCP_NoJudgingEval_ReturnsError(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	run := model.ForgeRun{
		OrgID:                    ids.orgID,
		AgentID:                  ids.agentID,
		ArchitectCredentialID:    ids.credentialID,
		ArchitectModel:           "gpt-4o",
		EvalDesignerCredentialID: ids.credentialID,
		EvalDesignerModel:        "gpt-4o",
		JudgeCredentialID:        ids.credentialID,
		JudgeModel:               "gpt-4o",
		Status:                   model.ForgeStatusRunning,
		CurrentIteration:         1,
	}
	db.Create(&run)

	iter := model.ForgeIteration{
		ForgeRunID: run.ID,
		Iteration:  1,
		Phase:      model.ForgePhaseJudging,
	}
	db.Create(&iter)

	// No eval result in judging status — should error

	handler := forge.NewForgeJudgeMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-judge/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-judge/%s/mcp", run.ID), initBody))

	callBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"submit_score","arguments":{"score":0.5,"passed":false,"failure_category":"correctness","critique":"Test critique","rubric_scores":[]}}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-judge/%s/mcp", run.ID), callBody))

	if recorder2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder2.Code)
	}

	// The tool call should return isError=true
	responseData := extractSSEData(t, recorder2.Body.String())
	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responseData, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !rpcResp.Result.IsError {
		t.Error("expected isError=true when no eval in judging status")
	}
}
