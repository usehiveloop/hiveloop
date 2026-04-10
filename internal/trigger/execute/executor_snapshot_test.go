package execute

import (
	"fmt"
	"testing"

	"github.com/ziraloop/ziraloop/internal/model"
)

// Snapshot tests — the goal of these tests is to make the FINAL BUILT
// INSTRUCTION visible to the reviewer. Unlike the substring-assertion
// tests, these print the whole assembled prompt to test output so you can
// read it and judge whether it's actually the kind of thing you'd hand to
// an LLM.
//
// Run with: go test -run TestSnapshot -v ./internal/trigger/execute/
//
// These are NOT golden-file assertions — they just pass if the pipeline
// runs without error and print the output. The substring tests elsewhere
// in the suite catch regressions; these exist so humans can see what the
// LLM would receive.

// Run with: go test -run TestSnapshot_GitHub_PRReviewer -v
func TestSnapshot_GitHub_PRReviewer(t *testing.T) {
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
**{{$pr.title}}** (#{{$pr.number}}) by {{$pr.user.login}}
State: {{$pr.state}} · Draft: {{$pr.draft}} · Mergeable: {{$pr.mergeable_state}}

{{$pr.body}}

## Changed files
{{$files}}

## Repository guidelines (optional — empty if the repo has no CONTRIBUTING.md)
Path: {{$guidelines.path}}
Size: {{$guidelines.size}} bytes

---

Review the changes against the guidelines. Leave exactly one summary review
comment covering style violations, obvious bugs, and missing test coverage.
If nothing needs fixing, post a single approval comment and stop. Do NOT
leave inline comments on individual files.`

	harness.addAgentTrigger(
		[]string{"pull_request.opened"},
		nil,
		contextActions,
		instructions,
	)

	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/pulls/2", "pulls_get.json")
	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/pulls/2/files", "pulls_list_files.json")
	harness.stubResponse("GET", "/repos/Codertocat/Hello-World/contents/.github/CONTRIBUTING.md", "repos_get_content_contributing.json")

	result := harness.runPipeline("pull_request", "opened", "pull_request.opened.json")
	if !result.IsExecutable() {
		t.Fatalf("pipeline failed: skipped=%v reason=%q errors=%v",
			result.Skipped, result.SkipReason, result.ContextErrors)
	}

	fmt.Println()
	fmt.Println("================================================================")
	fmt.Println("SNAPSHOT: GitHub PR reviewer — final instruction for the LLM")
	fmt.Println("================================================================")
	fmt.Println(result.FinalInstructions)
	fmt.Println("================================================================")
	fmt.Println()
}

// Run with: go test -run TestSnapshot_Slack_ThreadResponder -v
func TestSnapshot_Slack_ThreadResponder(t *testing.T) {
	harness := newSlackPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "thread", Action: "conversations_replies", Ref: "slack_thread"},
		{As: "sender", Action: "users_info", Params: map[string]any{
			"user": "$refs.user",
		}},
	}

	instructions := `<@$refs.user> mentioned you in <#$refs.channel_id>.

## Sender
{{$sender.user.real_name}} ({{$sender.user.profile.title}}) — {{$sender.user.profile.email}}

## Mention text
> $refs.text

## Full thread context
{{$thread.messages}}

---

Read the thread carefully. Figure out what task you've been asked to work on.
Your system prompt describes how to go from a task to a pull request — follow
that process. Post a status reply in this Slack thread acknowledging what
you're starting, then begin working.`

	harness.addAgentTrigger(
		[]string{"app_mention"},
		nil,
		contextActions,
		instructions,
	)

	harness.stubResponse("POST", "/conversations.replies", "conversations_replies.json")
	harness.stubResponse("POST", "/users.info", "users_info.json")

	result := harness.runPipeline("app_mention", "", "app_mention.top_level.json")
	if !result.IsExecutable() {
		t.Fatalf("pipeline failed: skipped=%v reason=%q errors=%v",
			result.Skipped, result.SkipReason, result.ContextErrors)
	}

	fmt.Println()
	fmt.Println("================================================================")
	fmt.Println("SNAPSHOT: Slack @mention responder — final instruction for the LLM")
	fmt.Println("================================================================")
	fmt.Println(result.FinalInstructions)
	fmt.Println("================================================================")
	fmt.Println()
}
