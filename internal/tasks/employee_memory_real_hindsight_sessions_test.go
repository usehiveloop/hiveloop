package tasks

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type productionSession struct {
	user   string
	text   string
	tools  []string
	reply  string
	noise  []string
	source string
}

func productionMemorySessions() []productionSession {
	return []productionSession{
		{user: "Kim", text: "Can you check why the API deploy is red? The Railway deploy says migrations failed but I haven't opened the logs yet.", tools: []string{"bash: railway logs --service api", "read_file: checked latest migration"}, reply: "Migration 2026051201 is failing on a missing index. I’ll patch the migration and rerun the focused DB test.", noise: []string{"one sec", "threading here"}},
		{user: "Maya", text: "The GitHub repo picker modal feels weird. It should match the existing onboarding selection pattern, not look like a settings table.", tools: []string{"read_file: inspected onboarding choice card usage", "edit_file: changed repository picker to choice-card layout"}, reply: "Updated to use the existing choice-card pattern. Private repos show a lock icon.", noise: []string{"hmm", "closer"}},
		{user: "Nora", text: "The Stripe retry job may have double-deducted credits for one org. Please verify before touching anything.", tools: []string{"bash: queried credit ledger rows by reference id", "read_file: checked credit ledger service"}, reply: "No second deduction found. The retry reused the same stable reference id, so Postgres blocked a duplicate write.", noise: []string{"okay", "good"}},
		{user: "Daniel", text: "Can you look at the employee bridge build? The sandbox booted but healthz refused the connection.", tools: []string{"bash: checked /proc/1/environ", "bash: ran employee-bridge --help"}, reply: "The binary required a newer glibc than the image had. We need the Debian 13 runtime image for this build.", noise: []string{"classic", "ugh"}},
		{user: "Aisha", text: "For production deploys, Platform requires rollback notes before merge and a named owner for the deploy window. Add that to the checklist while you're in there.", tools: []string{"read_file: checked deploy checklist", "edit_file: added rollback note and deploy-owner fields"}, reply: "Done. The deploy checklist now requires rollback notes and a named deploy owner.", noise: []string{"thanks"}},
		{user: "Kim", text: "The Slack bot answered but then failed two turns later. Can you find the actual error from the sandbox?", tools: []string{"bash: sqlite3 /app/data/employee-bridge.db selected latest error events", "bash: checked process env"}, reply: "Found it. The model call hit HTTP 402 insufficient credits through the proxy, not a Slack delivery issue.", noise: []string{"ah", "makes sense"}},
		{user: "Maya", text: "This onboarding screen is stuck when org creation returns duplicate name. It shouldn't pretend we're still launching.", tools: []string{"read_file: checked onboarding mutation", "edit_file: surfaced create-org error state"}, reply: "Fixed. Duplicate org-name errors now stop the launch state and show the API error.", noise: []string{"nice"}},
		{user: "Nora", text: "If anything touches billing or credits, I want DB-level idempotency, not just handler checks.", tools: []string{"read_file: inspected billing migration", "bash: verified unique index coverage"}, reply: "Confirmed. Billing and credits changes need Postgres-backed idempotency before release.", noise: []string{"please remember this"}},
		{user: "Daniel", text: "Can you dispatch browser tests for the new topbar? I don't want the main sandbox stuck running Playwright for ten minutes.", tools: []string{"specialist_launch_task: launched topbar browser regression task", "specialist_task_status: checked initial status"}, reply: "Dispatched. I’ll monitor it and keep this thread for coordination.", noise: []string{"ty"}},
		{user: "Aisha", text: "The employee sandbox must use proxy.usehivy.com with a ptok token. If OPENROUTER_API_KEY appears in sandbox env, that's a release blocker.", tools: []string{"read_file: checked employee runtime compile env", "bash: verified sandbox env var names"}, reply: "Verified. Sandbox env uses HIVY_PROXY_API_KEY and no provider key.", noise: []string{"important"}},
		{user: "Kim", text: "Small style thing: don't write a paragraph when the answer is just 'Done'. You sound like support chat sometimes.", tools: []string{"memory_retain: retained communication preference"}, reply: "Done. Shorter, more direct.", noise: []string{"+1", "exactly"}},
		{user: "Maya", text: "Can you check why switching workspaces still shows onboarding even though the org is onboarded?", tools: []string{"read_file: inspected active workspace state", "edit_file: refetched onboarding status after workspace switch"}, reply: "Fixed. Workspace switching now refetches onboarding state before rendering the flow.", noise: []string{"works locally"}},
		{user: "Nora", text: "The invoice-failure alert is mine. If it fires again, loop me in first and include the org id plus failed invoice id.", tools: []string{"search_knowledge_base: found invoice alert runbook", "post_status_update: drafted compact incident handoff"}, reply: "Got it. Nora is first contact for invoice-failure alerts, with org id and failed invoice id included.", noise: []string{"ok"}},
		{user: "Daniel", text: "Employee runtime changes need integration tests. Unit tests alone are not enough there.", tools: []string{"bash: ran go test ./internal/employeeruntime", "edit_file: added compile integration coverage"}, reply: "Added integration coverage for employee runtime compile.", noise: []string{"ship it maybe"}},
		{user: "Aisha", text: "Can you clean the old benchmark data before committing? Keep db-todos.txt only.", tools: []string{"bash: listed benchmark files", "bash: removed generated bench artifacts"}, reply: "Cleaned. Only db-todos.txt remains from that batch.", noise: []string{"thanks"}},
		{user: "Kim", text: "The agent was too confident about production state without checking logs. Please make sure claims cite the source: logs, DB row, API response, or tool output.", tools: []string{"bash: queried production logs", "post_status_update: cited the relevant log event"}, reply: "Noted. Production claims need source evidence.", noise: []string{"yep"}},
		{user: "Maya", text: "This repo picker is closer but the selected state still isn't obvious.", tools: []string{"read_file: inspected choice-card selected styling", "edit_file: aligned selected repository card state"}, reply: "Selected repos now use the same border and check treatment as onboarding cards.", noise: []string{"better"}},
		{user: "Nora", text: "Can you check whether the webhook retry is safe if Stripe sends the same event three times?", tools: []string{"bash: ran webhook idempotency test", "read_file: checked webhook event table"}, reply: "Safe. Duplicate webhook event ids hit the unique constraint and return the existing processing result.", noise: []string{"great"}},
		{user: "Daniel", text: "The Rust bridge release should run cargo check across the workspace before we publish the image.", tools: []string{"bash: cargo check --workspace", "bash: cargo test -p employee-bridge"}, reply: "Passed cargo check workspace and employee bridge tests.", noise: []string{"ship"}},
		{user: "Aisha", text: "Don't retain pasted secrets. If someone drops a token in Slack, ignore the value and only remember the policy if it matters.", tools: []string{"memory_retain: retained secret-handling policy without secret value"}, reply: "Understood. Secret values are never memory content.", noise: []string{"please"}},
		{user: "Kim", text: "Can you compare prod DB schema to local and write the cleanup todos?", tools: []string{"bash: inspected prod schema metadata", "write_file: wrote db-todos.txt"}, reply: "Done. db-todos.txt has the schema cleanup findings.", noise: []string{"ty"}},
		{user: "Maya", text: "Vague feedback: the employees page feels off. I don't have better words yet.", tools: []string{"read_file: checked exploration page structure", "edit_file: tightened employees page spacing and primary action hierarchy"}, reply: "I made the smallest defensible layout improvement: clearer page title, tighter table, stronger primary action.", noise: []string{"yeah that's closer"}},
		{user: "Nora", text: "Subscriptions and credits are fragile. Please keep those patches small and focused, with targeted tests.", tools: []string{"bash: ran subscription focused tests", "bash: ran credit ledger focused tests"}, reply: "Done. Small patch, focused tests only.", noise: []string{"yes"}},
		{user: "Daniel", text: "When the bridge posts 'Sorry, something went wrong', first check the sandbox SQLite event log before guessing.", tools: []string{"bash: sqlite3 /app/data/employee-bridge.db selected latest error events"}, reply: "Confirmed. Sandbox SQLite event log is the first stop for generic bridge errors.", noise: []string{"handy"}},
		{user: "Aisha", text: "Production auth testing uses OTP to JWT. Don't put real JWTs in files, logs, commits, or test fixtures.", tools: []string{"read_file: checked auth smoke script", "edit_file: removed persisted jwt example"}, reply: "Removed the persisted JWT example. Auth smoke tests now read it from the shell only.", noise: []string{"good"}},
		{user: "Kim", text: "Can you list the five most recent initiatives from Linear again? Use the cheaper path this time if possible.", tools: []string{"skill_view: loaded linear-graphql", "bash: queried Linear GraphQL and filtered with jq"}, reply: "Pulled the five most recent initiatives with GraphQL and jq-filtered fields only.", noise: []string{"nice"}},
		{user: "Maya", text: "The settings menu in the topbar should not depend on SidebarProvider anymore.", tools: []string{"read_file: inspected settings menu", "edit_file: made account menu header-safe"}, reply: "Fixed. The topbar account menu no longer depends on sidebar context.", noise: []string{"ok"}},
		{user: "Nora", text: "Can you check the failed release job before we cut another patch release?", tools: []string{"bash: gh run view --log", "edit_file: fixed ghcr release workflow tag"}, reply: "Release workflow failed on the image tag. I patched the GHCR tag and reran checks.", noise: []string{"cool"}},
		{user: "Daniel", text: "If a job is expected to take more than a few minutes, don't do it inline. Launch a specialist and monitor it.", tools: []string{"specialist_launch_task: launched long-running docker runtime verification", "specialist_task_status: checked task status"}, reply: "Dispatched and monitoring. This thread stays for decisions and status.", noise: []string{"ok"}},
		{user: "Aisha", text: "If Hindsight is down, Slack responses should continue. Memory recall failure can't break a user turn.", tools: []string{"read_file: checked compile fallback", "edit_file: added failure-path test"}, reply: "Covered. Hindsight recall failure degrades to empty memory context.", noise: []string{"good"}},
		{user: "Kim", text: "Aria joke break: why did the database admin leave the party? Too many relationships.", tools: nil, reply: "Painfully relational. I respect it.", noise: []string{"lol", "bad but good", "😂"}},
		{user: "Maya", text: "Important question, not work: is pineapple on pizza a deploy blocker?", tools: nil, reply: "Only if it reaches production without rollback notes.", noise: []string{"hahaha", "fair"}},
		{user: "Nora", text: "Lunch thread: I vote ramen. Also someone please hide the office coffee machine before it files an incident.", tools: nil, reply: "Ramen has quorum. Coffee machine remains under observation.", noise: []string{"same", "coffee slander"}},
		{user: "Daniel", text: "Can you roast this function name: doThingButActuallyMaybeDoOtherThing?", tools: nil, reply: "That name is a cry for architectural help.", noise: []string{"accurate", "brutal"}},
		{user: "Aisha", text: "Friday playlist thread. Add anything except lo-fi beats named 'deep focus engineering monk'.", tools: nil, reply: "Noted for the vibe committee, not memory.", noise: []string{"add afrobeats", "yes"}},
		{user: "Kim", text: "If our rubber duck starts approving PRs, do we give it admin access?", tools: nil, reply: "No. It gets review rights and emotional support responsibilities only.", noise: []string{"quack", "ship it duck"}},
		{user: "Maya", text: "Someone make the staging banner pirate-themed. Actually don't. Unless?", tools: nil, reply: "Tempting, but staging gets clarity before comedy.", noise: []string{"yarr", "lol"}},
		{user: "Nora", text: "Meme check: the bug is not reproducible if nobody is brave enough to reproduce it.", tools: nil, reply: "Strong meme. Weak incident policy.", noise: []string{"exactly", "😂"}},
	}
}

func (s productionSession) toEvents(t *testing.T, orgID, agentID, sandboxID uuid.UUID, sessionID string, index int) []model.EmployeeMemoryEvent {
	t.Helper()
	source := s.source
	if source == "" {
		source = "slack"
	}
	events := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, sessionID, "user.message.received", map[string]any{
			"source": source, "channel": "C123", "thread_ts": "1770000000." + uuid.NewString()[:6], "user_display_name": s.user,
			"text": s.text,
		}),
	}
	for _, noise := range s.noise {
		events = append(events, memoryEvent(t, orgID, agentID, sandboxID, sessionID, "user.message.received", map[string]any{
			"source": source, "channel": "C123", "user_display_name": s.user, "text": noise,
		}))
	}
	for _, tool := range s.tools {
		name, summary, _ := strings.Cut(tool, ":")
		events = append(events, memoryEvent(t, orgID, agentID, sandboxID, sessionID, "tool.invoked", map[string]any{
			"source": source, "tool": strings.TrimSpace(name), "result_summary": strings.TrimSpace(summary),
		}))
	}
	events = append(events, memoryEvent(t, orgID, agentID, sandboxID, sessionID, "agent.message.sent", map[string]any{
		"source": source, "text": s.reply,
	}))
	if index%4 == 0 {
		events = append(events, memoryEvent(t, orgID, agentID, sandboxID, sessionID, "session.completed", map[string]any{
			"source": source,
		}))
	}
	return events
}

func distinctProductionSessionIDs(t *testing.T, db interface {
	Raw(string, ...any) *gorm.DB
}, agentID, sandboxID uuid.UUID) []string {
	t.Helper()
	var sessionIDs []string
	if err := db.Raw(
		"SELECT DISTINCT session_id FROM employee_memory_events WHERE employee_id = ? AND sandbox_id = ? ORDER BY session_id",
		agentID, sandboxID,
	).Scan(&sessionIDs).Error; err != nil {
		t.Fatalf("load session ids: %v", err)
	}
	return sessionIDs
}
