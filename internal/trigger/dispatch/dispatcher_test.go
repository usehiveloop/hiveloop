package dispatch

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/model"
)

// All tests in this file follow the same shape:
//   1. Build a fresh harness (in-memory store + real catalog).
//   2. Seed one or more agent triggers with realistic config.
//   3. Load a real GitHub webhook fixture from testdata/github/.
//   4. Run the dispatcher.
//   5. Assert on the resulting PreparedRun(s).
//
// The fixtures are real GitHub webhook payloads from octokit/webhooks (see
// testdata/github/SOURCES.md). The catalog is the real embedded catalog so
// trigger refs and resource ref_bindings come from production data.

// --- Test 1: issues.opened — happy path with three context requests --------
//
// Verifies the full happy path: refs are extracted, instructions are
// substituted, three context actions resolve to fully-formed Path/Query
// requests with $refs.x replacements applied to the search query template.

func TestDispatch_IssuesOpened_HappyPath(t *testing.T) {
	harness := newHarness(t)

	contextActions := []model.ContextAction{
		{As: "issue", Action: "issues_get", Ref: "issue"},
		{As: "labels", Action: "issues_list_labels_for_repo", Ref: "label", Params: map[string]any{
			"owner": "$refs.owner",
			"repo":  "$refs.repo",
		}},
		{As: "similar", Action: "search_issues_and_pull_requests", Params: map[string]any{
			"q": "repo:$refs.owner/$refs.repo is:issue state:open",
		}},
	}
	instructions := "An issue was opened in $refs.repo by $refs.sender. Issue #$refs.issue_number."

	harness.addTrigger(
		[]string{"issues.opened"},
		nil,
		contextActions,
		instructions,
	)

	payload := loadFixture(t, "issues.opened.json")
	runs := harness.run("issues", "opened", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected run to fire, got skip: %s", run.SkipReason)
	}
	if run.TriggerKey != "issues.opened" {
		t.Errorf("trigger key = %q, want issues.opened", run.TriggerKey)
	}
	if run.SandboxStrategy != SandboxStrategyReusePool {
		t.Errorf("sandbox strategy = %q, want %q", run.SandboxStrategy, SandboxStrategyReusePool)
	}
	if run.RunIntent != RunIntentNormal {
		t.Errorf("run intent = %q, want %q", run.RunIntent, RunIntentNormal)
	}
	// ResourceKey is resolved from the issue resource's template.
	if run.ResourceKey != "Codertocat/Hello-World#issue-1" {
		t.Errorf("resource key = %q, want Codertocat/Hello-World#issue-1", run.ResourceKey)
	}

	// Refs come from the real fixture: Codertocat/Hello-World, issue #1.
	wantRefs := map[string]string{
		"owner":        "Codertocat",
		"repo":         "Hello-World",
		"repository":   "Codertocat/Hello-World",
		"sender":       "Codertocat",
		"issue_number": "1",
		"issue_title":  "Spelling error in the README file",
	}
	for refName, wantValue := range wantRefs {
		if got := run.Refs[refName]; got != wantValue {
			t.Errorf("refs[%q] = %q, want %q", refName, got, wantValue)
		}
	}

	// Instructions: $refs.x bare references substituted.
	wantInstructions := "An issue was opened in Hello-World by Codertocat. Issue #1."
	if run.Instructions != wantInstructions {
		t.Errorf("instructions = %q, want %q", run.Instructions, wantInstructions)
	}

	// Three context requests in input order.
	if len(run.ContextRequests) != 3 {
		t.Fatalf("expected 3 context requests, got %d", len(run.ContextRequests))
	}

	// issue: ref:issue → ref_bindings fill owner/repo/issue_number → path substituted.
	issueRequest := assertContextRequest(t, run, "issue")
	if issueRequest.ActionKey != "issues_get" {
		t.Errorf("issue action = %q, want issues_get", issueRequest.ActionKey)
	}
	if issueRequest.Method != "GET" {
		t.Errorf("issue method = %q, want GET", issueRequest.Method)
	}
	wantPath := "/repos/Codertocat/Hello-World/issues/1"
	if issueRequest.Path != wantPath {
		t.Errorf("issue path = %q, want %q", issueRequest.Path, wantPath)
	}

	// labels: explicit params with $refs.x substitution → path filled, no query.
	labelsRequest := assertContextRequest(t, run, "labels")
	if labelsRequest.Path != "/repos/Codertocat/Hello-World/labels" {
		t.Errorf("labels path = %q", labelsRequest.Path)
	}

	// similar: search query string with multiple $refs.x in one value.
	similarRequest := assertContextRequest(t, run, "similar")
	if similarRequest.Path != "/search/issues" {
		t.Errorf("similar path = %q", similarRequest.Path)
	}
	if similarRequest.Query["q"] != "repo:Codertocat/Hello-World is:issue state:open" {
		t.Errorf("similar query.q = %q", similarRequest.Query["q"])
	}
}

// --- Test 2: issues.opened — bot author filtered by not_one_of -------------
//
// The condition uses the not_one_of operator with a list of bot usernames.
// We patch the fixture to set sender.login to dependabot[bot] so the condition
// fires the skip path. The PreparedRun is still emitted with SkipReason set
// (for observability) but should not be enqueued by the executor.

func TestDispatch_IssuesOpened_BotFiltered(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "sender.login", Operator: "not_one_of", Value: []any{"dependabot[bot]", "renovate[bot]"}},
		},
	}
	harness.addTrigger(
		[]string{"issues.opened"},
		conditions,
		[]model.ContextAction{{As: "issue", Action: "issues_get", Ref: "issue"}},
		"trigger fired",
	)

	payload := loadFixture(t, "issues.opened.json")
	// Patch the sender to dependabot[bot] so the not_one_of condition fails.
	patchPath(t, payload, "sender.login", "dependabot[bot]")

	runs := harness.run("issues", "opened", payload)
	run := assertSinglePrepared(t, runs)

	if !run.Skipped() {
		t.Fatalf("expected run to skip, got fire")
	}
	if !strings.Contains(run.SkipReason, "sender.login") {
		t.Errorf("skip reason should mention sender.login, got %q", run.SkipReason)
	}
	if !strings.Contains(run.SkipReason, "not_one_of") {
		t.Errorf("skip reason should mention operator not_one_of, got %q", run.SkipReason)
	}
	// Refs are still populated even on skip — debugging visibility.
	if run.Refs["sender"] != "dependabot[bot]" {
		t.Errorf("expected refs.sender = dependabot[bot] on skip path, got %q", run.Refs["sender"])
	}
}

// --- Test 3: pull_request multi-event trigger fires on opened --------------
//
// The trigger lists three keys (opened/synchronize/ready_for_review). The
// dispatcher must pick the matching one and run the trigger. The fixture has
// draft=false so the no-draft condition passes.

func TestDispatch_PullRequestOpened_MultiEvent(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "pull_request.draft", Operator: "not_equals", Value: true},
		},
	}
	contextActions := []model.ContextAction{
		{As: "pr", Action: "pulls_get", Ref: "pull_request"},
		{As: "files", Action: "pulls_list_files", Ref: "pull_request"},
	}
	harness.addTrigger(
		[]string{"pull_request.opened", "pull_request.synchronize", "pull_request.ready_for_review"},
		conditions,
		contextActions,
		"PR #$refs.pull_number opened in $refs.repo",
	)

	payload := loadFixture(t, "pull_request.opened.json")
	runs := harness.run("pull_request", "opened", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}
	if run.Refs["pull_number"] != "2" {
		t.Errorf("pull_number = %q, want 2", run.Refs["pull_number"])
	}
	if run.ResourceKey != "Codertocat/Hello-World#pr-2" {
		t.Errorf("resource key = %q, want Codertocat/Hello-World#pr-2", run.ResourceKey)
	}
	if run.Instructions != "PR #2 opened in Hello-World" {
		t.Errorf("instructions = %q", run.Instructions)
	}

	files := assertContextRequest(t, run, "files")
	if files.Path != "/repos/Codertocat/Hello-World/pulls/2/files" {
		t.Errorf("files path = %q", files.Path)
	}
}

// --- Test 4: pull_request draft filtered out --------------------------------
//
// Same trigger as test 3, but with the fixture's pull_request.draft patched to
// true. The not_equals condition fails and the run is skipped. Demonstrates
// the draft check that PR review agents typically use.

func TestDispatch_PullRequestOpened_DraftSkipped(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "pull_request.draft", Operator: "not_equals", Value: true},
		},
	}
	harness.addTrigger(
		[]string{"pull_request.opened"},
		conditions,
		nil,
		"",
	)

	payload := loadFixture(t, "pull_request.opened.json")
	patchPath(t, payload, "pull_request.draft", true)

	runs := harness.run("pull_request", "opened", payload)
	run := assertSinglePrepared(t, runs)

	if !run.Skipped() {
		t.Fatalf("expected skip, got fire")
	}
	if !strings.Contains(run.SkipReason, "pull_request.draft") {
		t.Errorf("skip reason should mention pull_request.draft, got %q", run.SkipReason)
	}
}

// --- Test 5: issue_comment with @mention condition + not_exists check ------
//
// Two conditions combined with match: all. The comment must contain @zira AND
// the issue must NOT have a pull_request field (i.e. it's a real issue, not a
// PR comment). Tests both the contains operator and not_exists operator.
// Also exercises {{$refs.x}} mustache substitution in the instructions.

func TestDispatch_IssueComment_MentionOnIssue(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "comment.body", Operator: "contains", Value: "@zira"},
			{Path: "issue.pull_request", Operator: "not_exists"},
		},
	}
	contextActions := []model.ContextAction{
		{As: "issue", Action: "issues_get", Ref: "issue"},
	}
	instructions := "{{$refs.sender}} mentioned the bot on issue #{{$refs.issue_number}}"
	harness.addTrigger(
		[]string{"issue_comment.created"},
		conditions,
		contextActions,
		instructions,
	)

	payload := loadFixture(t, "issue_comment.created.issue.json")
	runs := harness.run("issue_comment", "created", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}
	// Mustache form substituted.
	if !strings.Contains(run.Instructions, "issue #1") {
		t.Errorf("instructions should contain issue #1, got %q", run.Instructions)
	}
	if strings.Contains(run.Instructions, "{{") {
		t.Errorf("instructions should have no leftover mustache, got %q", run.Instructions)
	}
}

// --- Test 6: same trigger fires on issue but skips on PR --------------------
//
// Same conditions as test 5, but the fixture has issue.pull_request populated
// (it's a comment on a PR). The not_exists condition should fail.

func TestDispatch_IssueComment_PRCommentSkipped(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "comment.body", Operator: "contains", Value: "@zira"},
			{Path: "issue.pull_request", Operator: "not_exists"},
		},
	}
	harness.addTrigger(
		[]string{"issue_comment.created"},
		conditions,
		nil,
		"",
	)

	payload := loadFixture(t, "issue_comment.created.pr.json")
	runs := harness.run("issue_comment", "created", payload)
	run := assertSinglePrepared(t, runs)

	if !run.Skipped() {
		t.Fatalf("expected skip, got fire")
	}
	if !strings.Contains(run.SkipReason, "issue.pull_request") {
		t.Errorf("skip reason should mention issue.pull_request, got %q", run.SkipReason)
	}
	if !strings.Contains(run.SkipReason, "not_exists") {
		t.Errorf("skip reason should mention not_exists, got %q", run.SkipReason)
	}
}

// --- Test 7: push to main matches regex ------------------------------------
//
// The push.new-branch fixture pushes to refs/heads/master. The condition uses
// the matches operator with a regex anchored to ^refs/heads/master$. Validates
// the regex operator and confirms the actionless trigger key shape ("push"
// rather than "push.something").

func TestDispatch_Push_RegexMatch(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "ref", Operator: "matches", Value: "^refs/heads/master$"},
		},
	}
	harness.addTrigger(
		[]string{"push"},
		conditions,
		nil,
		"main branch updated to $refs.after",
	)

	payload := loadFixture(t, "push.new-branch.json")
	runs := harness.run("push", "", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}
	if run.TriggerKey != "push" {
		t.Errorf("trigger key = %q, want push (no .action suffix)", run.TriggerKey)
	}
	if run.Refs["ref"] != "refs/heads/master" {
		t.Errorf("ref = %q", run.Refs["ref"])
	}
	// push maps to the repository resource, which has no template — always new run.
	if run.ResourceKey != "" {
		t.Errorf("resource key = %q, want empty (push has no continuation)", run.ResourceKey)
	}
}

// --- Test 8: push to a tag does not match the heads/main regex --------------
//
// The push.json fixture pushes to refs/tags/simple-tag. The same regex from
// test 7 should not match.

func TestDispatch_Push_RegexMissMisses(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "ref", Operator: "matches", Value: "^refs/heads/master$"},
		},
	}
	harness.addTrigger(
		[]string{"push"},
		conditions,
		nil,
		"",
	)

	payload := loadFixture(t, "push.json")
	runs := harness.run("push", "", payload)
	run := assertSinglePrepared(t, runs)

	if !run.Skipped() {
		t.Fatalf("expected skip, got fire (ref=%s)", run.Refs["ref"])
	}
}

// --- Test 9: workflow_run completed with conclusion=failure ----------------
//
// The fixture is patched to set conclusion=failure. The condition checks the
// equals operator on a nested path. Demonstrates the workflow resource ref
// binding (run_id → $refs.run_id) resolves correctly using the catalog's fixed
// trigger refs.

func TestDispatch_WorkflowRun_FailureOnly(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "workflow_run.conclusion", Operator: "equals", Value: "failure"},
		},
	}
	contextActions := []model.ContextAction{
		{As: "run", Action: "actions_get_workflow_run", Ref: "workflow"},
	}
	harness.addTrigger(
		[]string{"workflow_run.completed"},
		conditions,
		contextActions,
		"workflow $refs.workflow_name failed (run $refs.run_id)",
	)

	payload := loadFixture(t, "workflow_run.completed.failure.json")
	runs := harness.run("workflow_run", "completed", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}
	// run_id ref comes from workflow_run.id in the catalog.
	if run.Refs["run_id"] == "" {
		t.Errorf("run_id ref should be populated, got empty")
	}
	runRequest := assertContextRequest(t, run, "run")
	// The workflow_run fixture is from octo-org/octo-repo (not the Codertocat
	// fixture used by issue/PR tests). The path must use whatever repo refs
	// were extracted from this fixture, not what we assumed.
	wantPath := "/repos/" + run.Refs["owner"] + "/" + run.Refs["repo"] + "/actions/runs/" + run.Refs["run_id"]
	if runRequest.Path != wantPath {
		t.Errorf("run path = %q, want %q", runRequest.Path, wantPath)
	}
}

// --- Test 10: dedicated agent → CreateDedicated sandbox strategy ------------
//
// release.published with a dedicated agent. The dispatcher must record the
// sandbox strategy correctly so the executor knows to provision a fresh
// sandbox per run instead of reusing a pool sandbox.

func TestDispatch_Release_DedicatedAgent(t *testing.T) {
	harness := newHarness(t)

	contextActions := []model.ContextAction{
		{As: "repo", Action: "repos_get", Ref: "repository"},
	}
	harness.addTrigger(
		[]string{"release.published"},
		nil,
		contextActions,
		"release $refs.release_tag published",
		func(agent *model.Agent, _ *model.AgentTrigger) {
			agent.SandboxType = "dedicated"
			agent.SandboxID = nil
		},
	)

	payload := loadFixture(t, "release.published.json")
	runs := harness.run("release", "published", payload)
	run := assertSinglePrepared(t, runs)

	if run.SandboxStrategy != SandboxStrategyCreateDedicated {
		t.Errorf("sandbox strategy = %q, want %q", run.SandboxStrategy, SandboxStrategyCreateDedicated)
	}
	if run.SandboxID != nil {
		t.Errorf("sandbox id should be nil for dedicated agent, got %v", run.SandboxID)
	}
	if run.Instructions != "release 0.0.1 published" {
		t.Errorf("instructions = %q", run.Instructions)
	}
	repoRequest := assertContextRequest(t, run, "repo")
	if repoRequest.Path != "/repos/Codertocat/Hello-World" {
		t.Errorf("repo path = %q", repoRequest.Path)
	}
}

// --- Test 11: two agents on the same connection both fire ------------------
//
// Both agents listen for issues.opened on the same connection. The dispatcher
// must produce two PreparedRuns in deterministic order (sorted by trigger ID).
// One agent is shared, one is dedicated — confirms per-run sandbox decisions
// don't bleed across runs.

func TestDispatch_TwoAgents_FanOut(t *testing.T) {
	harness := newHarness(t)

	// Lower trigger ID first so it sorts first deterministically.
	lowerID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	upperID := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")

	harness.addTrigger(
		[]string{"issues.opened"},
		nil,
		[]model.ContextAction{{As: "issue", Action: "issues_get", Ref: "issue"}},
		"shared: $refs.issue_number",
		func(_ *model.Agent, trigger *model.AgentTrigger) {
			trigger.ID = lowerID
		},
	)
	harness.addTrigger(
		[]string{"issues.opened"},
		nil,
		[]model.ContextAction{{As: "issue", Action: "issues_get", Ref: "issue"}},
		"dedicated: $refs.issue_number",
		func(agent *model.Agent, trigger *model.AgentTrigger) {
			agent.SandboxType = "dedicated"
			agent.SandboxID = nil
			trigger.ID = upperID
		},
	)

	payload := loadFixture(t, "issues.opened.json")
	runs := harness.run("issues", "opened", payload)

	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].AgentTriggerID != lowerID {
		t.Errorf("first run trigger ID = %v, want %v (sorted by ID)", runs[0].AgentTriggerID, lowerID)
	}
	if runs[1].AgentTriggerID != upperID {
		t.Errorf("second run trigger ID = %v, want %v", runs[1].AgentTriggerID, upperID)
	}
	if runs[0].SandboxStrategy != SandboxStrategyReusePool {
		t.Errorf("first run should be ReusePool, got %q", runs[0].SandboxStrategy)
	}
	if runs[1].SandboxStrategy != SandboxStrategyCreateDedicated {
		t.Errorf("second run should be CreateDedicated, got %q", runs[1].SandboxStrategy)
	}
	if runs[0].Instructions != "shared: 1" || runs[1].Instructions != "dedicated: 1" {
		t.Errorf("instructions wrong: [%q, %q]", runs[0].Instructions, runs[1].Instructions)
	}
}

// --- Test 12: cross-trigger continuation ------------------------------------
//
// Three different event types fired on the same issue — issues.opened,
// issues.labeled, and issue_comment.created — all resolve to the same
// ResourceKey. This is the foundation of the continue-vs-new decision: the
// executor will route all three to the same conversation.
//
// The agent listens to all three event keys on a single trigger config. We
// fire each event in sequence and assert every PreparedRun has the same
// ResourceKey, so a real executor would look them all up under one row.

func TestDispatch_CrossTriggerContinuation(t *testing.T) {
	harness := newHarness(t)

	harness.addTrigger(
		[]string{"issues.opened", "issues.labeled", "issue_comment.created"},
		nil,
		[]model.ContextAction{{As: "issue", Action: "issues_get", Ref: "issue"}},
		"event on issue #$refs.issue_number",
	)

	wantKey := "Codertocat/Hello-World#issue-1"

	opened := assertSinglePrepared(t, harness.run("issues", "opened", loadFixture(t, "issues.opened.json")))
	if opened.Skipped() {
		t.Fatalf("opened skipped: %s", opened.SkipReason)
	}
	if opened.ResourceKey != wantKey {
		t.Errorf("opened resource key = %q, want %q", opened.ResourceKey, wantKey)
	}

	labeled := assertSinglePrepared(t, harness.run("issues", "labeled", loadFixture(t, "issues.labeled.json")))
	if labeled.Skipped() {
		t.Fatalf("labeled skipped: %s", labeled.SkipReason)
	}
	if labeled.ResourceKey != wantKey {
		t.Errorf("labeled resource key = %q, want %q", labeled.ResourceKey, wantKey)
	}

	commented := assertSinglePrepared(t, harness.run("issue_comment", "created", loadFixture(t, "issue_comment.created.issue.json")))
	if commented.Skipped() {
		t.Fatalf("commented skipped: %s", commented.SkipReason)
	}
	if commented.ResourceKey != wantKey {
		t.Errorf("commented resource key = %q, want %q", commented.ResourceKey, wantKey)
	}

	// All three runs target the same subject — this is the core invariant the
	// executor will rely on to thread them into a single conversation.
	if opened.ResourceKey != labeled.ResourceKey || labeled.ResourceKey != commented.ResourceKey {
		t.Errorf("cross-trigger keys drifted: opened=%q labeled=%q commented=%q",
			opened.ResourceKey, labeled.ResourceKey, commented.ResourceKey)
	}
}

// --- Test 13: terminate with graceful final run ------------------------------
//
// A PR-review agent watches pull_request.opened/synchronize and terminates on
// pull_request.closed when the PR was merged. The terminate rule has its own
// context actions and instructions for a final summary. We patch the fixture
// to set `merged: true` so the terminate rule's condition passes.

func TestDispatch_TerminateMergedPR_GracefulClose(t *testing.T) {
	harness := newHarness(t)

	harness.addTerminateTrigger(
		[]string{"pull_request.opened", "pull_request.synchronize"},
		nil,
		[]model.ContextAction{{As: "pr", Action: "pulls_get", Ref: "pull_request"}},
		"review PR #$refs.pull_number",
		[]model.TerminateRule{
			{
				TriggerKeys: []string{"pull_request.closed"},
				Conditions: &model.TriggerMatch{
					Mode: "all",
					Conditions: []model.TriggerCondition{
						{Path: "pull_request.merged", Operator: "equals", Value: true},
					},
				},
				ContextActions: []model.ContextAction{
					{As: "final", Action: "pulls_get", Ref: "pull_request"},
				},
				Instructions: "PR #$refs.pull_number was merged — post a short summary",
			},
		},
	)

	payload := loadFixture(t, "pull_request.opened.json")
	// Simulate a close event by patching the action + merged flag.
	payload["action"] = "closed"
	patchPath(t, payload, "pull_request.merged", true)
	patchPath(t, payload, "pull_request.state", "closed")

	runs := harness.run("pull_request", "closed", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected terminate run to fire, got skip: %s", run.SkipReason)
	}
	if run.RunIntent != RunIntentTerminate {
		t.Errorf("run intent = %q, want %q", run.RunIntent, RunIntentTerminate)
	}
	if run.SilentClose {
		t.Error("expected graceful close (SilentClose=false)")
	}
	if run.ResourceKey != "Codertocat/Hello-World#pr-2" {
		t.Errorf("resource key = %q", run.ResourceKey)
	}
	// Final context action should resolve using the same PR refs.
	final := assertContextRequest(t, run, "final")
	if final.Path != "/repos/Codertocat/Hello-World/pulls/2" {
		t.Errorf("final path = %q", final.Path)
	}
	if run.Instructions != "PR #2 was merged — post a short summary" {
		t.Errorf("instructions = %q", run.Instructions)
	}
}

// --- Test 14: first-match-wins on multiple terminate rules ------------------
//
// Two rules on the same terminate key — one for merged, one for not-merged
// with silent: true. A closed-without-merge PR should match the second rule,
// not the first. Validates rule ordering, condition evaluation, and silent
// close semantics together.

func TestDispatch_TerminateClosedUnmerged_SilentClose(t *testing.T) {
	harness := newHarness(t)

	harness.addTerminateTrigger(
		[]string{"pull_request.opened", "pull_request.synchronize"},
		nil,
		[]model.ContextAction{{As: "pr", Action: "pulls_get", Ref: "pull_request"}},
		"review PR",
		[]model.TerminateRule{
			{
				TriggerKeys: []string{"pull_request.closed"},
				Conditions: &model.TriggerMatch{
					Mode: "all",
					Conditions: []model.TriggerCondition{
						{Path: "pull_request.merged", Operator: "equals", Value: true},
					},
				},
				Instructions: "PR merged — summarize",
			},
			{
				TriggerKeys: []string{"pull_request.closed"},
				Conditions: &model.TriggerMatch{
					Mode: "all",
					Conditions: []model.TriggerCondition{
						{Path: "pull_request.merged", Operator: "equals", Value: false},
					},
				},
				Silent: true,
			},
		},
	)

	payload := loadFixture(t, "pull_request.opened.json")
	payload["action"] = "closed"
	patchPath(t, payload, "pull_request.merged", false)
	patchPath(t, payload, "pull_request.state", "closed")

	runs := harness.run("pull_request", "closed", payload)
	run := assertSinglePrepared(t, runs)

	if run.RunIntent != RunIntentTerminate {
		t.Errorf("run intent = %q, want %q", run.RunIntent, RunIntentTerminate)
	}
	if !run.SilentClose {
		t.Error("expected SilentClose=true for closed-without-merge")
	}
	// Silent runs have no context requests and no instructions.
	if len(run.ContextRequests) != 0 {
		t.Errorf("silent close should have no context requests, got %d", len(run.ContextRequests))
	}
	if run.Instructions != "" {
		t.Errorf("silent close should have empty instructions, got %q", run.Instructions)
	}
	// But ResourceKey MUST be populated so the executor can find the conversation.
	if run.ResourceKey != "Codertocat/Hello-World#pr-2" {
		t.Errorf("silent close must have resource key, got %q", run.ResourceKey)
	}
}

// --- Test 15: parent conditions inherited into terminate rule ---------------
//
// Parent trigger skips draft PRs. A terminate rule fires on pull_request.closed.
// If the PR being closed is a draft, the parent's draft filter should propagate
// into the terminate rule automatically — we never terminate a conversation
// we were never tracking.

func TestDispatch_TerminateInheritsParentConditions(t *testing.T) {
	harness := newHarness(t)

	parentConditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "pull_request.draft", Operator: "not_equals", Value: true},
		},
	}

	harness.addTerminateTrigger(
		[]string{"pull_request.opened"},
		parentConditions,
		nil,
		"",
		[]model.TerminateRule{
			{
				TriggerKeys:  []string{"pull_request.closed"},
				Instructions: "PR closed",
			},
		},
	)

	// Fire a close event on a draft PR. Parent conditions say "not draft",
	// so the terminate rule should also be skipped.
	payload := loadFixture(t, "pull_request.opened.json")
	payload["action"] = "closed"
	patchPath(t, payload, "pull_request.draft", true)
	patchPath(t, payload, "pull_request.state", "closed")

	runs := harness.run("pull_request", "closed", payload)
	run := assertSinglePrepared(t, runs)

	if !run.Skipped() {
		t.Fatalf("expected skip, got fire (inherited parent conditions should reject drafts)")
	}
	if !strings.Contains(run.SkipReason, "parent") {
		t.Errorf("skip reason should attribute the failure to parent conditions, got %q", run.SkipReason)
	}
	if run.RunIntent != RunIntentTerminate {
		t.Errorf("run intent = %q, want %q (skipped runs still carry their intended intent)", run.RunIntent, RunIntentTerminate)
	}
}

// --- Test 16: ambiguous key rejected at dispatch time ----------------------
//
// A trigger lists pull_request.closed in BOTH trigger_keys and terminate_on.
// The handler should reject this at save time, but the dispatcher has a
// safety-net check in case a trigger gets persisted that way (drift, direct
// DB edits, stale schemas). The run should be skipped with a clear reason
// mentioning the ambiguity.

func TestDispatch_AmbiguousKeyRejected(t *testing.T) {
	harness := newHarness(t)

	harness.addTerminateTrigger(
		[]string{"pull_request.opened", "pull_request.closed"}, // closed is in both lists
		nil,
		nil,
		"",
		[]model.TerminateRule{
			{
				TriggerKeys:  []string{"pull_request.closed"}, // also here — ambiguous
				Instructions: "close",
			},
		},
	)

	payload := loadFixture(t, "pull_request.opened.json")
	payload["action"] = "closed"
	patchPath(t, payload, "pull_request.state", "closed")

	runs := harness.run("pull_request", "closed", payload)
	run := assertSinglePrepared(t, runs)

	if !run.Skipped() {
		t.Fatalf("expected skip due to ambiguous config, got fire")
	}
	if !strings.Contains(run.SkipReason, "ambiguous") {
		t.Errorf("skip reason should mention ambiguity, got %q", run.SkipReason)
	}
}

// --- Test 17: terminate store query finds trigger by terminate_event_keys --
//
// The AgentTriggerStore query must match triggers whose EVENT only appears
// in terminate_event_keys (not in trigger_keys). This test configures a
// trigger that only fires normal runs on `pull_request.opened` but terminates
// on `pull_request.closed`. When a pull_request.closed webhook arrives, the
// store must return this trigger — otherwise the terminate path is dead on
// arrival.

func TestDispatch_TerminateOnlyKey_StoreMatches(t *testing.T) {
	harness := newHarness(t)

	harness.addTerminateTrigger(
		[]string{"pull_request.opened"}, // no `pull_request.closed` in normal keys
		nil,
		[]model.ContextAction{{As: "pr", Action: "pulls_get", Ref: "pull_request"}},
		"review PR",
		[]model.TerminateRule{
			{
				TriggerKeys:  []string{"pull_request.closed"},
				Instructions: "PR closed — wrap up",
			},
		},
	)

	payload := loadFixture(t, "pull_request.opened.json")
	payload["action"] = "closed"
	patchPath(t, payload, "pull_request.state", "closed")

	runs := harness.run("pull_request", "closed", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("store should have matched terminate_event_keys, got skip: %s", run.SkipReason)
	}
	if run.RunIntent != RunIntentTerminate {
		t.Errorf("run intent = %q, want %q", run.RunIntent, RunIntentTerminate)
	}
	if run.Instructions != "PR closed — wrap up" {
		t.Errorf("instructions = %q", run.Instructions)
	}
}

// --- Test 18: match: any (OR conditions) ------------------------------------
//
// Realistic triage scenario. A bug-escalation agent fires on any issue whose
// title matches one of several critical prefixes: "CRITICAL", "URGENT", "P0".
// The conditions list has three alternatives combined with match: any — if
// ANY of them is true, the trigger fires. If ALL of them are false, the run
// is skipped.
//
// This is the natural shape for `any` mode: "fire if any of these alerting
// signals is present." Most other conditions are AND (`all`) — `any` is the
// escape hatch for explicit OR semantics.
//
// We run the test three times with different titles to exercise each branch
// of the OR, plus one where no condition passes to verify the skip path.

func TestDispatch_MatchAny_BugEscalation(t *testing.T) {
	harness := newHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "any",
		Conditions: []model.TriggerCondition{
			{Path: "issue.title", Operator: "contains", Value: "CRITICAL"},
			{Path: "issue.title", Operator: "contains", Value: "URGENT"},
			{Path: "issue.title", Operator: "contains", Value: "P0"},
		},
	}

	harness.addTrigger(
		[]string{"issues.opened"},
		conditions,
		nil,
		"escalation: $refs.issue_title",
	)

	cases := []struct {
		name        string
		title       string
		shouldFire  bool
		matchBranch string // the condition index that should have passed
	}{
		{name: "first_branch_critical", title: "CRITICAL: login broken", shouldFire: true, matchBranch: "0"},
		{name: "second_branch_urgent", title: "URGENT - deploy failing", shouldFire: true, matchBranch: "1"},
		{name: "third_branch_p0", title: "[P0] production down", shouldFire: true, matchBranch: "2"},
		{name: "no_branch_matches", title: "minor typo in docs", shouldFire: false},
		{name: "case_sensitive_miss", title: "critical things (lowercase)", shouldFire: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := loadFixture(t, "issues.opened.json")
			patchPath(t, payload, "issue.title", testCase.title)

			runs := harness.run("issues", "opened", payload)
			run := assertSinglePrepared(t, runs)

			if testCase.shouldFire {
				if run.Skipped() {
					t.Errorf("expected fire for title %q, got skip: %s", testCase.title, run.SkipReason)
				}
			} else {
				if !run.Skipped() {
					t.Errorf("expected skip for title %q, got fire", testCase.title)
				}
				// Skip reason for `any` mode names the LAST failing condition so
				// logs are at least specific about what didn't match.
				if !strings.Contains(run.SkipReason, "no conditions matched") {
					t.Errorf("skip reason should say 'no conditions matched', got %q", run.SkipReason)
				}
			}
		})
	}
}

// --- Test 19: only_when on context actions ----------------------------------
//
// A multi-event trigger row can have context actions that only fire for
// specific events via the `only_when` filter. Useful when a single agent
// row handles multiple events but each event needs different context.
//
// Example: a PR lifecycle agent handles `opened` (needs initial files),
// `synchronize` (needs updated files), and `closed` (needs the final state
// + merged flag, but no diff). With only_when, each context action targets
// the events where it's meaningful — no wasted fetches on irrelevant events.
//
// We verify that firing `opened` produces the opened-specific context but
// not the closed-specific context, and vice versa.

func TestDispatch_MultiEvent_OnlyWhen(t *testing.T) {
	harness := newHarness(t)

	// Three context actions on one trigger row listening to two events.
	//   - `pr` always fires (no only_when, applies to both)
	//   - `files` only on opened (fetch diff while reviewing a new PR)
	//   - `final_pr` only on closed (fetch the post-close state for summary)
	contextActions := []model.ContextAction{
		{As: "pr", Action: "pulls_get", Ref: "pull_request"},
		{
			As:       "files",
			Action:   "pulls_list_files",
			Ref:      "pull_request",
			OnlyWhen: []string{"pull_request.opened"},
		},
		{
			As:       "final_pr",
			Action:   "pulls_get",
			Ref:      "pull_request",
			OnlyWhen: []string{"pull_request.closed"},
		},
	}

	harness.addTrigger(
		[]string{"pull_request.opened", "pull_request.closed"},
		nil,
		contextActions,
		"event on PR #$refs.pull_number",
	)

	t.Run("opened fires files but not final_pr", func(t *testing.T) {
		payload := loadFixture(t, "pull_request.opened.json")
		runs := harness.run("pull_request", "opened", payload)
		run := assertSinglePrepared(t, runs)

		if run.Skipped() {
			t.Fatalf("opened run skipped: %s", run.SkipReason)
		}

		// Expect exactly two context requests: pr + files. final_pr is filtered out.
		names := contextRequestNames(run.ContextRequests)
		if !containsString(names, "pr") {
			t.Error("pr (unconditional) should be present on opened")
		}
		if !containsString(names, "files") {
			t.Error("files (only_when opened) should be present on opened")
		}
		if containsString(names, "final_pr") {
			t.Error("final_pr (only_when closed) should NOT be present on opened")
		}
		if len(run.ContextRequests) != 2 {
			t.Errorf("expected 2 context requests, got %d: %v", len(run.ContextRequests), names)
		}
	})

	t.Run("closed fires final_pr but not files", func(t *testing.T) {
		payload := loadFixture(t, "pull_request.opened.json")
		// Mutate to simulate a close event.
		payload["action"] = "closed"
		patchPath(t, payload, "pull_request.state", "closed")

		runs := harness.run("pull_request", "closed", payload)
		run := assertSinglePrepared(t, runs)

		if run.Skipped() {
			t.Fatalf("closed run skipped: %s", run.SkipReason)
		}

		names := contextRequestNames(run.ContextRequests)
		if !containsString(names, "pr") {
			t.Error("pr (unconditional) should be present on closed")
		}
		if !containsString(names, "final_pr") {
			t.Error("final_pr (only_when closed) should be present on closed")
		}
		if containsString(names, "files") {
			t.Error("files (only_when opened) should NOT be present on closed")
		}
		if len(run.ContextRequests) != 2 {
			t.Errorf("expected 2 context requests, got %d: %v", len(run.ContextRequests), names)
		}
	})
}

// --- Test 20: ignore_parent_conditions on terminate rules -------------------
//
// The default is that terminate rules inherit the parent trigger's conditions
// — if the parent says "skip drafts," the terminate rule also skips drafts.
// This is almost always what you want.
//
// The opt-out `ignore_parent_conditions: true` exists for the rare case where
// a terminate rule has intentionally different scope than the parent. The
// use case: the parent's conditions scope WHICH resources to track, but the
// terminate rule is a cleanup that should run on ANY closed resource the
// agent might have tracked — even ones that slipped through the scope filter
// via some earlier bug or config change.
//
// Scenario: the parent only tracks PRs targeting main. A PR closes that
// targets develop. Normally the terminate rule would be skipped via
// parent condition inheritance. With ignore_parent_conditions, the
// terminator fires anyway so the agent doesn't leak an orphan conversation.

func TestDispatch_Terminate_IgnoreParentConditions(t *testing.T) {
	harness := newHarness(t)

	parentConditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "pull_request.base.ref", Operator: "equals", Value: "main"},
		},
	}

	harness.addTerminateTrigger(
		[]string{"pull_request.opened"},
		parentConditions,
		nil,
		"",
		[]model.TerminateRule{
			{
				TriggerKeys:            []string{"pull_request.closed"},
				IgnoreParentConditions: true,
				Instructions:           "cleanup: PR #$refs.pull_number closed",
			},
		},
	)

	// Fire a close event on a PR targeting develop (not main). The parent
	// trigger's condition (base.ref equals main) would normally reject this
	// and the terminate rule would inherit the rejection — but with
	// ignore_parent_conditions, the terminate rule runs anyway.
	payload := loadFixture(t, "pull_request.opened.json")
	payload["action"] = "closed"
	patchPath(t, payload, "pull_request.base.ref", "develop")
	patchPath(t, payload, "pull_request.state", "closed")

	runs := harness.run("pull_request", "closed", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire via ignore_parent_conditions, got skip: %s", run.SkipReason)
	}
	if run.RunIntent != RunIntentTerminate {
		t.Errorf("expected terminate intent, got %q", run.RunIntent)
	}
	if run.Instructions != "cleanup: PR #2 closed" {
		t.Errorf("instructions = %q, want substituted cleanup message", run.Instructions)
	}
}

// --- Test 21: match: any combined with a required scope filter -------------
//
// The practical pattern where `any` really shines: one required scope
// condition paired with a set of alternative triggering signals. This is
// written as NESTED matches conceptually, but since the conditions layer
// only supports a single match mode, we have to choose — and `any` alone
// isn't enough because the scope filter must ALSO hold.
//
// The usual workaround: put the scope filter in one agent trigger row,
// and the OR alternatives in a second row. Both target the same agent,
// so either path wakes it up.
//
// This test covers the realistic pattern: TWO trigger rows on one agent,
// same connection, both listening on issues.opened. One has `match: all`
// for scope; the other has `match: any` for alternative signals. Firing
// one event should produce TWO matching runs if the event satisfies both
// filters.

func TestDispatch_MatchAny_MultiRowAgent(t *testing.T) {
	harness := newHarness(t)

	scopedRow := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "repository.owner.login", Operator: "equals", Value: "Codertocat"},
		},
	}
	anySignalRow := &model.TriggerMatch{
		Mode: "any",
		Conditions: []model.TriggerCondition{
			{Path: "issue.title", Operator: "contains", Value: "CRITICAL"},
			{Path: "issue.title", Operator: "contains", Value: "security"},
		},
	}

	harness.addTrigger([]string{"issues.opened"}, scopedRow, nil, "scoped")
	harness.addTrigger([]string{"issues.opened"}, anySignalRow, nil, "any-signal")

	t.Run("both rows fire when both filters pass", func(t *testing.T) {
		payload := loadFixture(t, "issues.opened.json")
		patchPath(t, payload, "issue.title", "CRITICAL: auth broken")

		runs := harness.run("issues", "opened", payload)
		if len(runs) != 2 {
			t.Fatalf("expected 2 runs (one per row), got %d", len(runs))
		}
		for _, run := range runs {
			if run.Skipped() {
				t.Errorf("run with instructions %q was skipped: %s", run.Instructions, run.SkipReason)
			}
		}
	})

	t.Run("only scoped row fires when title has no trigger words", func(t *testing.T) {
		payload := loadFixture(t, "issues.opened.json")
		patchPath(t, payload, "issue.title", "boring documentation update")

		runs := harness.run("issues", "opened", payload)
		if len(runs) != 2 {
			t.Fatalf("expected 2 runs (both match the store query), got %d", len(runs))
		}
		// One fires (the scoped row — org matches), one is skipped (the any row — no trigger words)
		fireCount := 0
		skipCount := 0
		for _, run := range runs {
			if run.Skipped() {
				skipCount++
			} else {
				fireCount++
			}
		}
		if fireCount != 1 || skipCount != 1 {
			t.Errorf("expected 1 fire + 1 skip, got %d fires, %d skips", fireCount, skipCount)
		}
	})
}

// --- helpers ---------------------------------------------------------------

// contextRequestNames extracts the `as` field from a list of context
// requests for assertion convenience.
func contextRequestNames(requests []ContextRequest) []string {
	out := make([]string, 0, len(requests))
	for _, request := range requests {
		out = append(out, request.As)
	}
	return out
}

// patchPath sets a value at a dot-path inside a JSON-decoded payload, creating
// intermediate maps as needed. Used by tests that derive a slightly-modified
// fixture (e.g. setting sender.login = "dependabot[bot]") without committing
// a separate fixture file for every variation.
func patchPath(t *testing.T, payload map[string]any, path string, value any) {
	t.Helper()
	segments := strings.Split(path, ".")
	current := payload
	for index := 0; index < len(segments)-1; index++ {
		segment := segments[index]
		next, ok := current[segment].(map[string]any)
		if !ok {
			t.Fatalf("patchPath %q: segment %q is not a map", path, segment)
		}
		current = next
	}
	current[segments[len(segments)-1]] = value
}
