export type ConversationSummary = {
  id: string
  title: string
  preview: string
  status: "active" | "ended" | "error"
  date: string
  tokenCount: number
}

export type MessageItem = {
  id: string
  role: "user" | "agent" | "error" | "tool_call" | "system"
  content: string
  timestamp: string
  toolName?: string
  toolParams?: Record<string, string>
  toolStatus?: "success" | "failed" | "running"
  toolDuration?: string
  toolResponse?: string
}

export const sidebarConversations: Record<string, ConversationSummary[]> = {
  Today: [
    {
      id: "conv_001",
      title: "Debug Safari login regression",
      preview: "Investigating SameSite cookie error on Safari 17.2...",
      status: "active",
      date: "10:42 AM",
      tokenCount: 12_400,
    },
    {
      id: "conv_002",
      title: "Triage issue #1025: Dark mode",
      preview: "Labeled as enhancement, assigned to frontend team",
      status: "ended",
      date: "9:15 AM",
      tokenCount: 3_200,
    },
    {
      id: "conv_003",
      title: "Deploy hotfix v2.8.1",
      preview: "Cherry-picked fix for auth cookie, deployed to staging",
      status: "ended",
      date: "8:30 AM",
      tokenCount: 8_900,
    },
  ],
  Yesterday: [
    {
      id: "conv_004",
      title: "API rate limit investigation",
      preview: "Analyzed rate limit breaches from IP 203.0.113.42",
      status: "ended",
      date: "4:22 PM",
      tokenCount: 6_100,
    },
    {
      id: "conv_005",
      title: "Update onboarding flow",
      preview: "Modified welcome email template and signup steps",
      status: "ended",
      date: "2:10 PM",
      tokenCount: 4_800,
    },
    {
      id: "conv_006",
      title: "Database migration review",
      preview: "Reviewed ALTER TABLE for users schema change",
      status: "error",
      date: "11:05 AM",
      tokenCount: 2_300,
    },
  ],
  "Last 7 days": [
    {
      id: "conv_007",
      title: "Refactor billing webhook handler",
      preview: "Split monolithic handler into separate processors",
      status: "ended",
      date: "Apr 12",
      tokenCount: 18_200,
    },
    {
      id: "conv_008",
      title: "Setup monitoring alerts",
      preview: "Configured PagerDuty alerts for p99 latency > 500ms",
      status: "ended",
      date: "Apr 11",
      tokenCount: 5_600,
    },
    {
      id: "conv_009",
      title: "Fix mobile nav regression",
      preview: "Resolved iOS Safari hamburger menu z-index issue",
      status: "ended",
      date: "Apr 10",
      tokenCount: 7_400,
    },
  ],
  "Last 30 days": [
    {
      id: "conv_010",
      title: "SSO integration with Okta",
      preview: "Implemented SAML 2.0 flow with attribute mapping",
      status: "ended",
      date: "Apr 3",
      tokenCount: 22_100,
    },
    {
      id: "conv_011",
      title: "Performance audit: dashboard",
      preview: "Reduced initial load from 3.2s to 1.1s with lazy loading",
      status: "ended",
      date: "Mar 28",
      tokenCount: 15_800,
    },
  ],
}

export const activeConversationMessages: MessageItem[] = [
  {
    id: "msg_001",
    role: "system",
    content: "Triage new GitHub issue #1024 \"Login fails on Safari 17.2 after latest deploy\". Read the issue, check for related issues, assign labels and priority, then notify the team.",
    timestamp: "10:42 AM",
  },
  {
    id: "msg_002",
    role: "tool_call",
    content: "",
    timestamp: "10:42 AM",
    toolName: "github.get_issue",
    toolParams: { repo: "acme/webapp", issue_number: "1024" },
    toolStatus: "success",
    toolDuration: "280ms",
    toolResponse: JSON.stringify({
      number: 1024,
      title: "Login fails on Safari 17.2 after latest deploy",
      state: "open",
      user: "jsmith",
      labels: [],
      body: "After upgrading to v2.8.0, login no longer works on Safari 17.2. Console shows SameSite cookie error. Steps to reproduce: 1. Open Safari 17.2 2. Navigate to login 3. Enter credentials 4. Click submit - redirects back to login page",
    }, null, 2),
  },
  {
    id: "msg_003",
    role: "agent",
    content: "I've read issue #1024. The reporter describes a login failure specifically on Safari 17.2 after the v2.8.0 deploy. The console shows a `SameSite` cookie error, and users get redirected back to the login page after submitting credentials.\n\nThis looks like a cookie policy regression. Let me search for related issues to see if this has been reported before.",
    timestamp: "10:42 AM",
  },
  {
    id: "msg_004",
    role: "tool_call",
    content: "",
    timestamp: "10:42 AM",
    toolName: "github.search_issues",
    toolParams: { repo: "acme/webapp", query: "Safari cookie SameSite login" },
    toolStatus: "success",
    toolDuration: "450ms",
    toolResponse: JSON.stringify({
      total_count: 2,
      items: [
        { number: 987, title: "Safari SameSite cookie fix", state: "closed" },
        { number: 1002, title: "Login redirect loop on WebKit", state: "open" },
      ],
    }, null, 2),
  },
  {
    id: "msg_005",
    role: "agent",
    content: "Found 2 related issues:\n\n- **#987** (closed) — \"Safari SameSite cookie fix\" was resolved in v2.7.1\n- **#1002** (open) — \"Login redirect loop on WebKit\" has similar symptoms\n\nThis appears to be a **regression** of the fix from #987. The v2.8.0 deploy likely reverted or broke the SameSite cookie configuration. I'll label this as a high-priority bug.",
    timestamp: "10:43 AM",
  },
  {
    id: "msg_006",
    role: "user",
    content: "This is affecting production logins. Please escalate to P1 and make sure the auth team gets notified on Slack immediately.",
    timestamp: "10:43 AM",
  },
  {
    id: "msg_007",
    role: "agent",
    content: "Understood — escalating to P1. I'll:\n\n1. Label the issue as `bug`, `P1`, `auth`, `browser-compat`\n2. Create an urgent Linear ticket for the auth team\n3. Send a notification to `#eng-auth` on Slack",
    timestamp: "10:43 AM",
  },
  {
    id: "msg_008",
    role: "tool_call",
    content: "",
    timestamp: "10:43 AM",
    toolName: "github.add_labels",
    toolParams: { repo: "acme/webapp", issue: "1024", labels: "bug, P1, auth, browser-compat" },
    toolStatus: "success",
    toolDuration: "190ms",
    toolResponse: JSON.stringify({ labels: ["bug", "P1", "auth", "browser-compat"] }, null, 2),
  },
  {
    id: "msg_009",
    role: "tool_call",
    content: "",
    timestamp: "10:44 AM",
    toolName: "linear.create_issue",
    toolParams: { team: "AUTH", title: "P1: Safari 17.2 login regression", priority: "urgent", description: "Regression of #987 fix" },
    toolStatus: "success",
    toolDuration: "340ms",
    toolResponse: JSON.stringify({ id: "AUTH-1842", url: "https://linear.app/acme/issue/AUTH-1842" }, null, 2),
  },
  {
    id: "msg_010",
    role: "tool_call",
    content: "",
    timestamp: "10:44 AM",
    toolName: "slack.post_message",
    toolParams: { channel: "#eng-auth", message: "P1 regression: Safari 17.2 login broken after v2.8.0. Issue #1024, Linear AUTH-1842. SameSite cookie regression." },
    toolStatus: "success",
    toolDuration: "220ms",
    toolResponse: JSON.stringify({ ok: true, channel: "C04ABCDEF", ts: "1718451840.000100" }, null, 2),
  },
  {
    id: "msg_011",
    role: "agent",
    content: "Done. Here's a summary of actions taken:\n\n| Action | Status | Detail |\n|--------|--------|--------|\n| GitHub Labels | Applied | `bug`, `P1`, `auth`, `browser-compat` |\n| Linear Ticket | Created | [AUTH-1842](https://linear.app/acme/issue/AUTH-1842) — Urgent |\n| Slack Notification | Sent | `#eng-auth` channel notified |\n\nThe auth team has been alerted. The issue is linked to the previous fix in #987, so they should have good context on the root cause. Is there anything else you'd like me to do?",
    timestamp: "10:44 AM",
  },
  {
    id: "msg_012",
    role: "user",
    content: "Can you also check if the cookie configuration changed in the v2.8.0 diff? Look at the auth middleware files.",
    timestamp: "10:45 AM",
  },
  {
    id: "msg_013",
    role: "tool_call",
    content: "",
    timestamp: "10:45 AM",
    toolName: "github.compare_commits",
    toolParams: { repo: "acme/webapp", base: "v2.7.1", head: "v2.8.0", path: "src/middleware/auth" },
    toolStatus: "running",
    toolDuration: "...",
  },
]

export const terminalOutput = `$ git diff v2.7.1..v2.8.0 -- src/middleware/auth/
diff --git a/src/middleware/auth/cookie.ts b/src/middleware/auth/cookie.ts
index 4a3b2c1..8f7d9e2 100644
--- a/src/middleware/auth/cookie.ts
+++ b/src/middleware/auth/cookie.ts
@@ -12,7 +12,7 @@ export function setAuthCookie(res: Response, token: string) {
     httpOnly: true,
     secure: process.env.NODE_ENV === 'production',
     maxAge: 86400 * 30,
-    sameSite: 'none',
+    sameSite: 'strict',
     path: '/',
   })
 }

$ npm run test -- --grep "auth cookie"
PASS  src/middleware/auth/__tests__/cookie.test.ts
  Auth Cookie
    ✓ sets cookie with correct options (12ms)
    ✓ clears cookie on logout (8ms)
    ✓ refreshes cookie on valid token (15ms)

Tests: 3 passed, 3 total
Time:  1.234s`

export const browserContent = {
  url: "https://staging.acme.com/login",
  title: "Acme App — Login",
  status: "loaded" as const,
  consoleErrors: [
    { level: "warn" as const, message: "Cookie \"session_token\" has been rejected because it is in a cross-site context and its \"SameSite\" is \"Strict\".", timestamp: "10:44:12" },
    { level: "error" as const, message: "POST https://api.acme.com/auth/login 302 (redirect loop detected)", timestamp: "10:44:13" },
  ],
}
