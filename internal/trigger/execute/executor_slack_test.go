package execute

import (
	"strings"
	"testing"

	"github.com/ziraloop/ziraloop/internal/model"
)

// End-to-end Slack executor tests.
//
// These prove that the Slack catalog (triggers + actions + resources with
// coalescing refs) produces correct downstream behavior through the real
// dispatcher + executor pipeline. Only Nango is mocked — everything else
// runs production code paths.
//
// Like the GitHub suite, the load-bearing assertion is on
// `result.FinalInstructions` — the string the LLM would see as the opening
// message of the agent's conversation.

// --- Test 1: top-level @mention → full thread fetched via conversations.replies ---
//
// A user @mentions the bot at the top level of a channel. The agent
// config fetches the thread via conversations.replies + the user who
// mentioned via users.info. The final instruction contains both.

func TestExecute_Slack_AppMention_TopLevel(t *testing.T) {
	harness := newSlackPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "thread", Action: "conversations_replies", Ref: "slack_thread"},
		{As: "sender", Action: "users_info", Params: map[string]any{
			"user": "$refs.user",
		}},
	}

	instructions := `<@$refs.user> mentioned you in <#$refs.channel_id>.

Their message: $refs.text

## Sender
Name: {{$sender.user.real_name}}
Title: {{$sender.user.profile.title}}
Email: {{$sender.user.profile.email}}

## Full thread
{{$thread.messages}}

Read the thread and respond helpfully in the same thread.`

	harness.addAgentTrigger(
		[]string{"app_mention"},
		nil,
		contextActions,
		instructions,
	)

	// Stubs for every Nango call the executor will fire.
	// The conversations.replies call uses the slack_thread resource's
	// ref_bindings — channel + ts come from refs.channel_id + refs.thread_id
	// (thread_id is coalesced from event.thread_ts || event.ts, so for a
	// top-level mention it's event.ts = "1595926230.009600").
	harness.stubResponse("POST", "/conversations.replies", "conversations_replies.json")
	harness.stubResponse("POST", "/users.info", "users_info.json")

	result := harness.runPipeline("app_mention", "", "app_mention.top_level.json")

	if !result.IsExecutable() {
		t.Fatalf("expected executable, got: skipped=%v silent=%v reason=%q errors=%v",
			result.Skipped, result.SilentClose, result.SkipReason, result.ContextErrors)
	}

	// Two Nango calls: conversations.replies, users.info
	if harness.fakeNango.CallCount() != 2 {
		t.Errorf("expected 2 Nango calls, got %d", harness.fakeNango.CallCount())
	}

	// Verify the conversations.replies call received the correct body.
	// The slack_thread resource binds channel → $refs.channel_id and
	// ts → $refs.thread_id. For a top-level mention, thread_id coalesces to event.ts.
	replyCalls := harness.fakeNango.CallFor("POST", "/conversations.replies")
	if len(replyCalls) != 1 {
		t.Fatalf("expected 1 conversations.replies call, got %d", len(replyCalls))
	}
	reply := replyCalls[0]
	if reply.Body["channel"] != "C111" {
		t.Errorf("conversations.replies body.channel = %v, want C111", reply.Body["channel"])
	}
	if reply.Body["ts"] != "1595926230.009600" {
		t.Errorf("conversations.replies body.ts = %v, want thread root (1595926230.009600)", reply.Body["ts"])
	}

	// users.info call should have been given the mention sender's user ID.
	userInfoCalls := harness.fakeNango.CallFor("POST", "/users.info")
	if len(userInfoCalls) != 1 {
		t.Fatalf("expected 1 users.info call, got %d", len(userInfoCalls))
	}
	userCall := userInfoCalls[0]
	if userCall.Body["user"] != "W222" {
		t.Errorf("users.info body.user = %v, want W222", userCall.Body["user"])
	}

	// Final instruction assertions.
	final := result.FinalInstructions

	// $refs.x (dispatcher-substituted)
	if !strings.Contains(final, "<@W222> mentioned you in <#C111>") {
		t.Errorf("refs not substituted in greeting: %q", final)
	}
	if !strings.Contains(final, "Their message: <@W111> can you look at this issue?") {
		t.Errorf("text ref not substituted: %q", final)
	}

	// Deep dot-path into users.info response
	if !strings.Contains(final, "Name: Alice Example") {
		t.Errorf("sender.user.real_name missing: %q", final)
	}
	if !strings.Contains(final, "Title: Engineering Manager") {
		t.Errorf("sender.user.profile.title missing: %q", final)
	}
	if !strings.Contains(final, "Email: alice@example.com") {
		t.Errorf("sender.user.profile.email missing: %q", final)
	}

	// {{$thread.messages}} — JSON-stringified array from the conversations.replies response
	if !strings.Contains(final, "can you look at this issue?") {
		t.Errorf("thread content missing: %q", final)
	}
	if !strings.Contains(final, "any progress? The deploy is waiting") {
		t.Errorf("follow-up thread message missing: %q", final)
	}

	// Sanity: no leftover placeholders
	if strings.Contains(final, "{{") {
		t.Errorf("unresolved {{...}} placeholders: %q", final)
	}
	if strings.Contains(final, "$refs.") {
		t.Errorf("unresolved $refs.x references: %q", final)
	}
	if strings.Contains(final, "[missing:") {
		t.Errorf("missing-step markers: %q", final)
	}
}

// --- Test 2: in-thread mention → fetches thread from thread root, not the reply ---
//
// The coalescing ref is the crucial piece here: when the mention is inside
// an existing thread, the fixture has event.thread_ts = the thread root
// (not the reply's own ts). The slack_thread resource's template uses
// thread_id which coalesces to event.thread_ts when present. The executor's
// conversations.replies call must target the THREAD ROOT, so the agent
// fetches the whole conversation context, not just its own reply.

func TestExecute_Slack_AppMention_InThread(t *testing.T) {
	harness := newSlackPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "thread", Action: "conversations_replies", Ref: "slack_thread"},
	}

	instructions := "Thread root: $refs.thread_id\nThread messages: {{$thread.messages}}"

	harness.addAgentTrigger(
		[]string{"app_mention"},
		nil,
		contextActions,
		instructions,
	)

	harness.stubResponse("POST", "/conversations.replies", "conversations_replies.json")

	result := harness.runPipeline("app_mention", "", "app_mention.in_thread.json")

	if !result.IsExecutable() {
		t.Fatalf("expected executable, got errors: %v reason=%q", result.ContextErrors, result.SkipReason)
	}

	// The fixture has:
	//   event.ts = 1595926540.012400 (the reply's own ts)
	//   event.thread_ts = 1595926230.009600 (the thread root)
	// The coalescing ref should prefer event.thread_ts, so the resolved
	// ts param to conversations.replies MUST be 1595926230.009600.
	replyCalls := harness.fakeNango.CallFor("POST", "/conversations.replies")
	if len(replyCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(replyCalls))
	}
	if replyCalls[0].Body["ts"] != "1595926230.009600" {
		t.Errorf("conversations.replies body.ts = %v, want thread root 1595926230.009600 (NOT reply ts 1595926540.012400)",
			replyCalls[0].Body["ts"])
	}

	// The $refs.thread_id in the instructions should also be the root.
	if !strings.Contains(result.FinalInstructions, "Thread root: 1595926230.009600") {
		t.Errorf("thread_id ref wrong in final: %q", result.FinalInstructions)
	}
}

// --- Test 3: DM with explicit-param context action ---
//
// A direct message (channel_type = im) fires message.im. The agent fetches
// conversation history via conversations_history with explicit params
// (since the slack_thread resource bindings only cover channel + ts, and
// history needs limit/latest/oldest). Verifies that explicit params in a
// context action work alongside $refs.x substitution.

func TestExecute_Slack_DMWithHistory(t *testing.T) {
	harness := newSlackPipelineHarness(t)

	contextActions := []model.ContextAction{
		{As: "history", Action: "conversations_history", Params: map[string]any{
			"channel": "$refs.channel_id",
			"limit":   "50",
		}},
	}

	instructions := `A customer DM'd you: $refs.text

## Recent conversation history
{{$history.messages}}

Draft a helpful response.`

	harness.addAgentTrigger(
		[]string{"message.im"},
		nil,
		contextActions,
		instructions,
	)

	harness.stubResponse("POST", "/conversations.history", "conversations_history_dm.json")

	result := harness.runPipeline("message", "im", "message.im.json")

	if !result.IsExecutable() {
		t.Fatalf("expected executable, got errors: %v", result.ContextErrors)
	}

	// Verify the history call received the correct channel from the DM fixture.
	historyCalls := harness.fakeNango.CallFor("POST", "/conversations.history")
	if len(historyCalls) != 1 {
		t.Fatalf("expected 1 conversations.history call, got %d", len(historyCalls))
	}
	call := historyCalls[0]
	if call.Body["channel"] != "D024BE91L" {
		t.Errorf("history body.channel = %v, want D024BE91L", call.Body["channel"])
	}
	if call.Body["limit"] != "50" {
		t.Errorf("history body.limit = %v, want 50", call.Body["limit"])
	}

	// Final instruction checks.
	final := result.FinalInstructions
	if !strings.Contains(final, "A customer DM'd you: Hello hello can you hear me?") {
		t.Errorf("DM text ref missing: %q", final)
	}
	if !strings.Contains(final, "Actually, I have a question about pricing") {
		t.Errorf("history content missing: %q", final)
	}
	if !strings.Contains(final, "Hi there, are you the support bot?") {
		t.Errorf("older history message missing: %q", final)
	}
}
