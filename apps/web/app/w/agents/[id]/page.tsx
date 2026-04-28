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
import { $api } from "@/lib/api/hooks"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Textarea } from "@/components/ui/textarea"
import { Workspace } from "./_components/workspace"

type ToolCall = { name: string; status: "running" | "done"; summary: string }

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
  toolCalls?: ToolCall[]
  diff?: DiffPayload
}

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

export default function AgentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: agent } = $api.useQuery("get", "/v1/agents/{id}", {
    params: { path: { id } },
  })

  const agentName = agent?.name ?? "Agent"
  const initial = (agentName ?? "?").slice(0, 1).toUpperCase()

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

          <ScrollArea className="min-h-0 flex-1">
            <div className="mx-auto flex w-full max-w-2xl flex-col px-6 py-8">
              {messages.map((message, index) => {
                const previous = messages[index - 1]
                const next = messages[index + 1]
                const isFirstInGroup = !previous || previous.author !== message.author
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
            </div>
          </ScrollArea>

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
        {message.body ? (
          <p className="max-w-[85%] self-end rounded-2xl bg-secondary px-3.5 py-2 text-[14px] leading-relaxed text-secondary-foreground">
            {message.body}
          </p>
        ) : null}
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

      {message.toolCalls && message.toolCalls.length > 0 ? (
        <div className="flex flex-col gap-1">
          {message.toolCalls.map((call, callIndex) => (
            <ToolCallChip key={`${call.name}-${callIndex}`} call={call} />
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

function ThinkingBlock({ text }: { text: string }) {
  return (
    <div className="rounded-lg border border-border/60 bg-muted/40 px-3 py-2">
      <p className="font-mono text-[10px] uppercase tracking-[1px] text-muted-foreground/60">Thinking</p>
      <p className="mt-1 text-[12px] leading-relaxed text-muted-foreground">{text}</p>
    </div>
  )
}

function ToolCallChip({ call }: { call: ToolCall }) {
  const [open, setOpen] = React.useState(false)
  const isRunning = call.status === "running"

  return (
    <div className="w-full overflow-hidden rounded-md border border-border/60 bg-muted/30">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="flex w-full items-center gap-2 px-2.5 py-1 text-left transition-colors hover:bg-muted/60"
      >
        <span
          className={`size-1.5 shrink-0 rounded-full ${isRunning ? "bg-amber-500 animate-pulse" : "bg-emerald-500"}`}
        />
        <span className="flex-1 truncate font-mono text-[11px] text-foreground">{call.name}</span>
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
            <div className="border-t border-border/60 px-2.5 py-1.5">
              <span className="font-mono text-[11px] text-muted-foreground/80">{call.summary}</span>
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


