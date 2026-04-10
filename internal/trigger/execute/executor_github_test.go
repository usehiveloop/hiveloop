package execute

import (
	"errors"
	"strings"
	"testing"

	"github.com/ziraloop/ziraloop/internal/model"
)

// End-to-end GitHub executor tests.
//
// Each test:
//   1. Builds a trigger config (conditions, context actions, instructions)
//   2. Loads a real webhook fixture from ../dispatch/testdata/github/
//   3. Stubs Nango responses for the context actions we expect
//   4. Runs the full pipeline (dispatcher + executor) and asserts on the
//      final instruction string
//
// The executor is configured with the FakeNangoProxy — no real Nango calls.
// Everything else (dispatcher, catalog, ref extraction, template
// substitution, context request building, step result threading) runs
// production code paths.

// --- Test 1: PR opened → full flow with all context resolved ---
//
// The big happy-path test. A PR review agent listens for pull_request.opened,
// fetches the PR + files + CONTRIBUTING.md, and assembles an instruction
// with every placeholder substituted. After the executor runs, the
// instruction should contain: the PR title and body, the list of files with
// their filenames, and the contributing guidelines (decoded from the
// repos_get_content response).

func TestExecute_GitHub_PROpened_FullFlow(t *testing.T) {
	harness := newGitHubPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "pr", Action: "pulls_get", Ref: "pull_request"},
		{As: "files", Action: "pulls_list_files", Ref: "pull_request"},
		{
			As:       "guidelines",
			Action:   "repos_get_content",
			Ref:      "repository",
			Params:   map[string]any{"path": ".github/CONTRIBUTING.md"},
			Optional: true,
		},
	}

	instructions := `A pull request was updated in $refs.repository.

## Pull Request
Title: {{$pr.title}}
Author: {{$pr.user.login}}
State: {{$pr.state}}
Description: {{$pr.body}}

## Files changed
{{$files}}

## Contributing guidelines (base64-encoded for brevity)
Path: {{$guidelines.path}}

Review the changes against the guidelines. Leave one summary review comment.`

	harness.addAgentTrigger(
		[]string{"pull_request.opened"},
		nil,
		contextActions,
		instructions,
	)

	// Stub every Nango call the executor will make. Paths are what the
	// dispatcher produces after path substitution — they match what the
	// GitHub catalog's execution.path says + the refs from the webhook.
	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/pulls/2", "pulls_get.json")
	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/pulls/2/files", "pulls_list_files.json")
	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/contents/.github/CONTRIBUTING.md", "repos_get_content_contributing.json")

	result := harness.runPipeline("pull_request", "opened", "pull_request.opened.json")

	if !result.IsExecutable() {
		t.Fatalf("expected executable run, got skipped=%v silent=%v reason=%q",
			result.Skipped, result.SilentClose, result.SkipReason)
	}
	if len(result.ContextErrors) != 0 {
		t.Errorf("no context errors expected, got: %v", result.ContextErrors)
	}

	// All three context actions should have been called.
	if harness.fakeNango.CallCount() != 3 {
		t.Errorf("expected 3 Nango calls, got %d", harness.fakeNango.CallCount())
	}

	// Assertions on the final built instruction — the load-bearing output.
	// This is THE string the LLM would see as the first user message.
	final := result.FinalInstructions

	// $refs.x from the dispatcher layer
	if !strings.Contains(final, "Codertocat/Hello-World") {
		t.Errorf("instruction missing $refs.repository:\n%s", final)
	}

	// {{$pr.title}} dot-path lookup
	if !strings.Contains(final, "Title: Update the README with new information.") {
		t.Errorf("instruction missing PR title:\n%s", final)
	}

	// {{$pr.user.login}} nested dot-path
	if !strings.Contains(final, "Author: Codertocat") {
		t.Errorf("instruction missing nested user.login:\n%s", final)
	}

	// {{$pr.state}} scalar
	if !strings.Contains(final, "State: open") {
		t.Errorf("instruction missing PR state:\n%s", final)
	}

	// {{$pr.body}} multi-word string
	if !strings.Contains(final, "This is a pretty simple change") {
		t.Errorf("instruction missing PR body:\n%s", final)
	}

	// {{$files}} is a whole array — should be JSON-stringified
	if !strings.Contains(final, "README.md") {
		t.Errorf("instruction missing files array content:\n%s", final)
	}
	if !strings.Contains(final, `"status": "modified"`) {
		t.Errorf("instruction missing files JSON formatting:\n%s", final)
	}

	// {{$guidelines.path}} nested field lookup on the object response
	if !strings.Contains(final, "Path: .github/CONTRIBUTING.md") {
		t.Errorf("instruction missing contributing path:\n%s", final)
	}

	// Sanity check: no unresolved placeholders left behind
	if strings.Contains(final, "{{") {
		t.Errorf("instruction has unresolved {{...}} placeholders:\n%s", final)
	}
	if strings.Contains(final, "$refs.") {
		t.Errorf("instruction has unresolved $refs.x references:\n%s", final)
	}
	if strings.Contains(final, "[missing:") {
		t.Errorf("instruction has [missing:] placeholders:\n%s", final)
	}
}

// --- Test 2: step chaining — one context action references a prior step ---
//
// The second context action's params reference a field from the first step's
// result. Here: fetch the PR first, then fetch the commit at the PR's head
// SHA, using `{{$pr.head.sha}}` in the second call's ref param. The executor
// must substitute the placeholder between calls so the second call receives
// the real SHA, not the template string.
//
// This test proves the executor's substitution-between-calls logic works
// end-to-end: dispatcher produces a ContextRequest with "{{$pr.head.sha}}"
// as a raw string, executor resolves it after the PR fetch completes, and
// the Nango call uses the resolved path.

func TestExecute_GitHub_StepChaining(t *testing.T) {
	harness := newGitHubPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "pr", Action: "pulls_get", Ref: "pull_request"},
		{
			As:     "head_commit",
			Action: "repos_get_commit",
			Params: map[string]any{
				"owner": "$refs.owner",
				"repo":  "$refs.repo",
				"ref":   "{{$pr.head.sha}}",
			},
		},
	}

	instructions := "PR by {{$pr.user.login}} at commit {{$head_commit.sha}} by {{$head_commit.commit.author.name}}"

	harness.addAgentTrigger(
		[]string{"pull_request.opened"},
		nil,
		contextActions,
		instructions,
	)

	// Stub the PR fetch with our standard response (head.sha = 34c5c7793cb3b279e22454cb6750c80560547b3a).
	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/pulls/2", "pulls_get.json")

	// Stub the commit fetch using the SHA from the PR response. The executor
	// must substitute {{$pr.head.sha}} → "34c5c7793cb3b279e22454cb6750c80560547b3a"
	// BEFORE the Nango call fires, so the path here must match the resolved form.
	harness.fakeNango.Stub("GET", "/repos/Codertocat/Hello-World/commits/34c5c7793cb3b279e22454cb6750c80560547b3a", map[string]any{
		"sha": "34c5c7793cb3b279e22454cb6750c80560547b3a",
		"commit": map[string]any{
			"author": map[string]any{
				"name":  "Codertocat",
				"email": "cody@example.com",
				"date":  "2019-05-15T15:20:33Z",
			},
			"message": "Update README.md",
		},
	})

	result := harness.runPipeline("pull_request", "opened", "pull_request.opened.json")

	if !result.IsExecutable() {
		t.Fatalf("expected executable run, skipped=%v silent=%v reason=%q errors=%v",
			result.Skipped, result.SilentClose, result.SkipReason, result.ContextErrors)
	}

	// Verify the CHAIN happened: the second call must have landed on a path
	// with the SHA substituted from the first call's result.
	commitCalls := harness.fakeNango.CallFor("GET", "/repos/Codertocat/Hello-World/commits/34c5c7793cb3b279e22454cb6750c80560547b3a")
	if len(commitCalls) != 1 {
		all := make([]string, 0, len(harness.fakeNango.Calls))
		for _, c := range harness.fakeNango.Calls {
			all = append(all, c.Method+" "+c.Path)
		}
		t.Fatalf("expected 1 call to the resolved commit path, got %d. All calls: %v", len(commitCalls), all)
	}

	// And the final instruction references fields from BOTH steps.
	if !strings.Contains(result.FinalInstructions, "PR by Codertocat") {
		t.Errorf("pr.user.login missing: %q", result.FinalInstructions)
	}
	if !strings.Contains(result.FinalInstructions, "at commit 34c5c7793cb3b279e22454cb6750c80560547b3a") {
		t.Errorf("head_commit.sha missing: %q", result.FinalInstructions)
	}
	// Deep nested chained reference: head_commit.commit.author.name
	if !strings.Contains(result.FinalInstructions, "by Codertocat") {
		t.Errorf("head_commit.commit.author.name missing: %q", result.FinalInstructions)
	}
}

// --- Test 3: optional context failure → run continues with empty result ---
//
// An optional context action fails (Nango returns an error). The executor
// should log it, store the error in ContextErrors, treat the step's value
// as nil, and continue with the rest of the flow. The final instructions
// should still assemble — the optional step just renders as empty where
// it's referenced.

func TestExecute_GitHub_OptionalFailureContinues(t *testing.T) {
	harness := newGitHubPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "pr", Action: "pulls_get", Ref: "pull_request"},
		{
			As:       "guidelines",
			Action:   "repos_get_content",
			Ref:      "repository",
			Params:   map[string]any{"path": ".github/CONTRIBUTING.md"},
			Optional: true,
		},
	}

	instructions := `PR title: {{$pr.title}}

Guidelines content: {{$guidelines.content}}

(Empty guidelines is fine — the repo just doesn't have a CONTRIBUTING.md)`

	harness.addAgentTrigger(
		[]string{"pull_request.opened"},
		nil,
		contextActions,
		instructions,
	)

	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/pulls/2", "pulls_get.json")
	// Force the CONTRIBUTING.md fetch to fail. Simulates a repo without that file.
	harness.stubError("GET", "/repos/Codertocat/Hello-World/contents/.github/CONTRIBUTING.md",
		errors.New("404 not found"))

	result := harness.runPipeline("pull_request", "opened", "pull_request.opened.json")

	// The run should still be executable — optional failure doesn't abort.
	if !result.IsExecutable() {
		t.Fatalf("expected executable run despite optional failure, got: %+v", result)
	}

	// The error should be recorded but the run continued.
	if len(result.ContextErrors) != 1 {
		t.Errorf("expected 1 context error, got %d", len(result.ContextErrors))
	}
	if _, hasErr := result.ContextErrors["guidelines"]; !hasErr {
		t.Errorf("expected error for 'guidelines' step, got: %v", result.ContextErrors)
	}

	// PR result should still be present.
	if result.ContextResults["pr"] == nil {
		t.Errorf("pr result missing")
	}

	// guidelines should be nil (stored as empty step)
	if result.ContextResults["guidelines"] != nil {
		t.Errorf("guidelines result should be nil on optional failure, got: %v", result.ContextResults["guidelines"])
	}

	// Final instruction should have PR title present but guidelines empty.
	if !strings.Contains(result.FinalInstructions, "PR title: Update the README with new information.") {
		t.Errorf("PR title missing: %q", result.FinalInstructions)
	}
	// When guidelines is nil, {{$guidelines.content}} should render as
	// "[missing: guidelines.content]" since the step is nil.
	// (Nil step → empty string for whole-step; nil step with dot-path →
	// missing since we can't walk into nil.)
	if !strings.Contains(result.FinalInstructions, "Guidelines content:") {
		t.Errorf("guidelines line missing: %q", result.FinalInstructions)
	}
}

// --- Test 4: required context failure → run aborts ---
//
// The opposite case: a NON-optional context action fails. The executor
// should return an error wrapping ErrRequiredContextFailed and the run
// should not assemble a final instruction.

func TestExecute_GitHub_RequiredFailureAborts(t *testing.T) {
	harness := newGitHubPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "pr", Action: "pulls_get", Ref: "pull_request"}, // NOT optional
	}

	instructions := "Title: {{$pr.title}}"

	harness.addAgentTrigger(
		[]string{"pull_request.opened"},
		nil,
		contextActions,
		instructions,
	)

	harness.stubError("GET", "/repos/Codertocat/Hello-World/pulls/2",
		errors.New("500 internal server error"))

	payload := harness.loadFixture("pull_request.opened.json")
	input := buildDispatchInput(harness, "pull_request", "opened", payload)

	runs, err := harness.dispatcher.Run(ctxFor(), input)
	if err != nil || len(runs) != 1 {
		t.Fatalf("dispatcher returned unexpected result: runs=%d err=%v", len(runs), err)
	}

	result, err := harness.executor.Execute(ctxFor(), runs[0])
	if err == nil {
		t.Fatal("expected executor to return an error on required context failure")
	}
	if !errors.Is(err, ErrRequiredContextFailed) {
		t.Errorf("expected error to wrap ErrRequiredContextFailed, got: %v", err)
	}

	// The partial result should still have the error recorded.
	if result == nil {
		t.Fatal("expected partial result to be returned alongside error")
	}
	if _, hasErr := result.ContextErrors["pr"]; !hasErr {
		t.Errorf("expected context error for 'pr' step")
	}
}

// --- Test 5: deep nested step path resolution -----------------------------
//
// The template system supports arbitrary dot-path depth into step results.
// Most agents use 2-3 level paths like {{$pr.user.login}} or
// {{$pr.head.repo.name}}, but nothing in the resolver limits depth — this
// test proves a 5-level path resolves correctly and a missing field at
// any depth produces the [missing: ...] marker.
//
// The scenario is realistic: Nango returns a deeply nested repo object
// (owner → plan → features → limits → concurrent_builds). The agent's
// instructions walk all the way down to a single leaf value. A customer
// might want this for reporting or conditional messaging ("your plan
// allows N concurrent builds, you're using M").

func TestExecute_GitHub_DeepNestedStepPath(t *testing.T) {
	harness := newGitHubPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "pr", Action: "pulls_get", Ref: "pull_request"},
	}

	instructions := `PR details:
- Title: {{$pr.title}}
- Author: {{$pr.user.login}}
- Head branch: {{$pr.head.ref}}
- Head repo name: {{$pr.head.repo.name}}
- Head repo full name: {{$pr.head.repo.full_name}}
- Head repo owner login: {{$pr.head.repo.owner.login}}

Missing-at-depth checks:
- Nonexistent intermediate: {{$pr.head.repo.nonexistent.deep}}
- Nonexistent leaf: {{$pr.head.repo.owner.unknown_field}}
- Nonexistent root field: {{$pr.absent_field}}`

	harness.addAgentTrigger(
		[]string{"pull_request.opened"},
		nil,
		contextActions,
		instructions,
	)

	// The existing pulls_get.json fixture has head.repo.owner.login all the
	// way down — a natural 5-level nested path. We use it to prove depth
	// works with real-shaped data, no contrived fixtures.
	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/pulls/2", "pulls_get.json")

	result := harness.runPipeline("pull_request", "opened", "pull_request.opened.json")

	if !result.IsExecutable() {
		t.Fatalf("expected executable, got errors: %v", result.ContextErrors)
	}

	final := result.FinalInstructions

	// Level-by-level verification. If any resolve breaks at a given depth,
	// the specific assertion tells you which level is broken.
	levelChecks := map[string]string{
		"title (L1)":                 "Title: Update the README with new information.",
		"user.login (L2)":            "Author: Codertocat",
		"head.ref (L2)":              "Head branch: changes",
		"head.repo.name (L3)":        "Head repo name: Hello-World",
		"head.repo.full_name (L3)":   "Head repo full name: Codertocat/Hello-World",
		"head.repo.owner.login (L4)": "Head repo owner login: Codertocat",
	}
	for label, expected := range levelChecks {
		if !strings.Contains(final, expected) {
			t.Errorf("%s failed — expected substring %q not found in:\n%s", label, expected, final)
		}
	}

	// Missing-at-depth checks. The resolver should produce [missing: <path>]
	// markers at EVERY depth, not crash or render "<nil>" or empty string.
	// Each missing marker uses the FULL dot-path the template referenced so
	// debugging from logs/output names the exact field that didn't resolve.
	missingChecks := map[string]string{
		"intermediate missing": "Nonexistent intermediate: [missing: pr.head.repo.nonexistent.deep]",
		"leaf missing":         "Nonexistent leaf: [missing: pr.head.repo.owner.unknown_field]",
		"root field missing":   "Nonexistent root field: [missing: pr.absent_field]",
	}
	for label, expected := range missingChecks {
		if !strings.Contains(final, expected) {
			t.Errorf("%s failed — expected marker %q not found in:\n%s", label, expected, final)
		}
	}
}

// --- Test 6: silent terminate short-circuits the executor ---
//
// A dispatcher-produced PreparedRun with SilentClose=true should not fire
// any context actions or assemble any instructions. It's purely a signal
// to the downstream caller to close an existing conversation.

func TestExecute_GitHub_SilentTerminate(t *testing.T) {
	harness := newGitHubPipelineHarness(t)

	terminateRules := []model.TerminateRule{
		{
			TriggerKeys: []string{"pull_request.closed"},
			Silent:      true,
		},
	}

	harness.addAgentTrigger(
		[]string{"pull_request.opened"},
		nil,
		nil,
		"",
		func(_ *model.Agent, trigger *model.AgentTrigger) {
			trigger.TerminateOn = marshalJSON(harness.t, terminateRules)
			trigger.TerminateEventKeys = []string{"pull_request.closed"}
		},
	)

	// Fire a close event. The dispatcher will build a terminate run with
	// SilentClose=true; the executor must short-circuit.
	payload := harness.loadFixture("pull_request.opened.json")
	// Mutate the payload to simulate a close event
	payload["action"] = "closed"
	if pr, ok := payload["pull_request"].(map[string]any); ok {
		pr["state"] = "closed"
	}
	input := buildDispatchInput(harness, "pull_request", "closed", payload)

	runs, err := harness.dispatcher.Run(ctxFor(), input)
	if err != nil || len(runs) != 1 {
		t.Fatalf("dispatcher: runs=%d err=%v", len(runs), err)
	}

	result, err := harness.executor.Execute(ctxFor(), runs[0])
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !result.SilentClose {
		t.Errorf("expected SilentClose=true, got %+v", result)
	}
	if result.IsExecutable() {
		t.Error("silent close run should not be executable")
	}
	// Crucially: no Nango calls happened.
	if harness.fakeNango.CallCount() != 0 {
		t.Errorf("expected 0 Nango calls on silent close, got %d", harness.fakeNango.CallCount())
	}
	if result.FinalInstructions != "" {
		t.Errorf("silent close should have empty final instructions, got: %q", result.FinalInstructions)
	}
}
