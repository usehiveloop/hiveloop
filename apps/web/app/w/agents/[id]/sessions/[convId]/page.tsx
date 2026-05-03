"use client"

import * as React from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import { useTheme } from "next-themes"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Attachment02Icon,
  MoreHorizontalIcon,
  PencilEdit02Icon,
  Sent02Icon,
} from "@hugeicons/core-free-icons"
import { MultiFileDiff, type FileContents } from "@pierre/diffs/react"
import { Group as PanelGroup, Panel, Separator as PanelResizer } from "react-resizable-panels"
import ScrollToBottom from "react-scroll-to-bottom"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"
import { $api } from "@/lib/api/hooks"
import { useAgentSessions } from "@/hooks/use-agent-sessions"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"
import { Workspace } from "@/app/w/agents/[id]/_components/workspace"

type ToolCall = {
  id: string
  title: string
  status: "running" | "done"
  summary: string
}

type ToolGroup = {
  name: string
  calls: ToolCall[]
}

type DiffPayload = {
  oldFile: FileContents
  newFile: FileContents
}

type Message = {
  id: string
  author: "user" | "agent"
  body?: string
  timestamp: string
  thinking?: string
  toolGroups?: ToolGroup[]
  diff?: DiffPayload
}

/* eslint-disable @typescript-eslint/no-unused-vars */
// Static demo data — kept for reference while we wire up real conversation events.
/*
const stripeHandlerOld = `package handler

import (
	"encoding/json"
	"net/http"
)

func (h *WebhookHandler) HandleStripe(w http.ResponseWriter, r *http.Request) {
	var event stripeEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid payload"))
		return
	}

	if err := h.processor.Process(r.Context(), event); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("processing failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
`

const stripeHandlerNew = `package handler

import (
	"encoding/json"
	"errors"
	"net/http"
)

func (h *WebhookHandler) HandleStripe(w http.ResponseWriter, r *http.Request) {
	var event stripeEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid payload"))
		return
	}

	// Idempotency: drop duplicates instead of letting them bubble up as conflicts.
	if seen, err := h.dedupe.Seen(r.Context(), event.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("dedupe lookup failed"))
		return
	} else if seen {
		writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}

	if err := h.processor.Process(r.Context(), event); err != nil {
		if errors.Is(err, ErrTransient) {
			writeJSON(w, http.StatusServiceUnavailable, errorBody("retry later"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorBody("processing failed"))
		return
	}

	if err := h.dedupe.Mark(r.Context(), event.ID); err != nil {
		// Log but don't fail — the webhook succeeded; we just risk a future replay.
		h.log.Warn("dedupe.mark failed", "event_id", event.ID, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
`

const stripeTestOld = `package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripeWebhook_OK(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/webhooks/stripe", strings.NewReader(\`{"id":"evt_1"}\`))
	rr := httptest.NewRecorder()
	newHandler(t).HandleStripe(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
`

const stripeTestNew = `package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripeWebhook_OK(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/webhooks/stripe", strings.NewReader(\`{"id":"evt_1"}\`))
	rr := httptest.NewRecorder()
	newHandler(t).HandleStripe(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestStripeWebhook_Duplicate(t *testing.T) {
	h := newHandler(t)
	body := \`{"id":"evt_dup"}\`

	first := httptest.NewRecorder()
	h.HandleStripe(first, httptest.NewRequest("POST", "/v1/webhooks/stripe", strings.NewReader(body)))
	if first.Code != http.StatusOK {
		t.Fatalf("first delivery: expected 200, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	h.HandleStripe(second, httptest.NewRequest("POST", "/v1/webhooks/stripe", strings.NewReader(body)))
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate delivery: expected 200, got %d", second.Code)
	}
	if !strings.Contains(second.Body.String(), "duplicate") {
		t.Fatalf("expected duplicate marker, got %s", second.Body.String())
	}
}
`

const messages: Message[] = [
  {
    id: "m1",
    author: "user",
    body: "Triage the new Stripe webhook errors from this morning. Group by error code and show me which customers are affected.",
    timestamp: "9:14 AM",
  },
  {
    id: "m2",
    author: "agent",
    thinking:
      "Pulling webhook events from the last 6h, bucketing by event.code, then cross-referencing customer ids against the billing table.",
    body: "Looking at 247 failed deliveries since 03:00 UTC. Top three error codes account for ~91% of failures.",
    timestamp: "9:14 AM",
    toolCalls: [
      { name: "logs.query", status: "done", summary: "stripe.webhook.delivery_failed since=6h ago" },
      { name: "billing.lookup_customers", status: "done", summary: "ids=[247], deduped=31" },
    ],
  },
  {
    id: "m3",
    author: "agent",
    body: "Breakdown: `card_declined` (138), `webhook_signature_mismatch` (62), `idempotency_conflict` (24). Affected: 31 customers, 4 enterprise. The idempotency conflicts look like the easy win — same event id replayed by Stripe within seconds.",
    timestamp: "9:15 AM",
  },
  {
    id: "m4",
    author: "user",
    body: "Fix the idempotency conflict first. Add a dedupe layer to the handler.",
    timestamp: "9:18 AM",
  },
  {
    id: "m4b",
    author: "user",
    body: "Use Redis if there's already a client wired up — don't pull in a new dep.",
    timestamp: "9:18 AM",
  },
  {
    id: "m4c",
    author: "user",
    body: "And only mark events as seen *after* the processor succeeds.",
    timestamp: "9:18 AM",
  },
  {
    id: "m5",
    author: "agent",
    thinking: "Need to find the current handler, see how it's wired, and check whether we already have a dedupe primitive somewhere I can reuse.",
    body: "Searching the repo for the existing handler and any dedupe helpers.",
    timestamp: "9:18 AM",
    toolCalls: [
      { name: "search_code", status: "done", summary: "query=\"HandleStripe\"" },
      { name: "search_code", status: "done", summary: "query=\"Dedupe|Idempotency\"" },
      { name: "read_file", status: "done", summary: "internal/handler/webhook_stripe.go" },
    ],
  },
  {
    id: "m6",
    author: "agent",
    body: "There's a `Dedupe` interface already in `internal/cache/dedupe.go` backed by Redis with a 24h TTL. Wiring it into the handler now — drop duplicates with 200 OK, and only mark after successful processing so transient failures get retried by Stripe.",
    timestamp: "9:19 AM",
    toolCalls: [
      { name: "apply_patch", status: "done", summary: "internal/handler/webhook_stripe.go (+18 -2)" },
    ],
    diff: {
      oldFile: { name: "internal/handler/webhook_stripe.go", contents: stripeHandlerOld },
      newFile: { name: "internal/handler/webhook_stripe.go", contents: stripeHandlerNew },
    },
  },
  {
    id: "m7",
    author: "agent",
    body: "Adding a regression test for the duplicate path before running the suite.",
    timestamp: "9:20 AM",
    toolCalls: [
      { name: "apply_patch", status: "done", summary: "internal/handler/webhook_stripe_test.go (+19 -0)" },
    ],
    diff: {
      oldFile: { name: "internal/handler/webhook_stripe_test.go", contents: stripeTestOld },
      newFile: { name: "internal/handler/webhook_stripe_test.go", contents: stripeTestNew },
    },
  },
  {
    id: "m8",
    author: "agent",
    body: "Tests pass. The duplicate case now returns 200 with `status:\"duplicate\"` and the processor isn't called twice.",
    timestamp: "9:21 AM",
    toolCalls: [
      { name: "run_tests", status: "done", summary: "go test ./internal/handler -run TestStripeWebhook" },
    ],
  },
  {
    id: "m9",
    author: "user",
    body: "Nice. Open a PR and link the incident ticket.",
    timestamp: "9:22 AM",
  },
  {
    id: "m10",
    author: "agent",
    body: "Drafting the PR with the diff above and a link to INC-2041 in the description. Will pause for your review before pushing.",
    timestamp: "9:22 AM",
    toolCalls: [
      { name: "git.create_branch", status: "done", summary: "fix/stripe-webhook-dedupe" },
      { name: "git.commit", status: "done", summary: "2 files, +37 -2" },
      { name: "github.draft_pr", status: "running", summary: "title=\"Dedupe Stripe webhooks\" base=main" },
    ],
  },
]
*/
/* eslint-enable @typescript-eslint/no-unused-vars */

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return ""
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ""
  return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })
}

type ApiToolCall = components["schemas"]["conversationToolCallResponse"]
type ApiToolGroup = components["schemas"]["conversationToolGroupResponse"]
type ApiMessage = components["schemas"]["conversationMessageResponse"]

function apiMessageToMessage(m: ApiMessage): Message {
  return {
    id: m.id ?? "",
    author: m.author === "user" ? "user" : "agent",
    body: m.body,
    timestamp: formatTimestamp(m.timestamp),
    toolGroups: (m.tool_groups ?? []).map((g: ApiToolGroup) => ({
      name: g.name ?? "tool",
      calls: (g.calls ?? []).map((c: ApiToolCall) => ({
        id: c.id ?? "",
        title: c.title ?? g.name ?? "tool",
        status: c.status === "running" ? "running" : "done",
        summary: c.summary ?? "",
      })),
    })),
  }
}

export default function AgentDetailPage() {
  const { id, convId } = useParams<{ id: string; convId: string }>()
  const { data: agent } = $api.useQuery("get", "/v1/agents/{id}", {
    params: { path: { id } },
  })

  const { sessions } = useAgentSessions(id ?? null)

  const agentName = agent?.name ?? "Agent"
  const initial = (agentName ?? "?").slice(0, 1).toUpperCase()

  const messagesQuery = $api.useQuery(
    "get",
    "/v1/conversations/{convID}/messages",
    { params: { path: { convID: convId }, query: { limit: 1000 } } },
    { enabled: Boolean(convId), refetchInterval: 5000 },
  )

  const messages = React.useMemo<Message[]>(
    () => (messagesQuery.data?.data ?? []).map(apiMessageToMessage),
    [messagesQuery.data],
  )

  return (
    <div className="fixed inset-0 z-50 bg-background">
      <PanelGroup orientation="horizontal" className="flex h-full">
        <Panel
          id="chat"
          defaultSize="25%"
          minSize="25%"
          maxSize="60%"
          className="flex h-full flex-col"
        >
          <ChatHeader name={agentName} avatarUrl={agent?.avatar_url ?? null} initial={initial} agentId={id} />

          <ScrollToBottom
            className="min-h-0 flex-1"
            followButtonClassName="hidden"
            initialScrollBehavior="auto"
          >
            <div className="mx-auto flex w-full max-w-2xl flex-col gap-3 px-6 py-8">
              {messages.map((message, index) => {
                const prev = messages[index - 1]
                const next = messages[index + 1]
                const isFirstInGroup = !prev || prev.author !== message.author
                const isLastInGroup = !next || next.author !== message.author
                return (
                  <MessageBubble
                    key={message.id}
                    message={message}
                    agentName={agentName}
                    avatarUrl={agent?.avatar_url ?? null}
                    initial={initial}
                    isFirstInGroup={isFirstInGroup}
                    isLastInGroup={isLastInGroup}
                    isFirst={index === 0}
                  />
                )
              })}

              {!messagesQuery.isLoading && messages.length === 0 ? (
                <p className="text-center text-[12px] text-muted-foreground/70">
                  No messages yet.
                </p>
              ) : null}
            </div>
          </ScrollToBottom>

          <Composer />
        </Panel>

        <PanelResizer
          id="resizer"
          className="relative w-px shrink-0 grow-0 bg-border/60 transition-colors hover:bg-primary/40 data-[separator-state=drag]:bg-primary"
        >
          <div className="absolute inset-y-0 -left-1 -right-1" />
        </PanelResizer>

        <Panel id="workspace" defaultSize="75%" minSize="40%" className="flex h-full flex-col bg-muted/20">
          <Workspace />
        </Panel>
      </PanelGroup>
    </div>
  )
}

/* ────────────────────────────────────────────────────────────────── */

function ChatHeader({
  name,
  avatarUrl,
  initial,
  agentId,
}: {
  name: string
  avatarUrl: string | null
  initial: string
  agentId: string
}) {
  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-3 px-4">
      <div className="flex min-w-0 items-center gap-3">
        <Button
          variant="ghost"
          size="icon-sm"
          render={<Link href="/w/agents" />}
          aria-label="Back to agents"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={16} />
        </Button>

        <Avatar size="sm" className="rounded-md after:rounded-md">
          {avatarUrl ? <AvatarImage src={avatarUrl} alt={name} className="rounded-md" /> : null}
          <AvatarFallback className="rounded-md">{initial}</AvatarFallback>
        </Avatar>

        <span className="truncate text-[13px] font-medium text-foreground">{name}</span>
      </div>

      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon-sm"
          render={<Link href={`/w/agents/${agentId}/edit`} />}
          aria-label="Edit agent"
        >
          <HugeiconsIcon icon={PencilEdit02Icon} size={14} />
        </Button>
        <Button variant="ghost" size="icon-sm" disabled aria-label="More">
          <HugeiconsIcon icon={MoreHorizontalIcon} size={14} />
        </Button>
      </div>
    </header>
  )
}

function MessageBubble({
  message,
  agentName,
  avatarUrl,
  initial,
  isFirstInGroup,
  isLastInGroup,
  isFirst,
}: {
  message: Message
  agentName: string
  avatarUrl: string | null
  initial: string
  isFirstInGroup: boolean
  isLastInGroup: boolean
  isFirst: boolean
}) {
  const isAgent = message.author === "agent"
  const topSpacing = isFirst ? "" : isFirstInGroup ? "mt-6" : "mt-2"

  if (!isAgent) {
    return (
      <div className={`flex flex-col gap-2 ${topSpacing}`}>
        {message.body ? <UserMessageBody body={message.body} /> : null}
        {isLastInGroup ? (
          <div className="flex items-center justify-end gap-2">
            <span className="text-[11px] text-muted-foreground/70">{message.timestamp}</span>
            <div className="flex size-6 items-center justify-center rounded-md bg-foreground text-[11px] font-medium text-background">
              U
            </div>
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <div className={`flex flex-col gap-2 ${topSpacing}`}>
      {isFirstInGroup ? (
        <div className="flex items-center gap-2">
          <Avatar size="sm" className="rounded-md after:rounded-md">
            {avatarUrl ? <AvatarImage src={avatarUrl} alt={agentName} className="rounded-md" /> : null}
            <AvatarFallback className="rounded-md">{initial}</AvatarFallback>
          </Avatar>
          <span className="text-[12px] font-medium text-foreground">{agentName}</span>
          <span className="text-[11px] text-muted-foreground/70">{message.timestamp}</span>
        </div>
      ) : null}

      {message.thinking ? <ThinkingBlock text={message.thinking} /> : null}

      {message.toolGroups && message.toolGroups.length > 0 ? (
        <div className="flex flex-col gap-1">
          {message.toolGroups.map((group, groupIndex) => (
            <ToolGroupChip key={`${group.name}-${groupIndex}`} group={group} />
          ))}
        </div>
      ) : null}

      {message.diff ? <DiffBlock diff={message.diff} /> : null}

      {message.body ? (
        <p className="text-[14px] leading-relaxed text-foreground">{message.body}</p>
      ) : null}
    </div>
  )
}

function DiffBlock({ diff }: { diff: DiffPayload }) {
  const [open, setOpen] = React.useState(false)
  const { resolvedTheme } = useTheme()
  const theme = resolvedTheme === "dark" ? "pierre-dark" : "pierre-light"

  return (
    <div className="w-full overflow-hidden rounded-lg border border-border/60 bg-muted/20">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors hover:bg-muted/40"
      >
        <span className="flex-1 truncate font-mono text-[11px] text-muted-foreground">
          {diff.newFile.name}
        </span>
        <motion.span
          animate={{ rotate: open ? 90 : 0 }}
          transition={{ duration: 0.18, ease: [0.32, 0.72, 0, 1] }}
          className="text-muted-foreground/70"
        >
          <HugeiconsIcon icon={ArrowRight01Icon} size={12} />
        </motion.span>
      </button>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            key="content"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.22, ease: [0.32, 0.72, 0, 1] }}
            className="overflow-hidden border-t border-border/60"
            style={{ ["--diffs-font-size" as never]: "11px" }}
          >
            <div className="overflow-x-auto">
              <MultiFileDiff
                oldFile={diff.oldFile}
                newFile={diff.newFile}
                options={{ theme, diffStyle: "unified" }}
              />
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

function looksLikeMarkdown(text: string): boolean {
  if (text.length === 0) return false
  const patterns: RegExp[] = [
    /^#{1,6}\s+\S/m,
    /^\s*[-*+]\s+\S/m,
    /^\s*\d+\.\s+\S/m,
    /^\s*>\s+\S/m,
    /^\s*```/m,
    /^\s*\|.+\|\s*$/m,
    /^\s*-{3,}\s*$/m,
    /\[[^\]]+\]\([^)]+\)/,
    /!\[[^\]]*\]\([^)]+\)/,
    /\*\*[^*\n]+\*\*/,
    /__[^_\n]+__/,
    /(^|[^`])`[^`\n]+`/,
  ]
  return patterns.some((p) => p.test(text))
}

const previewProseClasses =
  "prose prose-sm dark:prose-invert max-w-none " +
  "prose-headings:my-0 prose-headings:font-medium " +
  "prose-h1:text-[12px] prose-h2:text-[12px] prose-h3:text-[12px] prose-h4:text-[12px] " +
  "prose-p:my-0 prose-p:text-[12px] prose-p:leading-relaxed " +
  "prose-ul:my-0 prose-ol:my-0 prose-li:my-0 prose-li:text-[12px] " +
  "prose-pre:my-0 prose-code:text-[11px] prose-code:before:content-none prose-code:after:content-none " +
  "prose-blockquote:my-0 prose-blockquote:border-l-2 prose-blockquote:pl-2 " +
  "prose-hr:my-1 prose-table:my-0"

function MarkdownView({ content }: { content: string }) {
  return (
    <div className="prose prose-sm dark:prose-invert max-w-none prose-headings:font-heading prose-h1:text-base prose-h2:text-sm prose-h3:text-[13px] prose-p:text-[12px] prose-p:leading-relaxed prose-li:text-[12px] prose-code:text-[11px] prose-code:before:content-none prose-code:after:content-none prose-pre:rounded-xl prose-table:text-[12px] prose-th:text-[11px] prose-td:text-[11px]">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          code({ className, children, ...props }) {
            const match = /language-(\w+)/.exec(className ?? "")
            const codeString = String(children).replace(/\n$/, "")

            if (match) {
              return (
                <SyntaxHighlighter
                  style={oneDark}
                  language={match[1]}
                  PreTag="div"
                  customStyle={{ fontSize: "12px", borderRadius: "12px", margin: 0 }}
                >
                  {codeString}
                </SyntaxHighlighter>
              )
            }

            return (
              <code className={cn("rounded bg-muted px-1.5 py-0.5 font-mono text-xs", className)} {...props}>
                {children}
              </code>
            )
          },
          a({ children, href, ...props }) {
            return (
              <a
                href={href}
                target="_blank"
                rel="noreferrer noopener"
                {...props}
              >
                {children}
              </a>
            )
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}

function UserMessageBody({ body }: { body: string }) {
  const [open, setOpen] = React.useState(false)
  const isMarkdown = React.useMemo(() => looksLikeMarkdown(body), [body])

  const PREVIEW_MAX_HEIGHT_REM = 5

  return (
    <>
      <div className="flex justify-end">
        <div
          role="button"
          tabIndex={0}
          aria-label="Open full message"
          title="Click to view full message"
          onClick={() => setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault()
              setOpen(true)
            }
          }}
          className="relative max-w-[85%] cursor-pointer overflow-hidden rounded-2xl bg-secondary px-3.5 py-2 text-secondary-foreground transition-colors hover:bg-secondary/80 focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          style={{ maxHeight: `${PREVIEW_MAX_HEIGHT_REM}rem` }}
        >
          <div className="pointer-events-none">
            {isMarkdown ? (
              <div className={previewProseClasses}>
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{body}</ReactMarkdown>
              </div>
            ) : (
              <p className="whitespace-pre-wrap break-words text-[12px] leading-relaxed">
                {body}
              </p>
            )}
          </div>
          <div className="pointer-events-none absolute inset-x-0 bottom-0 h-6 bg-gradient-to-t from-secondary to-transparent" />
        </div>
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Message</DialogTitle>
          </DialogHeader>
          <div className="max-h-[70vh] overflow-auto rounded-md bg-muted/40 p-4">
            {isMarkdown ? (
              <MarkdownView content={body} />
            ) : (
              <pre className="whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed text-foreground">
                {body}
              </pre>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

function ThinkingBlock({ text }: { text: string }) {
  return (
    <div className="rounded-lg border border-border/60 bg-muted/40 px-3 py-2">
      <p className="font-mono text-[10px] uppercase tracking-[1px] text-muted-foreground/60">Thinking</p>
      <p className="mt-1 text-[12px] leading-relaxed text-muted-foreground">{text}</p>
    </div>
  )
}

function ToolGroupChip({ group }: { group: ToolGroup }) {
  const [open, setOpen] = React.useState(false)
  const anyRunning = group.calls.some((c) => c.status === "running")
  const count = group.calls.length

  return (
    <div className="w-full overflow-hidden rounded-md border border-border/60 bg-muted/30">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="flex w-full items-center gap-2 px-2.5 py-1 text-left transition-colors hover:bg-muted/60"
      >
        <span
          className={`size-1.5 shrink-0 rounded-full ${anyRunning ? "bg-amber-500 animate-pulse" : "bg-emerald-500"}`}
        />
        <span className="flex-1 truncate font-mono text-[11px] text-foreground">
          {group.name}
          {count > 1 ? (
            <span className="ml-1.5 text-muted-foreground/70">({count})</span>
          ) : null}
        </span>
        <motion.span
          animate={{ rotate: open ? 90 : 0 }}
          transition={{ duration: 0.18, ease: [0.32, 0.72, 0, 1] }}
          className="text-muted-foreground/70"
        >
          <HugeiconsIcon icon={ArrowRight01Icon} size={12} />
        </motion.span>
      </button>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            key="content"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.22, ease: [0.32, 0.72, 0, 1] }}
            className="overflow-hidden"
          >
            <div className="flex flex-col gap-1 border-t border-border/60 px-2.5 py-1.5">
              {group.calls.map((call) => (
                <div key={call.id || call.title} className="flex items-start gap-2">
                  <span
                    className={`mt-1.5 size-1.5 shrink-0 rounded-full ${call.status === "running" ? "bg-amber-500 animate-pulse" : "bg-emerald-500"}`}
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-[11px] text-foreground">{call.title}</p>
                    {call.summary ? (
                      <pre className="mt-0.5 max-h-32 overflow-auto whitespace-pre-wrap break-words font-mono text-[10px] leading-relaxed text-muted-foreground/80">
                        {call.summary}
                      </pre>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

function Composer() {
  return (
    <div className="shrink-0 bg-background px-4 py-3">
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-2 rounded-2xl border border-border bg-muted/30 p-2 focus-within:border-primary/60">
        <Textarea
          placeholder="Reply to agent…"
          rows={2}
          className="min-h-0 resize-none border-0 bg-transparent px-2 py-1.5 text-[14px] shadow-none focus-visible:ring-0"
        />
        <div className="flex items-center justify-between gap-2 px-1">
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="icon-sm" disabled aria-label="Attach">
              <HugeiconsIcon icon={Attachment02Icon} size={14} />
            </Button>
          </div>
          <Button size="sm" className="gap-1.5" disabled>
            Send
            <HugeiconsIcon icon={Sent02Icon} size={13} />
          </Button>
        </div>
      </div>
    </div>
  )
}


