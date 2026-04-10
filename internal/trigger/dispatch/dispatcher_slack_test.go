package dispatch

import (
	"strings"
	"testing"

	"github.com/ziraloop/ziraloop/internal/model"
)

// Slack dispatcher tests.
//
// These validate that the dispatcher's context-recipe logic works correctly
// for Slack — a provider that uses coalescing refs to unify top-level messages
// and in-thread messages into the same resource key. The tests mirror the
// GitHub suite in style: one trigger config per test, a real webhook fixture
// from testdata/slack/, assertions on the resulting PreparedRun.
//
// The fixtures are sourced from slackapi/bolt-python test fixtures and
// docs.slack.dev official examples. See testdata/slack/SOURCES.md for the
// full provenance and the "thread_ts asymmetry" that makes coalescing refs
// necessary.
//
// Every test exercises a different facet of the Slack trigger path:
//
//   1. TopLevel_HappyPath       — basic dispatch, refs, context action, resource key via event.ts
//   2. InThread_HappyPath       — in-thread mention, resource key via event.thread_ts
//   3. Continuation             — top-level + in-thread reply produce the SAME resource key
//   4. BotExcluded              — self-exclusion condition filters the bot's own mention
//   5. MessageIM                — direct message, different channel_type, context still resolves
//   6. MessageChannels_Thread   — a thread reply via message.channels joins the mention's conversation
//   7. ReactionAdded            — reaction trigger, resource key from item.channel/item.ts
//   8. MemberJoined_NoKey       — member_joined_channel uses slack_channel resource (no template), empty key

// --- Test 1: top-level @mention happy path -------------------------------
//
// A user @mentions the bot as a top-level message in a channel (no
// event.thread_ts). The coalescing ref resolves thread_id to event.ts so
// the mention establishes a new thread root. The context action fetches
// the thread's replies using conversations.replies with channel + ts.

func TestDispatch_Slack_AppMention_TopLevel_HappyPath(t *testing.T) {
	harness := newSlackHarness(t)

	contextActions := []model.ContextAction{
		{As: "thread", Action: "conversations_replies", Ref: "slack_thread"},
	}
	instructions := "User $refs.user mentioned you in channel $refs.channel_id. Message: $refs.text"

	harness.addTrigger(
		[]string{"app_mention"},
		nil,
		contextActions,
		instructions,
	)

	payload := loadSlackFixture(t, "app_mention.top_level.json")
	runs := harness.run("app_mention", "", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected run to fire, got skip: %s", run.SkipReason)
	}
	if run.TriggerKey != "app_mention" {
		t.Errorf("trigger key = %q, want app_mention", run.TriggerKey)
	}

	// Refs come from the fixture: team T111, channel C111, user W222, ts 1595926230.009600.
	// The thread_id ref coalesces event.thread_ts || event.ts. Top-level
	// mentions don't have event.thread_ts, so it resolves to event.ts.
	wantRefs := map[string]string{
		"team_id":    "T111",
		"channel_id": "C111",
		"user":       "W222",
		"message_ts": "1595926230.009600",
		"thread_id":  "1595926230.009600", // coalesced from event.ts
	}
	for refName, wantValue := range wantRefs {
		if got := run.Refs[refName]; got != wantValue {
			t.Errorf("refs[%q] = %q, want %q", refName, got, wantValue)
		}
	}

	// Resource key uses team + channel + thread_id → stable across thread events.
	wantResourceKey := "slack:T111:C111:1595926230.009600"
	if run.ResourceKey != wantResourceKey {
		t.Errorf("resource key = %q, want %q", run.ResourceKey, wantResourceKey)
	}

	// Instructions have $refs.x substituted.
	if !strings.Contains(run.Instructions, "W222 mentioned you in channel C111") {
		t.Errorf("instructions did not substitute refs: %q", run.Instructions)
	}

	// Context request: conversations.replies with channel + ts from the thread resource's ref_bindings.
	thread := assertContextRequest(t, run, "thread")
	if thread.ActionKey != "conversations_replies" {
		t.Errorf("action key = %q", thread.ActionKey)
	}
	if thread.Method != "POST" {
		t.Errorf("method = %q, want POST (Slack uses POST for everything)", thread.Method)
	}
	if thread.Path != "/conversations.replies" {
		t.Errorf("path = %q, want /conversations.replies", thread.Path)
	}
	// conversations.replies uses body_mapping for its params (Slack Web API convention).
	if thread.Body["channel"] != "C111" {
		t.Errorf("body.channel = %v, want C111", thread.Body["channel"])
	}
	if thread.Body["ts"] != "1595926230.009600" {
		t.Errorf("body.ts = %v, want 1595926230.009600 (thread root)", thread.Body["ts"])
	}
}

// --- Test 2: in-thread @mention happy path -------------------------------
//
// A user @mentions the bot as a reply inside an existing thread. The
// fixture has both event.ts (the reply's own timestamp) and event.thread_ts
// (the parent thread root). Coalescing must prefer event.thread_ts so the
// resource key identifies the THREAD, not the individual reply.

func TestDispatch_Slack_AppMention_InThread_HappyPath(t *testing.T) {
	harness := newSlackHarness(t)

	harness.addTrigger(
		[]string{"app_mention"},
		nil,
		[]model.ContextAction{
			{As: "thread", Action: "conversations_replies", Ref: "slack_thread"},
		},
		"follow-up in thread $refs.thread_id",
	)

	payload := loadSlackFixture(t, "app_mention.in_thread.json")
	runs := harness.run("app_mention", "", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}

	// The reply's OWN ts is 1595926540.012400, but the thread's root is
	// 1595926230.009600 (in event.thread_ts). Coalescing must pick the
	// thread_ts value so the resource key identifies the thread, not the reply.
	if run.Refs["message_ts"] != "1595926540.012400" {
		t.Errorf("message_ts = %q, want the reply's own ts", run.Refs["message_ts"])
	}
	if run.Refs["thread_id"] != "1595926230.009600" {
		t.Errorf("thread_id = %q, want the thread root ts (coalesced from event.thread_ts)", run.Refs["thread_id"])
	}

	wantResourceKey := "slack:T111:C111:1595926230.009600"
	if run.ResourceKey != wantResourceKey {
		t.Errorf("resource key = %q, want %q", run.ResourceKey, wantResourceKey)
	}

	// The context action targets the THREAD root, not the reply — so replies
	// fetch the whole thread, not just themselves.
	thread := assertContextRequest(t, run, "thread")
	if thread.Body["ts"] != "1595926230.009600" {
		t.Errorf("thread.ts = %v, want thread root (not reply ts)", thread.Body["ts"])
	}
}

// --- Test 3: Continuation across event variants --------------------------
//
// THE critical test for Slack. A top-level mention and a follow-up in-thread
// mention on the same thread must produce the SAME resource key, because the
// executor's continuation lookup is an exact match on (agent, connection,
// resource_key). If coalescing isn't working correctly, this test fails and
// Slack agents lose their conversations between events.

func TestDispatch_Slack_AppMention_Continuation(t *testing.T) {
	harness := newSlackHarness(t)

	harness.addTrigger(
		[]string{"app_mention"},
		nil,
		nil,
		"",
	)

	topLevelPayload := loadSlackFixture(t, "app_mention.top_level.json")
	inThreadPayload := loadSlackFixture(t, "app_mention.in_thread.json")

	topLevelRun := assertSinglePrepared(t, harness.run("app_mention", "", topLevelPayload))
	inThreadRun := assertSinglePrepared(t, harness.run("app_mention", "", inThreadPayload))

	if topLevelRun.ResourceKey == "" || inThreadRun.ResourceKey == "" {
		t.Fatalf("both runs must have a resource key: top=%q in_thread=%q",
			topLevelRun.ResourceKey, inThreadRun.ResourceKey)
	}
	if topLevelRun.ResourceKey != inThreadRun.ResourceKey {
		t.Errorf("resource keys diverged — continuation is broken:\n  top_level  = %q\n  in_thread  = %q",
			topLevelRun.ResourceKey, inThreadRun.ResourceKey)
	}

	// Both should be the thread-root identifier (top-level's event.ts).
	wantKey := "slack:T111:C111:1595926230.009600"
	if topLevelRun.ResourceKey != wantKey {
		t.Errorf("top-level resource key = %q, want %q", topLevelRun.ResourceKey, wantKey)
	}
}

// --- Test 4: self-exclusion filters the bot's own mentions ----------------
//
// A mention where event.user matches the bot's user ID should be filtered
// out by a self-exclusion condition. Without this, the bot replies to its
// own messages in an infinite loop. The equivalent of GitHub's
// "sender.login not_equals bot-name" pattern, for Slack.

func TestDispatch_Slack_AppMention_BotExcluded(t *testing.T) {
	harness := newSlackHarness(t)

	conditions := &model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{
			{Path: "event.user", Operator: "not_equals", Value: "W222"},
		},
	}

	harness.addTrigger(
		[]string{"app_mention"},
		conditions,
		nil,
		"",
	)

	// The fixture has event.user = "W222", which matches the exclusion.
	payload := loadSlackFixture(t, "app_mention.top_level.json")
	runs := harness.run("app_mention", "", payload)
	run := assertSinglePrepared(t, runs)

	if !run.Skipped() {
		t.Fatalf("expected skip, got fire")
	}
	if !strings.Contains(run.SkipReason, "event.user") {
		t.Errorf("skip reason should mention event.user, got %q", run.SkipReason)
	}
	if !strings.Contains(run.SkipReason, "not_equals") {
		t.Errorf("skip reason should name operator, got %q", run.SkipReason)
	}

	// Refs are still populated on the skipped run — debugging visibility.
	if run.Refs["user"] != "W222" {
		t.Errorf("refs should still be populated on skip, got %v", run.Refs)
	}
}

// --- Test 5: direct message (channel_type = im) --------------------------
//
// A DM to the bot fires message.im. The channel is a D... ID (direct
// message conversation), not a channel. The resource key still uses the
// coalesced thread_id (which is event.ts for DMs since DMs are flat).

func TestDispatch_Slack_MessageIM(t *testing.T) {
	harness := newSlackHarness(t)

	harness.addTrigger(
		[]string{"message.im"},
		nil,
		[]model.ContextAction{
			{As: "history", Action: "conversations_history", Params: map[string]any{
				"channel": "$refs.channel_id",
				"limit":   "20",
			}},
		},
		"DM from $refs.user: $refs.text",
	)

	payload := loadSlackFixture(t, "message.im.json")
	runs := harness.run("message", "im", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}

	// Channel is a D... ID for DMs.
	if run.Refs["channel_id"] != "D024BE91L" {
		t.Errorf("channel_id = %q, want D024BE91L", run.Refs["channel_id"])
	}
	if run.Refs["channel_type"] != "im" {
		t.Errorf("channel_type = %q, want im", run.Refs["channel_type"])
	}

	// Resource key for DMs — DMs don't have thread_ts, so thread_id falls through to event.ts.
	wantKey := "slack:T123ABC456:D024BE91L:1355517523.000005"
	if run.ResourceKey != wantKey {
		t.Errorf("resource key = %q, want %q", run.ResourceKey, wantKey)
	}

	// Context action built correctly with the DM channel.
	history := assertContextRequest(t, run, "history")
	if history.Body["channel"] != "D024BE91L" {
		t.Errorf("history.channel = %v", history.Body["channel"])
	}
	if history.Body["limit"] != "20" {
		t.Errorf("history.limit = %v", history.Body["limit"])
	}
}

// --- Test 6: thread reply via message.channels continues the mention's thread ---
//
// This test exercises the "continuation-only listener" pattern. An agent
// listens to BOTH app_mention (to start work) and message.channels (to
// receive follow-up replies). The message.channels fixture is a thread
// reply to the same thread root as the top-level mention fixture. Both
// events should produce the same resource_key — meaning the executor would
// route them to the same conversation.

func TestDispatch_Slack_MessageChannels_ThreadReply(t *testing.T) {
	harness := newSlackHarness(t)

	harness.addTrigger(
		[]string{"app_mention", "message.channels"},
		nil,
		nil,
		"",
	)

	mentionRun := assertSinglePrepared(t, harness.run("app_mention", "", loadSlackFixture(t, "app_mention.top_level.json")))
	replyRun := assertSinglePrepared(t, harness.run("message", "channels", loadSlackFixture(t, "message.channels.thread_reply.json")))

	// Both must produce the same resource key to route to the same conversation.
	if mentionRun.ResourceKey != replyRun.ResourceKey {
		t.Errorf("resource key mismatch — mention and thread reply should converge:\n  mention = %q\n  reply   = %q",
			mentionRun.ResourceKey, replyRun.ResourceKey)
	}

	// The reply's trigger key is "message.channels".
	if replyRun.TriggerKey != "message.channels" {
		t.Errorf("reply trigger key = %q, want message.channels", replyRun.TriggerKey)
	}

	// The reply's message_ts is its own ts; thread_id is the parent's.
	if replyRun.Refs["message_ts"] != "1595926780.018000" {
		t.Errorf("reply message_ts = %q", replyRun.Refs["message_ts"])
	}
	if replyRun.Refs["thread_id"] != "1595926230.009600" {
		t.Errorf("reply thread_id = %q, want thread root", replyRun.Refs["thread_id"])
	}
}

// --- Test 7: reaction_added uses item.channel and item.ts as the thread key ---
//
// Reactions don't have event.channel directly — they have event.item.channel
// (the reacted-to message's channel). The Slack catalog maps channel_id and
// thread_id refs to these nested fields. The resulting resource key is
// the thread containing the reacted-to message, which means reaction events
// continue the same conversation as other events in that thread.

func TestDispatch_Slack_ReactionAdded(t *testing.T) {
	harness := newSlackHarness(t)

	harness.addTrigger(
		[]string{"reaction_added"},
		&model.TriggerMatch{
			Mode: "all",
			Conditions: []model.TriggerCondition{
				{Path: "event.reaction", Operator: "one_of", Value: []any{"heart_eyes", "thumbsup", "eyes"}},
			},
		},
		nil,
		"Reaction: $refs.reaction on message $refs.item_ts",
	)

	payload := loadSlackFixture(t, "reaction_added.json")
	runs := harness.run("reaction_added", "", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}
	if run.Refs["reaction"] != "heart_eyes" {
		t.Errorf("reaction = %q", run.Refs["reaction"])
	}
	if run.Refs["item_channel"] != "C111" {
		t.Errorf("item_channel = %q", run.Refs["item_channel"])
	}
	if run.Refs["item_ts"] != "1599529504.000400" {
		t.Errorf("item_ts = %q", run.Refs["item_ts"])
	}

	// Resource key uses channel_id (mapped to item.channel) + thread_id
	// (mapped to item.ts). Events on this same message — another reaction,
	// a reply, another mention in its thread — all resolve to the same key.
	wantKey := "slack:T111:C111:1599529504.000400"
	if run.ResourceKey != wantKey {
		t.Errorf("resource key = %q, want %q", run.ResourceKey, wantKey)
	}

	if !strings.Contains(run.Instructions, "heart_eyes on message 1599529504.000400") {
		t.Errorf("instructions not substituted: %q", run.Instructions)
	}
}

// --- Test 8: member_joined_channel has no continuation (slack_channel resource) ---
//
// member_joined_channel points at the slack_channel resource, which has no
// resource_key_template. Every join event produces an empty resource_key,
// which means the executor always creates a new conversation — correct
// behavior for welcome-flow agents where each join is a distinct event
// with no ongoing "channel conversation" to continue.

func TestDispatch_Slack_MemberJoined_NoContinuation(t *testing.T) {
	harness := newSlackHarness(t)

	harness.addTrigger(
		[]string{"member_joined_channel"},
		nil,
		[]model.ContextAction{
			{As: "channel_info", Action: "conversations_info", Ref: "slack_channel"},
			{As: "user_info", Action: "users_info", Ref: "slack_user"},
		},
		"$refs.user joined $refs.channel_id",
	)

	payload := loadSlackFixture(t, "member_joined_channel.json")
	runs := harness.run("member_joined_channel", "", payload)
	run := assertSinglePrepared(t, runs)

	if run.Skipped() {
		t.Fatalf("expected fire, got skip: %s", run.SkipReason)
	}

	// slack_channel has no resource_key_template — empty means "always new conversation."
	if run.ResourceKey != "" {
		t.Errorf("resource key = %q, want empty (slack_channel has no template)", run.ResourceKey)
	}

	if run.Refs["user"] != "W23456789" {
		t.Errorf("user = %q", run.Refs["user"])
	}
	if run.Refs["channel_id"] != "C111" {
		t.Errorf("channel_id = %q", run.Refs["channel_id"])
	}

	// Two context actions: one on the channel, one on the user.
	channelInfo := assertContextRequest(t, run, "channel_info")
	if channelInfo.Body["channel"] != "C111" {
		t.Errorf("channel_info.channel = %v", channelInfo.Body["channel"])
	}
	userInfo := assertContextRequest(t, run, "user_info")
	if userInfo.Body["user"] != "W23456789" {
		t.Errorf("user_info.user = %v", userInfo.Body["user"])
	}

	// Instructions substituted from refs.
	if run.Instructions != "W23456789 joined C111" {
		t.Errorf("instructions = %q", run.Instructions)
	}
}
