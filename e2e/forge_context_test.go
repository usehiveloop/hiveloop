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

// ─── Context MCP Handler Tests ──────────────────────────────────────────────

func TestForgeContextMCP_StartForgeToolRegistered(t *testing.T) {
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
		Status:                   model.ForgeStatusGatheringContext,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create forge run: %v", err)
	}

	handler := forge.NewForgeContextMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-context/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	// Initialize
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), initBody))
	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	// List tools — should have start_forge
	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), listBody))
	if recorder2.Code != http.StatusOK {
		t.Fatalf("tools/list expected 200, got %d: %s", recorder2.Code, recorder2.Body.String())
	}

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(extractSSEData(t, recorder2.Body.String()), &rpcResp); err != nil {
		t.Fatalf("unmarshal tools/list: %v (body: %s)", err, recorder2.Body.String())
	}

	if len(rpcResp.Result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(rpcResp.Result.Tools))
	}
	if rpcResp.Result.Tools[0].Name != "start_forge" {
		t.Errorf("expected tool name 'start_forge', got %q", rpcResp.Result.Tools[0].Name)
	}
}

func TestForgeContextMCP_WrongStatus_NoTools(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	// Create run in 'running' status — context MCP should return empty server
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
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create forge run: %v", err)
	}

	handler := forge.NewForgeContextMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-context/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), initBody))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), listBody))

	var rpcResp struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(extractSSEData(t, recorder2.Body.String()), &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rpcResp.Result.Tools) != 0 {
		t.Errorf("expected 0 tools for wrong status, got %d", len(rpcResp.Result.Tools))
	}
}

func TestForgeContextMCP_NonexistentRun_NoTools(t *testing.T) {
	db := forgeTestDB(t)

	handler := forge.NewForgeContextMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-context/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	fakeID := uuid.New()
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", fakeID), initBody))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", fakeID), listBody))

	var rpcResp struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(extractSSEData(t, recorder2.Body.String()), &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rpcResp.Result.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(rpcResp.Result.Tools))
	}
}

func TestForgeContextMCP_StartForge_StoresContext(t *testing.T) {
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
		Status:                   model.ForgeStatusGatheringContext,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create forge run: %v", err)
	}

	handler := forge.NewForgeContextMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-context/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	// Initialize
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), initBody))

	// Call start_forge tool
	callBody := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"start_forge","arguments":{"requirements_summary":"A support triage agent","success_criteria":["Routes tickets correctly","Responds helpfully"],"edge_cases":["Angry customer"],"tone_and_style":"Friendly","constraints":["Never share IDs"],"priority_focus":"accuracy"}}}`)
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), callBody))

	if recorder2.Code != http.StatusOK {
		t.Fatalf("tools/call expected 200, got %d: %s", recorder2.Code, recorder2.Body.String())
	}

	// Verify context was stored in DB
	var updated model.ForgeRun
	if err := db.Where("id = ?", run.ID).First(&updated).Error; err != nil {
		t.Fatalf("reload forge run: %v", err)
	}
	if len(updated.Context) == 0 {
		t.Fatal("expected context to be stored, but it's empty")
	}

	var ctx forge.ForgeContext
	if err := json.Unmarshal(updated.Context, &ctx); err != nil {
		t.Fatalf("unmarshal context: %v", err)
	}
	if ctx.RequirementsSummary != "A support triage agent" {
		t.Errorf("expected requirements_summary 'A support triage agent', got %q", ctx.RequirementsSummary)
	}
	if len(ctx.SuccessCriteria) != 2 {
		t.Errorf("expected 2 success criteria, got %d", len(ctx.SuccessCriteria))
	}
	if ctx.PriorityFocus != "accuracy" {
		t.Errorf("expected priority_focus 'accuracy', got %q", ctx.PriorityFocus)
	}
}

func TestForgeContextMCP_StartForge_MissingRequired(t *testing.T) {
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
		Status:                   model.ForgeStatusGatheringContext,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create forge run: %v", err)
	}

	handler := forge.NewForgeContextMCPHandler(db)
	router := chi.NewRouter()
	router.Route("/forge-context/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", handler.StreamableHTTPHandler())
	})

	// Initialize
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), initBody))

	// Call without requirements_summary — should get error
	callBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"start_forge","arguments":{"success_criteria":["test"]}}}`
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge-context/%s/mcp", run.ID), callBody))

	// The tool should return an error result (isError: true), not an HTTP error
	if recorder2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder2.Code)
	}

	// Verify the response indicates an error
	responseData := extractSSEData(t, recorder2.Body.String())
	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responseData, &rpcResp); err == nil && rpcResp.Result.IsError {
		// Good — tool returned error as expected
	} else {
		// If the tool didn't return isError, check that context wasn't stored with empty summary
		var updated model.ForgeRun
		db.Where("id = ?", run.ID).First(&updated)
		if len(updated.Context) > 0 {
			var ctx forge.ForgeContext
			json.Unmarshal(updated.Context, &ctx)
			if ctx.RequirementsSummary == "" {
				t.Error("expected requirements_summary to be required, but context was stored without it")
			}
		}
	}
}

// ─── Eval Review Flow Tests ─────────────────────────────────────────────────

func TestForgeEvalReview_CRUDFlow(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	// Create ForgeRun in reviewing_evals status
	run := model.ForgeRun{
		OrgID:                    ids.orgID,
		AgentID:                  ids.agentID,
		ArchitectCredentialID:    ids.credentialID,
		ArchitectModel:           "gpt-4o",
		EvalDesignerCredentialID: ids.credentialID,
		EvalDesignerModel:        "gpt-4o",
		JudgeCredentialID:        ids.credentialID,
		JudgeModel:               "gpt-4o",
		Status:                   model.ForgeStatusReviewingEvals,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create forge run: %v", err)
	}

	// Create initial eval cases
	for index, name := range []string{"Basic triage", "Security escalation", "Angry customer"} {
		evalCase := model.ForgeEvalCase{
			ForgeRunID:      run.ID,
			TestName:        name,
			Category:        "happy_path",
			Tier:            model.ForgeEvalTierBasic,
			RequirementType: model.ForgeRequirementHard,
			SampleCount:     3,
			TestPrompt:      fmt.Sprintf("Test prompt for %s", name),
			ExpectedBehavior: fmt.Sprintf("Expected behavior for %s", name),
			ToolMocks:       model.RawJSON("{}"),
			Rubric:          model.RawJSON("[]"),
			DeterministicChecks: model.RawJSON("[]"),
			OrderIndex:      index,
		}
		if err := db.Create(&evalCase).Error; err != nil {
			t.Fatalf("create eval case %s: %v", name, err)
		}
	}

	// Verify listing
	var cases []model.ForgeEvalCase
	db.Where("forge_run_id = ?", run.ID).Order("order_index ASC").Find(&cases)
	if len(cases) != 3 {
		t.Fatalf("expected 3 eval cases, got %d", len(cases))
	}
	if cases[0].TestName != "Basic triage" {
		t.Errorf("expected first case 'Basic triage', got %q", cases[0].TestName)
	}

	// Update an eval case
	db.Model(&cases[0]).Updates(map[string]interface{}{
		"test_name":   "Updated triage",
		"sample_count": 5,
	})
	var updated model.ForgeEvalCase
	db.Where("id = ?", cases[0].ID).First(&updated)
	if updated.TestName != "Updated triage" {
		t.Errorf("expected updated name 'Updated triage', got %q", updated.TestName)
	}
	if updated.SampleCount != 5 {
		t.Errorf("expected sample_count 5, got %d", updated.SampleCount)
	}

	// Delete an eval case
	db.Where("id = ?", cases[2].ID).Delete(&model.ForgeEvalCase{})
	var remaining int64
	db.Model(&model.ForgeEvalCase{}).Where("forge_run_id = ?", run.ID).Count(&remaining)
	if remaining != 2 {
		t.Errorf("expected 2 remaining eval cases, got %d", remaining)
	}

	// Create a new eval case
	newCase := model.ForgeEvalCase{
		ForgeRunID:      run.ID,
		TestName:        "New custom eval",
		Category:        "edge_case",
		Tier:            model.ForgeEvalTierStandard,
		RequirementType: model.ForgeRequirementSoft,
		SampleCount:     2,
		TestPrompt:      "Custom test prompt",
		ExpectedBehavior: "Custom expected behavior",
		ToolMocks:       model.RawJSON("{}"),
		Rubric:          model.RawJSON("[]"),
		DeterministicChecks: model.RawJSON("[]"),
		OrderIndex:      10,
	}
	if err := db.Create(&newCase).Error; err != nil {
		t.Fatalf("create new eval case: %v", err)
	}

	db.Model(&model.ForgeEvalCase{}).Where("forge_run_id = ?", run.ID).Count(&remaining)
	if remaining != 3 {
		t.Errorf("expected 3 eval cases after create, got %d", remaining)
	}
}

func TestForgeEvalReview_StatusTransitions(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	t.Run("designing_evals_to_reviewing_evals", func(t *testing.T) {
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
		if err := db.Create(&run).Error; err != nil {
			t.Fatalf("create run: %v", err)
		}

		// Simulate what DesignEvals does
		db.Model(&run).Update("status", model.ForgeStatusReviewingEvals)

		var updated model.ForgeRun
		db.Where("id = ?", run.ID).First(&updated)
		if updated.Status != model.ForgeStatusReviewingEvals {
			t.Errorf("expected reviewing_evals, got %q", updated.Status)
		}
	})

	t.Run("reviewing_evals_to_queued", func(t *testing.T) {
		run := model.ForgeRun{
			OrgID:                    ids.orgID,
			AgentID:                  ids.agentID,
			ArchitectCredentialID:    ids.credentialID,
			ArchitectModel:           "gpt-4o",
			EvalDesignerCredentialID: ids.credentialID,
			EvalDesignerModel:        "gpt-4o",
			JudgeCredentialID:        ids.credentialID,
			JudgeModel:               "gpt-4o",
			Status:                   model.ForgeStatusReviewingEvals,
		}
		if err := db.Create(&run).Error; err != nil {
			t.Fatalf("create run: %v", err)
		}

		// Create at least one eval case
		evalCase := model.ForgeEvalCase{
			ForgeRunID:  run.ID,
			TestName:    "test",
			TestPrompt:  "prompt",
			ToolMocks:   model.RawJSON("{}"),
			Rubric:      model.RawJSON("[]"),
			DeterministicChecks: model.RawJSON("[]"),
		}
		db.Create(&evalCase)

		// Simulate ApproveEvals
		db.Model(&run).Update("status", model.ForgeStatusQueued)

		var updated model.ForgeRun
		db.Where("id = ?", run.ID).First(&updated)
		if updated.Status != model.ForgeStatusQueued {
			t.Errorf("expected queued, got %q", updated.Status)
		}
	})

	t.Run("cancel_from_reviewing_evals", func(t *testing.T) {
		run := model.ForgeRun{
			OrgID:                    ids.orgID,
			AgentID:                  ids.agentID,
			ArchitectCredentialID:    ids.credentialID,
			ArchitectModel:           "gpt-4o",
			EvalDesignerCredentialID: ids.credentialID,
			EvalDesignerModel:        "gpt-4o",
			JudgeCredentialID:        ids.credentialID,
			JudgeModel:               "gpt-4o",
			Status:                   model.ForgeStatusReviewingEvals,
		}
		if err := db.Create(&run).Error; err != nil {
			t.Fatalf("create run: %v", err)
		}

		db.Model(&run).Update("status", model.ForgeStatusCancelled)

		var updated model.ForgeRun
		db.Where("id = ?", run.ID).First(&updated)
		if updated.Status != model.ForgeStatusCancelled {
			t.Errorf("expected cancelled, got %q", updated.Status)
		}
	})
}

func TestForgeEvalReview_PreExistingEvalsSkipDesigning(t *testing.T) {
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
		Status:                   model.ForgeStatusQueued,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}

	// Pre-create eval cases (as DesignEvals would)
	for index, name := range []string{"Eval 1", "Eval 2", "Eval 3"} {
		evalCase := model.ForgeEvalCase{
			ForgeRunID:  run.ID,
			TestName:    name,
			TestPrompt:  fmt.Sprintf("Test prompt %d", index),
			ToolMocks:   model.RawJSON("{}"),
			Rubric:      model.RawJSON("[]"),
			DeterministicChecks: model.RawJSON("[]"),
			OrderIndex:  index,
		}
		if err := db.Create(&evalCase).Error; err != nil {
			t.Fatalf("create eval case: %v", err)
		}
	}

	// Verify eval cases exist
	var cases []model.ForgeEvalCase
	db.Where("forge_run_id = ?", run.ID).Order("order_index ASC").Find(&cases)
	if len(cases) != 3 {
		t.Fatalf("expected 3 pre-existing eval cases, got %d", len(cases))
	}

	// When iteration 1 runs, it should find these and skip eval designing
	// (This is a data setup test — the actual skip logic is in the controller)
	if cases[0].TestName != "Eval 1" {
		t.Errorf("expected first case 'Eval 1', got %q", cases[0].TestName)
	}
	if cases[0].OrderIndex != 0 {
		t.Errorf("expected order_index 0, got %d", cases[0].OrderIndex)
	}
}
