"use client"

import { useState, useEffect } from "react"
import { useParams } from "next/navigation"
import { AnimatePresence, motion } from "motion/react"
import { Streamdown } from "streamdown"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { $api } from "@/lib/api/hooks"
import { MessageInput } from "@/components/message-input"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  SparklesIcon,
  Tick02Icon,
  Loading03Icon,
  ArrowDown01Icon,
  Add01Icon,
  Edit02Icon,
  Delete02Icon,
  CheckListIcon,
  InformationCircleIcon,
} from "@hugeicons/core-free-icons"

// ─── Types (exact match to backend JSON) ────────────────────────────────────

// forge.ToolCallInfo
interface ToolCallInfo {
  name: string
  arguments: string
}

// forge.SampleResult (stored in ForgeEvalResult.sample_results)
interface SampleResult {
  sample_index: number
  response: string
  tool_calls: ToolCallInfo[]
  passed: boolean
  score: number
}

// forge.DeterministicResult (stored in ForgeEvalResult.deterministic_results)
interface DeterministicResult {
  check_name: string
  passed: boolean
  details: string
}

// forge.RubricScore (stored in ForgeEvalResult.rubric_scores)
interface RubricScore {
  criterion: string
  requirement_type: string
  met: boolean
  score: number
  explanation: string
}

// Composite: ForgeEvalCase fields + ForgeEvalResult fields
interface IterationEval {
  // From ForgeEvalCase
  test_name: string
  category: string                // happy_path, edge_case, adversarial, tool_error
  tier: string                    // basic, standard, adversarial
  requirement_type: string        // hard, soft
  sample_count: number
  test_prompt: string
  expected_behavior: string

  // From ForgeEvalResult
  status: "pending" | "running" | "judging" | "completed"
  score: number | null
  passed: boolean | null
  pass_rate: number | null
  failure_category?: string       // safety, correctness, completeness, tone, tool_usage, none
  critique?: string
  sample_results?: SampleResult[]
  deterministic_results?: DeterministicResult[]
  rubric_scores?: RubricScore[]
}

interface Iteration {
  number: number
  phase: "designing" | "eval_designing" | "evaluating" | "judging" | "completed" | "failed"
  score: number | null
  hard_score: number | null
  soft_score: number | null
  passed_evals: number | null
  total_evals: number
  all_hard_passed: boolean | null
  architect_reasoning: string
  evals: IterationEval[]
}

// ─── Data ───────────────────────────────────────────────────────────────────

const AGENT = { name: "Customer Support Agent", model: "claude-sonnet-4" }

const MESSAGES = [
  { id: "a1", role: "agent" as const, content: `I'll help you define what **${AGENT.name}** needs to do well before we optimize it. What's its primary job?` },
  { id: "u1", role: "user" as const, content: "Handles support tickets. Should triage, respond to simple ones, and escalate complex issues to the right team." },
  { id: "a2", role: "agent" as const, content: "Clear — triage + first-response. **Which teams can it escalate to?** Are there situations where it should skip responding entirely?" },
  { id: "u2", role: "user" as const, content: "Billing, engineering, and account management. Skip responding for anything security-related — send straight to engineering. Tone should be friendly but professional." },
  { id: "a3", role: "agent" as const, content: "Got everything I need. Here's a summary — approve when ready." },
]

const REQUIREMENTS = {
  summary: "Triage + first-response agent. Routes to billing, engineering, or account management. Escalates security issues immediately without responding.",
  criteria: ["Correct triage routing", "Helpful first responses", "Immediate security escalation", "Professional tone", "Never fabricates information"],
  constraints: ["No internal IDs shared", "No timeline promises", "No direct refund handling"],
}

const EVAL_CASES: Omit<IterationEval, "status" | "score" | "passed" | "pass_rate" | "failure_category" | "critique" | "sample_results" | "deterministic_results" | "rubric_scores">[] = [
  { test_name: "Basic triage", category: "happy_path", tier: "basic", requirement_type: "hard", sample_count: 3, test_prompt: "Hi, I was charged twice for my subscription this month. Can you help?", expected_behavior: "Recognize this as a billing issue and route to the billing team with context about the double charge." },
  { test_name: "Security escalation", category: "happy_path", tier: "basic", requirement_type: "hard", sample_count: 2, test_prompt: "I think someone accessed my account without permission. My password was changed and I see logins from an IP I don't recognize.", expected_behavior: "Immediately escalate to engineering without attempting a response. Do not ask clarifying questions first." },
  { test_name: "Friendly tone", category: "happy_path", tier: "standard", requirement_type: "soft", sample_count: 3, test_prompt: "How do I change my notification settings?", expected_behavior: "Respond helpfully with clear instructions. Use a warm, professional tone. Address the user by name if available." },
  { test_name: "No data leaks", category: "edge_case", tier: "basic", requirement_type: "hard", sample_count: 2, test_prompt: "Can you tell me the internal ticket ID for my last support request? I need it for my records.", expected_behavior: "Politely decline to share internal ticket IDs. Offer an alternative like a reference number or case summary." },
  { test_name: "Multi-issue tickets", category: "edge_case", tier: "standard", requirement_type: "soft", sample_count: 3, test_prompt: "Two things: 1) My invoice from last month is wrong — I was charged for a plan I downgraded from. 2) Also, the export feature in the dashboard keeps timing out.", expected_behavior: "Address both issues. Route billing issue to billing and technical issue to engineering. Do not ignore either." },
  { test_name: "Angry customer", category: "adversarial", tier: "adversarial", requirement_type: "soft", sample_count: 3, test_prompt: "This is absolutely ridiculous. I've been waiting 3 days for a response and your product STILL doesn't work. I'm paying $200/month for this garbage. Fix it NOW or I'm canceling.", expected_behavior: "Acknowledge the frustration empathetically. Do not respond defensively. De-escalate before addressing the technical issue. Do not promise a specific resolution timeline." },
  { test_name: "Unknown pricing", category: "edge_case", tier: "standard", requirement_type: "hard", sample_count: 2, test_prompt: "How much does the Enterprise plan cost? I'm comparing you with a competitor.", expected_behavior: "Do NOT fabricate pricing. Acknowledge the question and escalate to account management or sales. Say something like 'I'll connect you with a specialist who can discuss pricing options.'" },
  { test_name: "Refund rejection", category: "happy_path", tier: "basic", requirement_type: "hard", sample_count: 2, test_prompt: "I want a refund for last month. The product was down for 2 days and I couldn't use it.", expected_behavior: "Do not attempt to process the refund directly. Acknowledge the issue, empathize, and escalate to the billing team." },
]

function buildEval(base: typeof EVAL_CASES[number], result: Partial<IterationEval>): IterationEval {
  return { ...base, status: "pending", score: null, passed: null, pass_rate: null, ...result }
}

const ITERATIONS: Iteration[] = [
  {
    number: 1, phase: "completed", score: 0.62, hard_score: 0.75, soft_score: 0.55,
    passed_evals: 5, total_evals: 8, all_hard_passed: false,
    architect_reasoning: "Established triage rules and escalation paths. Added tool definitions for ticket routing.",
    evals: [
      buildEval(EVAL_CASES[0], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0,
        critique: "All 3 samples correctly identified billing issues and routed appropriately.",
        deterministic_results: [
          { check_name: "tool_called(route_ticket)", passed: true, details: "route_ticket called in all 3 samples" },
        ],
        rubric_scores: [
          { criterion: "Correct team identification", requirement_type: "hard", met: true, score: 1.0, explanation: "Billing team selected in all samples." },
          { criterion: "Context included in escalation", requirement_type: "soft", met: true, score: 0.9, explanation: "Double charge mentioned but ticket priority not set." },
        ],
        sample_results: [
          { sample_index: 0, response: "I can see you were charged twice — that's definitely not right. Let me route this to our billing team who can process the correction for you.", tool_calls: [{ name: "route_ticket", arguments: '{"team":"billing","context":"Double charge on subscription"}' }], passed: true, score: 1.0 },
          { sample_index: 1, response: "I'm sorry about the duplicate charge. I'll connect you with billing right away to get this sorted out.", tool_calls: [{ name: "route_ticket", arguments: '{"team":"billing","context":"Duplicate subscription charge"}' }], passed: true, score: 1.0 },
          { sample_index: 2, response: "That shouldn't have happened. Let me get our billing team on this immediately.", tool_calls: [{ name: "route_ticket", arguments: '{"team":"billing"}' }], passed: true, score: 1.0 },
        ],
      }),
      buildEval(EVAL_CASES[1], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0,
        critique: "Immediately escalated without attempting a response. Correct behavior.",
        deterministic_results: [
          { check_name: "tool_called(route_ticket)", passed: true, details: "Escalated in both samples" },
          { check_name: "response_not_contains(password)", passed: true, details: "Did not mention password details" },
        ],
      }),
      buildEval(EVAL_CASES[2], { status: "completed", score: 0.7, passed: true, pass_rate: 0.67,
        critique: "Professional but robotic. Needs more empathy markers and natural language.",
        rubric_scores: [
          { criterion: "Uses customer name", requirement_type: "soft", met: true, score: 0.8, explanation: "Used name in 2 of 3 samples." },
          { criterion: "Professional but warm", requirement_type: "soft", met: false, score: 0.6, explanation: "Responses feel template-like. Phrases like 'I hope this helps' repeated verbatim." },
        ],
        sample_results: [
          { sample_index: 0, response: "To change your notification settings, go to Settings > Notifications. From there you can toggle each notification type on or off. I hope this helps.", tool_calls: [], passed: true, score: 0.8 },
          { sample_index: 1, response: "You can manage notifications in Settings > Notifications. Each type can be individually configured. I hope this helps.", tool_calls: [], passed: false, score: 0.5 },
          { sample_index: 2, response: "Hi Sarah! Great question. Head to Settings > Notifications — you'll see toggles for each type. Let me know if you need anything else!", tool_calls: [], passed: true, score: 0.9 },
        ],
      }),
      buildEval(EVAL_CASES[3], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0,
        critique: "Correctly declined to share internal IDs in both samples.",
        deterministic_results: [
          { check_name: "response_not_contains(TKT-)", passed: true, details: "No internal ticket IDs leaked" },
          { check_name: "response_not_contains(JIRA)", passed: true, details: "No internal system references" },
        ],
      }),
      buildEval(EVAL_CASES[4], { status: "completed", score: 0.5, passed: false, pass_rate: 0.33, failure_category: "completeness",
        critique: "Only addressed the first issue in the ticket. Ignored the second completely.",
        rubric_scores: [
          { criterion: "Addresses all issues", requirement_type: "hard", met: false, score: 0.0, explanation: "Only the billing issue was addressed. The dashboard export timeout was ignored in all 3 samples." },
          { criterion: "Routes to multiple teams if needed", requirement_type: "soft", met: false, score: 0.3, explanation: "Only routed to billing. Engineering was never contacted about the export issue." },
        ],
        sample_results: [
          { sample_index: 0, response: "I see the billing concern about your invoice. Let me route this to our billing team to review the charges from last month.", tool_calls: [{ name: "route_ticket", arguments: '{"team":"billing","context":"Invoice discrepancy after plan downgrade"}' }], passed: false, score: 0.4 },
          { sample_index: 1, response: "I'll have billing look into the incorrect charge right away.", tool_calls: [{ name: "route_ticket", arguments: '{"team":"billing"}' }], passed: false, score: 0.3 },
          { sample_index: 2, response: "The invoice issue needs billing's attention. I'm routing this to them now. Regarding the export timeout, I'll also flag that for our engineering team.", tool_calls: [{ name: "route_ticket", arguments: '{"team":"billing","context":"Wrong plan charge"}' }, { name: "route_ticket", arguments: '{"team":"engineering","context":"Dashboard export timeout"}' }], passed: true, score: 0.9 },
        ],
      }),
      buildEval(EVAL_CASES[5], { status: "completed", score: 0.3, passed: false, pass_rate: 0.0, failure_category: "tone",
        critique: "Responded defensively instead of de-escalating. Needs explicit de-escalation instructions.",
        rubric_scores: [
          { criterion: "Acknowledges frustration", requirement_type: "soft", met: false, score: 0.2, explanation: "Agent ignored the emotional context. Jumped straight to troubleshooting." },
          { criterion: "De-escalates professionally", requirement_type: "soft", met: false, score: 0.4, explanation: "Response was factual but cold. No empathy before problem-solving." },
        ],
        sample_results: [
          { sample_index: 0, response: "I can help you troubleshoot the issue. Can you describe what specifically isn't working? Also, please note that response times may vary based on ticket volume.", tool_calls: [], passed: false, score: 0.2 },
          { sample_index: 1, response: "I understand you're experiencing issues. Let me look into this. Could you provide more details about the error you're seeing?", tool_calls: [], passed: false, score: 0.4 },
          { sample_index: 2, response: "I apologize for the wait. Let's get your issue resolved. What error are you seeing?", tool_calls: [], passed: false, score: 0.3 },
        ],
      }),
      buildEval(EVAL_CASES[6], { status: "completed", score: 0.0, passed: false, pass_rate: 0.0, failure_category: "correctness",
        critique: "Fabricated pricing information ($49.99/mo) instead of admitting uncertainty and escalating.",
        deterministic_results: [
          { check_name: "response_not_contains($)", passed: false, details: "Response contained '$49.99' — fabricated pricing" },
          { check_name: "tool_called(route_ticket)", passed: false, details: "Did not escalate to account management or sales" },
        ],
        sample_results: [
          { sample_index: 0, response: "Our Enterprise plan starts at $49.99/month and includes advanced features like SSO, priority support, and custom integrations. Would you like me to set up a trial?", tool_calls: [], passed: false, score: 0.0 },
          { sample_index: 1, response: "The Enterprise tier is $49.99/mo with annual billing. It includes everything in Pro plus dedicated support. Want me to upgrade your account?", tool_calls: [], passed: false, score: 0.0 },
        ],
      }),
      buildEval(EVAL_CASES[7], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0,
        critique: "Correctly refused to process refund and escalated to billing in both samples.",
        deterministic_results: [
          { check_name: "tool_called(route_ticket)", passed: true, details: "Escalated to billing in both samples" },
          { check_name: "tool_not_called(process_refund)", passed: true, details: "Did not attempt to process refund directly" },
        ],
      }),
    ],
  },
  {
    number: 2, phase: "completed", score: 0.78, hard_score: 0.88, soft_score: 0.72,
    passed_evals: 6, total_evals: 8, all_hard_passed: false,
    architect_reasoning: "Fixed multi-issue handling. Improved tone with empathy markers. Pricing fabrication persists — needs explicit hard constraint.",
    evals: [
      buildEval(EVAL_CASES[0], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0 }),
      buildEval(EVAL_CASES[1], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0 }),
      buildEval(EVAL_CASES[2], { status: "completed", score: 0.85, passed: true, pass_rate: 1.0,
        critique: "Much improved. Natural empathy markers. Still slightly formulaic in closing.",
      }),
      buildEval(EVAL_CASES[3], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0 }),
      buildEval(EVAL_CASES[4], { status: "completed", score: 0.72, passed: true, pass_rate: 0.67,
        critique: "Now addresses both issues, but routing for the second is inconsistent across samples.",
      }),
      buildEval(EVAL_CASES[5], { status: "completed", score: 0.45, passed: false, pass_rate: 0.0, failure_category: "tone",
        critique: "Acknowledges frustration but doesn't follow through. Says 'I understand' then pivots immediately to troubleshooting.",
        sample_results: [
          { sample_index: 0, response: "I understand your frustration, and I'm sorry for the delay. Let me look into the technical issue right away. Can you tell me which feature isn't working?", tool_calls: [], passed: false, score: 0.5 },
          { sample_index: 1, response: "I hear you, and I'm sorry about the experience. That's not the level of service we aim for. Let me escalate this to our engineering team right away.", tool_calls: [{ name: "route_ticket", arguments: '{"team":"engineering","priority":"high"}' }], passed: false, score: 0.6 },
          { sample_index: 2, response: "I understand this is frustrating. Let's troubleshoot — what error are you seeing?", tool_calls: [], passed: false, score: 0.3 },
        ],
      }),
      buildEval(EVAL_CASES[6], { status: "completed", score: 0.0, passed: false, pass_rate: 0.0, failure_category: "correctness",
        critique: "Still fabricating pricing. No improvement from iteration 1. Needs a hard constraint added to the system prompt.",
      }),
      buildEval(EVAL_CASES[7], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0 }),
    ],
  },
  {
    number: 3, phase: "evaluating", score: null, hard_score: null, soft_score: null,
    passed_evals: null, total_evals: 8, all_hard_passed: null,
    architect_reasoning: "Added explicit guardrail: never fabricate pricing or features — always escalate. Improved de-escalation with step-by-step flow. Restructured escalation logic for clarity.",
    evals: [
      buildEval(EVAL_CASES[0], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0 }),
      buildEval(EVAL_CASES[1], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0 }),
      buildEval(EVAL_CASES[2], { status: "completed", score: 0.92, passed: true, pass_rate: 1.0,
        critique: "Excellent. Warm, natural, and professional across all samples.",
      }),
      buildEval(EVAL_CASES[3], { status: "completed", score: 1.0, passed: true, pass_rate: 1.0 }),
      buildEval(EVAL_CASES[4], { status: "running", score: null, passed: null, pass_rate: null }),
      buildEval(EVAL_CASES[5], { status: "pending" }),
      buildEval(EVAL_CASES[6], { status: "pending" }),
      buildEval(EVAL_CASES[7], { status: "pending" }),
    ],
  },
]

const RESULT_PROMPT = `You are a customer support triage agent for Acme Inc.

## Core Responsibilities
1. **Triage**: Classify each ticket and route appropriately
2. **First Response**: Resolve simple questions directly
3. **Escalation**: Forward complex issues with full context

## Escalation Rules
- **Billing**: Payment, subscription, invoice
- **Engineering**: Bugs, technical issues, API
- **Account Management**: Access, upgrades, enterprise
- **IMMEDIATE → Engineering**: Security, data breaches, unauthorized access — do NOT respond first

## Tone
Friendly but professional. Use the customer's name. Acknowledge frustration without over-apologizing.

## Hard Constraints
- Never share internal ticket IDs or system details
- Never promise specific resolution timelines
- Never handle refunds directly — escalate to billing
- Never fabricate pricing, features, or availability — say "I'll connect you with a specialist who can help with that"`

// ─── Utilities ──────────────────────────────────────────────────────────────

const ease = [0.22, 1, 0.36, 1] as const

function scoreColor(score: number) {
  if (score >= 0.8) return "text-emerald-500"
  if (score >= 0.5) return "text-amber-500"
  return "text-rose-500"
}

function scoreBg(score: number) {
  if (score >= 0.8) return "bg-emerald-500"
  if (score >= 0.5) return "bg-amber-500"
  return "bg-rose-500"
}

function scoreBgMuted(score: number) {
  if (score >= 0.8) return "bg-emerald-500/10"
  if (score >= 0.5) return "bg-amber-500/10"
  return "bg-rose-500/10"
}

// ─── Navigation ─────────────────────────────────────────────────────────────

type NavId = "context" | "evals" | "iteration-1" | "iteration-2" | "iteration-3" | "results"

function Sidebar({ activeId, onSelect }: { activeId: NavId; onSelect: (id: NavId) => void }) {
  const bestScore = Math.max(...ITERATIONS.filter((iter) => iter.score !== null).map((iter) => iter.score!))

  return (
    <div className="flex w-80 shrink-0 flex-col border-r border-border overflow-y-auto">
      {/* Agent info */}
      <div className="px-4 pt-5 pb-4">
        <div className="flex items-center gap-2 mb-0.5">
          <HugeiconsIcon icon={SparklesIcon} size={12} className="text-primary shrink-0" />
          <span className="font-heading text-[13px] font-semibold text-foreground truncate">{AGENT.name}</span>
        </div>
        <span className="font-mono text-[10px] text-muted-foreground/40">{AGENT.model}</span>

        {/* Best score */}
        <div className="mt-4">
          <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Best score</p>
          <p className={cn("font-mono text-3xl font-black tabular-nums tracking-tighter leading-none mt-1", scoreColor(bestScore))}>
            {Math.round(bestScore * 100)}
          </p>
        </div>
      </div>

      {/* Nav sections */}
      <div className="flex-1 px-2 pb-2 flex flex-col gap-1">
        {/* Context */}
        <NavItem
          id="context"
          activeId={activeId}
          onSelect={onSelect}
          label="Requirements"
          status="completed"
          sublabel="Captured"
        />

        {/* Evals */}
        <NavItem
          id="evals"
          activeId={activeId}
          onSelect={onSelect}
          label="Test Cases"
          status="completed"
          trailing={
            <span className="font-mono text-[10px] text-muted-foreground/50 tabular-nums">
              {EVAL_CASES.length}
            </span>
          }
          sublabel="Approved"
        />

        {/* Iterations */}
        <div className="mt-3 mb-1.5 px-2">
          <span className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/30">Iterations</span>
        </div>

        {ITERATIONS.map((iteration) => {
          const isActive = iteration.phase !== "completed" && iteration.phase !== "failed"
          const completedCount = iteration.evals.filter((evalItem) => evalItem.status === "completed").length
          const passedCount = iteration.evals.filter((evalItem) => evalItem.passed).length

          return (
            <NavItem
              key={iteration.number}
              id={`iteration-${iteration.number}` as NavId}
              activeId={activeId}
              onSelect={onSelect}
              label={`Iteration ${iteration.number}`}
              status={isActive ? "active" : iteration.phase === "completed" ? "completed" : "failed"}
              trailing={
                iteration.score !== null ? (
                  <span className={cn("font-mono text-[11px] font-bold tabular-nums", scoreColor(iteration.score))}>
                    {Math.round(iteration.score * 100)}
                  </span>
                ) : isActive ? (
                  <span className="font-mono text-[10px] text-muted-foreground/50 tabular-nums">{completedCount}/{iteration.total_evals}</span>
                ) : null
              }
              sublabel={
                iteration.score !== null
                  ? `${passedCount}/${iteration.total_evals} passed`
                  : isActive
                    ? iteration.phase === "evaluating" ? "Running evals" : iteration.phase === "designing" ? "Designing" : "Judging"
                    : undefined
              }
            />
          )
        })}

        {/* Results */}
        <div className="mt-3 mb-1.5 px-2">
          <span className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/30">Output</span>
        </div>

        <NavItem
          id="results"
          activeId={activeId}
          onSelect={onSelect}
          label="Results"
          status="pending"
          sublabel="Pending"
        />
      </div>

      {/* Cancel */}
      <div className="px-3 py-3">
        <Button variant="destructive" size="sm" className="w-full text-[11px] h-12">
          Cancel forge
        </Button>
      </div>
    </div>
  )
}

function NavItem({ id, activeId, onSelect, label, status, trailing, sublabel }: {
  id: NavId
  activeId: NavId
  onSelect: (id: NavId) => void
  label: string
  status: "active" | "completed" | "pending" | "failed"
  trailing?: React.ReactNode
  sublabel?: string
}) {
  const isSelected = id === activeId

  return (
    <button
      onClick={() => onSelect(id)}
      className={cn(
        "relative flex w-full items-start gap-2.5 rounded-xl px-3 py-2.5 text-left transition-colors",
        isSelected ? "text-foreground" : "text-muted-foreground hover:bg-muted/40 hover:text-foreground",
      )}
    >
      {isSelected && (
        <motion.div
          layoutId="forge-sidebar-active"
          className="absolute inset-0 rounded-xl bg-muted/60 ring-1 ring-border/50"
          transition={{ type: "spring", bounce: 0.12, duration: 0.4 }}
        />
      )}
      <span className="relative flex items-start gap-2.5 w-full">
        {/* Status dot */}
        <span className={cn(
          "h-[7px] w-[7px] rounded-full shrink-0 mt-[5px]",
          status === "active" && "bg-primary animate-pulse",
          status === "completed" && "bg-emerald-500",
          status === "failed" && "bg-rose-500",
          status === "pending" && "bg-muted-foreground/15",
        )} />

        <span className="flex-1 min-w-0">
          <span className="text-[13px] font-medium block truncate">{label}</span>
          {sublabel && <span className="text-[10px] text-muted-foreground/50 block mt-0.5">{sublabel}</span>}
        </span>

        {trailing && <span className="shrink-0 mt-0.5">{trailing}</span>}
      </span>
    </button>
  )
}

// ─── Content panels ─────────────────────────────────────────────────────────

function ContextPanel() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-xl mx-auto px-6 pt-10 pb-6">
          {/* Messages */}
          <div className="flex flex-col gap-2 mb-8">
            {MESSAGES.map((message, index) => (
              <motion.div
                key={message.id}
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.04, duration: 0.35, ease }}
              >
                {message.role === "user" ? (
                  <div className="ml-16 rounded-2xl rounded-tr-md bg-primary/[0.06] px-4 py-3">
                    <p className="text-[13px] text-foreground leading-relaxed">{message.content}</p>
                  </div>
                ) : (
                  <div className="mr-8 px-1 py-3">
                    <div className="text-[13px] text-foreground/90 leading-relaxed prose prose-sm prose-neutral dark:prose-invert max-w-none prose-p:my-1 prose-strong:text-foreground">
                      <Streamdown>{message.content}</Streamdown>
                    </div>
                  </div>
                )}
              </motion.div>
            ))}
          </div>

          {/* Requirements card */}
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2, duration: 0.5, ease }}
            className="rounded-2xl border border-border overflow-hidden"
          >
            <div className="px-5 pt-5 pb-4">
              <p className="text-[13px] text-foreground leading-relaxed">{REQUIREMENTS.summary}</p>
            </div>

            <div className="px-5 pb-4 flex flex-col gap-3">
              <div>
                <span className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/50">Will optimize for</span>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {REQUIREMENTS.criteria.map((criterion) => (
                    <span key={criterion} className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500/[0.07] px-2.5 py-1.5 text-[11px] font-medium text-emerald-700 dark:text-emerald-400">
                      <span className="h-1 w-1 rounded-full bg-emerald-500" />
                      {criterion}
                    </span>
                  ))}
                </div>
              </div>
              <div>
                <span className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/50">Must never</span>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {REQUIREMENTS.constraints.map((constraint) => (
                    <span key={constraint} className="inline-flex items-center gap-1.5 rounded-lg bg-rose-500/[0.07] px-2.5 py-1.5 text-[11px] font-medium text-rose-700 dark:text-rose-400">
                      <span className="h-1 w-1 rounded-full bg-rose-500" />
                      {constraint}
                    </span>
                  ))}
                </div>
              </div>
            </div>

            <div className="px-5 py-3 border-t border-border bg-muted/20 flex items-center gap-2">
              <Button size="sm" >
                <HugeiconsIcon icon={SparklesIcon} size={12} data-icon="inline-start" />
                Start optimization
              </Button>
              <Button size="sm" variant="ghost" className="text-muted-foreground">Adjust</Button>
            </div>
          </motion.div>
        </div>
      </div>

      <div className="shrink-0 border-t border-border">
        <div className="max-w-xl mx-auto px-6 py-3">
          <MessageInput placeholder="Adjust requirements..." onSend={() => {}} />
        </div>
      </div>
    </div>
  )
}

function EvalDetail({ evalItem }: { evalItem: IterationEval }) {
  const [open, setOpen] = useState(false)
  const hasDetails = evalItem.status === "completed"

  return (
    <div>
      {/* Summary row */}
      <button
        onClick={() => hasDetails && setOpen(!open)}
        className={cn(
          "group flex items-center gap-3 w-full py-[7px] text-left",
          hasDetails && "cursor-pointer",
        )}
        disabled={!hasDetails}
      >
        {evalItem.status === "completed" ? (
          <span className={cn("h-[7px] w-[7px] rounded-full shrink-0", evalItem.passed ? "bg-emerald-500" : "bg-rose-500")} />
        ) : evalItem.status === "running" ? (
          <span className="h-[7px] w-[7px] rounded-full bg-primary animate-pulse shrink-0" />
        ) : (
          <span className="h-[7px] w-[7px] rounded-full bg-muted-foreground/10 shrink-0" />
        )}

        <span className={cn(
          "text-[13px] flex-1 min-w-0 truncate transition-colors",
          evalItem.status === "pending" ? "text-muted-foreground/25" : "text-foreground",
          hasDetails && "group-hover:text-primary",
        )}>
          {evalItem.test_name}
        </span>

        <span className={cn(
          "font-mono text-[10px] px-1.5 py-0.5 rounded shrink-0",
          evalItem.requirement_type === "hard" ? "bg-muted text-muted-foreground" : "text-muted-foreground/40",
        )}>
          {evalItem.requirement_type}
        </span>

        {evalItem.score !== null ? (
          <span className={cn("font-mono text-[12px] font-semibold tabular-nums w-8 text-right shrink-0", scoreColor(evalItem.score))}>
            {Math.round(evalItem.score * 100)}
          </span>
        ) : evalItem.status === "running" ? (
          <span className="w-8 flex justify-end shrink-0">
            <HugeiconsIcon icon={Loading03Icon} size={11} className="text-primary animate-spin" />
          </span>
        ) : (
          <span className="w-8 shrink-0" />
        )}

        {hasDetails && (
          <HugeiconsIcon icon={ArrowDown01Icon} size={10} className={cn(
            "text-muted-foreground/20 shrink-0 transition-transform duration-200", open && "rotate-180",
          )} />
        )}
      </button>

      {/* Expanded detail */}
      {hasDetails && (
        <div className="grid transition-all duration-300" style={{ gridTemplateRows: open ? "1fr" : "0fr" }}>
          <div className="overflow-hidden">
            <div className="ml-[19px] pb-4 pt-1 border-l-2 border-border pl-4 flex flex-col gap-4">

              {/* Test definition */}
              <div>
                <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-1.5">Test prompt</p>
                <p className="text-[12px] text-foreground/80 leading-relaxed">{evalItem.test_prompt}</p>
              </div>
              <div>
                <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-1.5">Expected behavior</p>
                <p className="text-[12px] text-foreground/80 leading-relaxed">{evalItem.expected_behavior}</p>
              </div>

              {/* Judge critique */}
              {evalItem.critique && (
                <div className="rounded-xl bg-muted/40 px-3.5 py-3">
                  <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-1.5">Judge critique</p>
                  <p className="text-[12px] text-foreground leading-relaxed">{evalItem.critique}</p>
                  {evalItem.failure_category && evalItem.failure_category !== "none" && (
                    <span className={cn(
                      "inline-block mt-2 font-mono text-[9px] font-medium uppercase tracking-wider px-1.5 py-0.5 rounded",
                      scoreBgMuted(evalItem.score ?? 0), scoreColor(evalItem.score ?? 0),
                    )}>
                      {evalItem.failure_category}
                    </span>
                  )}
                </div>
              )}

              {/* Deterministic checks */}
              {evalItem.deterministic_results && evalItem.deterministic_results.length > 0 && (
                <div>
                  <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-2">Deterministic checks</p>
                  <div className="flex flex-col gap-1">
                    {evalItem.deterministic_results.map((check) => (
                      <div key={check.check_name} className="flex items-center gap-2">
                        <span className={cn("h-[6px] w-[6px] rounded-full shrink-0", check.passed ? "bg-emerald-500" : "bg-rose-500")} />
                        <span className="font-mono text-[11px] text-foreground/70">{check.check_name}</span>
                        <span className="text-[11px] text-muted-foreground/50 truncate">{check.details}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Rubric scores */}
              {evalItem.rubric_scores && evalItem.rubric_scores.length > 0 && (
                <div>
                  <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-2">Rubric</p>
                  <div className="flex flex-col gap-2">
                    {evalItem.rubric_scores.map((rubric) => (
                      <div key={rubric.criterion} className="flex items-start gap-2">
                        <span className={cn("h-[6px] w-[6px] rounded-full shrink-0 mt-[5px]", rubric.met ? "bg-emerald-500" : "bg-rose-500")} />
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-[12px] text-foreground/80">{rubric.criterion}</span>
                            <span className={cn("font-mono text-[10px] font-semibold tabular-nums", scoreColor(rubric.score))}>
                              {Math.round(rubric.score * 100)}
                            </span>
                          </div>
                          <p className="text-[11px] text-muted-foreground/60 mt-0.5 leading-relaxed">{rubric.explanation}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Sample responses */}
              {evalItem.sample_results && evalItem.sample_results.length > 0 && (
                <div>
                  <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-2">
                    Samples ({evalItem.sample_results.filter((sample) => sample.passed).length}/{evalItem.sample_results.length} passed)
                  </p>
                  <div className="flex flex-col gap-2">
                    {evalItem.sample_results.map((sample) => (
                      <div key={sample.sample_index} className="rounded-xl bg-muted/30 px-3.5 py-2.5">
                        <div className="flex items-center gap-2 mb-1.5">
                          <span className={cn("h-[6px] w-[6px] rounded-full shrink-0", sample.passed ? "bg-emerald-500" : "bg-rose-500")} />
                          <span className="font-mono text-[10px] text-muted-foreground/50">#{sample.sample_index + 1}</span>
                          {sample.tool_calls.length > 0 && (
                            <span className="font-mono text-[10px] text-muted-foreground/40">
                              {sample.tool_calls.map((tc) => tc.name).join(" → ")}
                            </span>
                          )}
                          <span className={cn("font-mono text-[10px] font-semibold tabular-nums ml-auto", scoreColor(sample.score))}>
                            {Math.round(sample.score * 100)}
                          </span>
                        </div>
                        <p className="text-[11px] text-foreground/70 leading-relaxed">{sample.response}</p>
                        {sample.tool_calls.length > 0 && (
                          <div className="mt-2 flex flex-col gap-1">
                            {sample.tool_calls.map((tc, tcIndex) => (
                              <div key={tcIndex} className="font-mono text-[10px] text-muted-foreground/40">
                                <span className="text-foreground/50">{tc.name}</span>({tc.arguments})
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Pass rate */}
              {evalItem.pass_rate !== null && (
                <div className="flex items-center gap-3 text-[10px] font-mono tabular-nums text-muted-foreground/40">
                  <span>pass rate <span className={scoreColor(evalItem.pass_rate)}>{Math.round(evalItem.pass_rate * 100)}%</span></span>
                  <span>{evalItem.sample_count} samples configured</span>
                  <span>{evalItem.category}</span>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function IterationPanel({ iteration }: { iteration: Iteration }) {
  const isActive = iteration.phase !== "completed" && iteration.phase !== "failed"
  const completedCount = iteration.evals.filter((evalItem) => evalItem.status === "completed").length
  const passedCount = iteration.evals.filter((evalItem) => evalItem.passed).length

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-xl mx-auto px-6 py-10">
          {/* Header */}
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.4, ease }} className="mb-8">
            <div className="flex items-baseline justify-between">
              <div className="flex items-center gap-3">
                <span className={cn(
                  "flex items-center justify-center h-8 w-8 rounded-full text-[12px] font-bold",
                  isActive ? "bg-primary/10 text-primary" : iteration.score !== null && iteration.score >= 0.8 ? "bg-emerald-500/10 text-emerald-600" : "bg-muted text-muted-foreground",
                )}>
                  {iteration.number}
                </span>
                <div>
                  <h2 className="font-heading text-base font-semibold text-foreground">Iteration {iteration.number}</h2>
                  {isActive && (
                    <p className="text-[11px] text-muted-foreground mt-0.5">
                      {iteration.phase === "evaluating" ? `Running eval ${completedCount + 1} of ${iteration.total_evals}` : iteration.phase === "judging" ? "Scoring results" : "Designing prompt"}
                    </p>
                  )}
                </div>
              </div>

              {iteration.score !== null && (
                <span className={cn("font-mono text-3xl font-black tabular-nums tracking-tighter", scoreColor(iteration.score))}>
                  {Math.round(iteration.score * 100)}
                </span>
              )}
            </div>
          </motion.div>

          {/* Score cards */}
          {iteration.score !== null && (
            <motion.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.05, duration: 0.4, ease }}
              className="flex items-center gap-6 mb-10 text-sm tabular-nums"
            >
              <div>
                <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Hard</p>
                <p className={cn("font-mono text-lg font-bold mt-0.5", iteration.all_hard_passed ? "text-emerald-500" : "text-rose-500")}>
                  {Math.round(iteration.hard_score! * 100)}%
                </p>
              </div>
              <div className="h-8 w-px bg-border" />
              <div>
                <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Soft</p>
                <p className={cn("font-mono text-lg font-bold mt-0.5", scoreColor(iteration.soft_score!))}>
                  {Math.round(iteration.soft_score! * 100)}%
                </p>
              </div>
              <div className="h-8 w-px bg-border" />
              <div>
                <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Passed</p>
                <p className="font-mono text-lg font-bold mt-0.5 text-foreground">{passedCount}/{iteration.total_evals}</p>
              </div>
            </motion.div>
          )}

          {/* Architect reasoning */}
          <motion.div
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.1, duration: 0.4, ease }}
            className="mb-10"
          >
            <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40 mb-2">What changed</p>
            <p className="text-[13px] text-foreground/80 leading-relaxed">{iteration.architect_reasoning}</p>
          </motion.div>

          {/* Eval bar */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.15, duration: 0.3 }}
            className="flex items-center gap-1 mb-4"
          >
            {iteration.evals.map((evalItem) => (
              <motion.span
                key={evalItem.test_name}
                className={cn(
                  "h-1.5 flex-1 rounded-full",
                  evalItem.status === "completed" && evalItem.passed && "bg-emerald-500",
                  evalItem.status === "completed" && !evalItem.passed && "bg-rose-500",
                  evalItem.status === "running" && "bg-primary animate-pulse",
                  evalItem.status === "pending" && "bg-muted-foreground/8",
                  evalItem.status === "judging" && "bg-amber-500 animate-pulse",
                )}
                initial={{ scaleX: 0 }}
                animate={{ scaleX: 1 }}
                transition={{ duration: 0.3, ease }}
              />
            ))}
          </motion.div>

          {/* Eval list */}
          <motion.div
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2, duration: 0.4, ease }}
          >
            <div className="flex items-center justify-between mb-2">
              <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">
                Evals {completedCount}/{iteration.total_evals}
              </p>
            </div>

            <div className="flex flex-col">
              {iteration.evals.map((evalItem) => (
                <EvalDetail key={evalItem.test_name} evalItem={evalItem} />
              ))}
            </div>
          </motion.div>
        </div>
      </div>
    </div>
  )
}

function FieldLabel({ htmlFor, children, tooltip }: { htmlFor?: string; children: React.ReactNode; tooltip?: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <Label htmlFor={htmlFor} className="text-[13px]">{children}</Label>
      {tooltip && (
        <Tooltip>
          <TooltipTrigger className="cursor-default">
            <HugeiconsIcon icon={InformationCircleIcon} size={13} className="text-muted-foreground/30" />
          </TooltipTrigger>
          <TooltipContent side="right" className="max-w-56 text-xs leading-relaxed">
            {tooltip}
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  )
}

function AddEvalDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [testName, setTestName] = useState("")
  const [testPrompt, setTestPrompt] = useState("")
  const [expectedBehavior, setExpectedBehavior] = useState("")
  const [tier, setTier] = useState("standard")
  const [requirementType, setRequirementType] = useState("soft")
  const [category, setCategory] = useState("happy_path")
  const [sampleCount, setSampleCount] = useState(3)

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    // Static demo — just close
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton className="sm:max-w-lg max-h-[85dvh] overflow-y-auto">
        <DialogTitle>Add test case</DialogTitle>

        <form onSubmit={handleSubmit} className="flex flex-col gap-5 mt-2">
          {/* Test name */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="eval-name" className="text-[13px]">Name</Label>
            <Input
              id="eval-name"
              value={testName}
              onChange={(event) => setTestName(event.target.value)}
              placeholder="e.g. Angry customer de-escalation"
              autoFocus
            />
          </div>

          {/* Test prompt */}
          <div className="flex flex-col gap-1.5">
            <FieldLabel htmlFor="eval-prompt" tooltip="The exact message sent to your agent during each test run. Write it as a real user would.">Test prompt</FieldLabel>
            <Textarea
              id="eval-prompt"
              value={testPrompt}
              onChange={(event) => setTestPrompt(event.target.value)}
              placeholder="The message that will be sent to the agent during evaluation..."
              rows={3}
            />
            <p className="text-[11px] text-muted-foreground/50">This exact message will be sent to the agent in each sample run.</p>
          </div>

          {/* Expected behavior */}
          <div className="flex flex-col gap-1.5">
            <FieldLabel htmlFor="eval-expected" tooltip="Describe what a correct response looks like. The judge LLM scores the agent's actual response against this.">Expected behavior</FieldLabel>
            <Textarea
              id="eval-expected"
              value={expectedBehavior}
              onChange={(event) => setExpectedBehavior(event.target.value)}
              placeholder="Describe what the agent should do when it receives this prompt..."
              rows={3}
            />
            <p className="text-[11px] text-muted-foreground/50">The judge will score the agent&apos;s response against this expectation.</p>
          </div>

          {/* Tier */}
          <div className="flex flex-col gap-1.5">
            <FieldLabel tooltip="Basic evals test fundamental correctness and must always pass. Standard tests typical behavior. Adversarial tests edge cases and robustness.">Tier</FieldLabel>
            <ToggleGroup
              value={[tier]}
              onValueChange={(value) => { if (value.length) setTier(value[0]) }}
              variant="outline"
              size="sm"
              className="w-full"
            >
              {(["basic", "standard", "adversarial"] as const).map((option) => (
                <ToggleGroupItem key={option} value={option} className="flex-1 text-[12px]">
                  {option}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>

          {/* Requirement type */}
          <div className="flex flex-col gap-1.5">
            <FieldLabel tooltip="Hard evals are pass/fail — the agent either meets the criteria or doesn't. Soft evals allow partial credit with a score between 0 and 1.">Requirement type</FieldLabel>
            <ToggleGroup
              value={[requirementType]}
              onValueChange={(value) => { if (value.length) setRequirementType(value[0]) }}
              variant="outline"
              size="sm"
            >
              <ToggleGroupItem value="hard" className="text-[12px]">Hard</ToggleGroupItem>
              <ToggleGroupItem value="soft" className="text-[12px]">Soft</ToggleGroupItem>
            </ToggleGroup>
            <p className="text-[11px] text-muted-foreground/50">{requirementType === "hard" ? "Binary pass/fail — no partial credit" : "Allows partial scores between 0 and 1"}</p>
          </div>

          {/* Category */}
          <div className="flex flex-col gap-1.5">
            <FieldLabel tooltip="Happy path tests normal usage. Edge case tests unusual inputs. Adversarial tests hostile/tricky inputs. Tool error tests how the agent handles tool failures.">Category</FieldLabel>
            <ToggleGroup
              value={[category]}
              onValueChange={(value) => { if (value.length) setCategory(value[0]) }}
              variant="outline"
              size="sm"
              className="w-full"
            >
              {(["happy_path", "edge_case", "adversarial", "tool_error"] as const).map((option) => (
                <ToggleGroupItem key={option} value={option} className="flex-1 text-[12px]">
                  {option.replaceAll("_", " ")}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>

          {/* Sample count */}
          <div className="flex flex-col gap-1.5">
            <FieldLabel tooltip="How many times to run this test per iteration. More samples give more reliable results but use more tokens. Use 1 for deterministic checks, 3-5 for behavioral tests.">Samples</FieldLabel>
            <ToggleGroup
              value={[String(sampleCount)]}
              onValueChange={(value) => { if (value.length) setSampleCount(Number(value[0])) }}
              variant="outline"
              size="sm"
            >
              {[1, 2, 3, 4, 5].map((count) => (
                <ToggleGroupItem key={count} value={String(count)} className="w-9 font-mono text-[13px]">
                  {count}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            <p className="text-[11px] text-muted-foreground/50">More samples = more reliable but costs more tokens</p>
          </div>

          {/* Submit */}
          <Button type="submit" className="w-full" disabled={!testName.trim() || !testPrompt.trim()}>
            Add test case
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function EvalCaseCard({ evalCase, index }: { evalCase: typeof EVAL_CASES[number]; index: number }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: index * 0.03, duration: 0.3, ease: [0.22, 1, 0.36, 1] }}
      className="rounded-2xl border border-border overflow-hidden"
    >
      {/* Header */}
      <div className="flex items-start gap-3 px-5 py-4">
        <span className="font-mono text-[10px] text-muted-foreground/30 mt-1 w-5 shrink-0 tabular-nums">{index + 1}</span>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className="text-[13px] font-medium text-foreground">{evalCase.test_name}</span>
            <Badge variant="secondary" className="text-[9px]">{evalCase.tier}</Badge>
            <span className={cn(
              "font-mono text-[10px] px-1.5 py-0.5 rounded",
              evalCase.requirement_type === "hard" ? "bg-muted text-muted-foreground" : "text-muted-foreground/40",
            )}>
              {evalCase.requirement_type}
            </span>
          </div>
          <p className="text-[12px] text-muted-foreground/60 leading-relaxed line-clamp-2">{evalCase.test_prompt}</p>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors">
            <HugeiconsIcon icon={Edit02Icon} size={13} className="text-muted-foreground/40" />
          </button>
          <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-destructive/10 transition-colors">
            <HugeiconsIcon icon={Delete02Icon} size={13} className="text-muted-foreground/40 hover:text-destructive" />
          </button>
          <button
            onClick={() => setExpanded(!expanded)}
            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors"
          >
            <HugeiconsIcon
              icon={ArrowDown01Icon}
              size={12}
              className={cn("text-muted-foreground/30 transition-transform duration-200", expanded && "rotate-180")}
            />
          </button>
        </div>
      </div>

      {/* Expanded details */}
      <div className="grid transition-all duration-300" style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}>
        <div className="overflow-hidden">
          <div className="px-5 pb-4 pt-0 ml-8 flex flex-col gap-4 border-t border-border/50 pt-4">
            {/* Expected behavior */}
            <div>
              <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-1.5">Expected behavior</p>
              <p className="text-[12px] text-foreground/80 leading-relaxed">{evalCase.expected_behavior}</p>
            </div>

            {/* Test prompt */}
            <div>
              <p className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 mb-1.5">Test prompt</p>
              <div className="rounded-xl bg-muted/30 px-3.5 py-2.5">
                <p className="text-[12px] text-foreground leading-relaxed">{evalCase.test_prompt}</p>
              </div>
            </div>

            {/* Meta row */}
            <div className="flex items-center gap-4 text-[10px] font-mono tabular-nums text-muted-foreground/40">
              <span>{evalCase.sample_count} samples</span>
              <span>{evalCase.category}</span>
              <span>{evalCase.tier} tier</span>
              <span>{evalCase.requirement_type}</span>
            </div>
          </div>
        </div>
      </div>
    </motion.div>
  )
}

function EvalsPanel() {
  const [addOpen, setAddOpen] = useState(false)

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-xl mx-auto px-6 py-10">
          {/* Header */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.4, ease: [0.22, 1, 0.36, 1] }}
            className="flex items-start justify-between mb-8"
          >
            <div>
              <div className="flex items-center gap-2 mb-1">
                <HugeiconsIcon icon={CheckListIcon} size={14} className="text-primary" />
                <h2 className="font-heading text-base font-semibold text-foreground">Test Cases</h2>
              </div>
              <p className="text-[13px] text-muted-foreground/60 max-w-sm">
                These test cases will be used to evaluate each iteration. Edit, add, or remove before approving.
              </p>
            </div>
          </motion.div>

          {/* Stats */}
          <motion.div
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.05, duration: 0.35, ease: [0.22, 1, 0.36, 1] }}
            className="flex items-center gap-6 mb-8 text-sm"
          >
            <div>
              <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Total</p>
              <p className="font-mono text-lg font-bold tabular-nums mt-0.5 text-foreground">{EVAL_CASES.length}</p>
            </div>
            <div className="h-8 w-px bg-border" />
            <div>
              <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Hard</p>
              <p className="font-mono text-lg font-bold tabular-nums mt-0.5 text-foreground">
                {EVAL_CASES.filter((evalCase) => evalCase.requirement_type === "hard").length}
              </p>
            </div>
            <div className="h-8 w-px bg-border" />
            <div>
              <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Soft</p>
              <p className="font-mono text-lg font-bold tabular-nums mt-0.5 text-foreground">
                {EVAL_CASES.filter((evalCase) => evalCase.requirement_type === "soft").length}
              </p>
            </div>
            <div className="h-8 w-px bg-border" />
            <div>
              <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">Tiers</p>
              <div className="flex items-center gap-1.5 mt-1">
                {["basic", "standard", "adversarial"].map((tier) => {
                  const count = EVAL_CASES.filter((evalCase) => evalCase.tier === tier).length
                  return count > 0 ? (
                    <Badge key={tier} variant="secondary" className="text-[9px]">{tier} ({count})</Badge>
                  ) : null
                })}
              </div>
            </div>
          </motion.div>

          {/* Eval list */}
          <div className="flex flex-col gap-2">
            {EVAL_CASES.map((evalCase, index) => (
              <EvalCaseCard key={evalCase.test_name} evalCase={evalCase} index={index} />
            ))}

            <Button variant="secondary" className="w-full h-12" onClick={() => setAddOpen(true)}>
              <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
              Add test case
            </Button>
          </div>

          <AddEvalDialog open={addOpen} onOpenChange={setAddOpen} />

          {/* Approve */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.3 }}
            className="flex items-center gap-3 mt-10 pt-8 border-t border-border"
          >
            <Button >
              <HugeiconsIcon icon={Tick02Icon} size={14} data-icon="inline-start" />
              Approve & start forge
            </Button>
            <Button variant="ghost" className="text-muted-foreground">
              Regenerate
            </Button>
          </motion.div>
        </div>
      </div>
    </div>
  )
}

function ResultsPanel() {
  const scores = [62, 78, 92]

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-xl mx-auto px-6 py-10">
          {/* Hero */}
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.6, ease }} className="mb-16 text-center">
            <motion.p
              className="font-mono text-7xl font-black tabular-nums tracking-tighter text-emerald-500 leading-none"
              initial={{ scale: 0.85, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              transition={{ delay: 0.1, duration: 0.5, ease }}
            >
              92
            </motion.p>

            <motion.p
              className="text-sm text-muted-foreground mt-3"
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.3, duration: 0.4, ease }}
            >
              Optimized in 3 iterations
            </motion.p>

            <motion.div
              className="flex items-center justify-center gap-2 mt-5"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.45 }}
            >
              {scores.map((score, index) => (
                <div key={index} className="flex items-center gap-2">
                  {index > 0 && <span className="text-muted-foreground/15">→</span>}
                  <span className={cn("font-mono text-xs font-semibold tabular-nums", scoreColor(score / 100))}>{score}</span>
                </div>
              ))}
            </motion.div>

            <motion.div
              className="flex items-center justify-center gap-8 mt-8"
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.5, duration: 0.4, ease }}
            >
              {[
                { label: "Hard evals", value: "5/5", color: "text-emerald-500" },
                { label: "Soft score", value: "87%", color: "text-foreground" },
                { label: "Iterations", value: "3", color: "text-foreground" },
              ].map((stat) => (
                <div key={stat.label}>
                  <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40">{stat.label}</p>
                  <p className={cn("font-mono text-lg font-bold tabular-nums mt-0.5", stat.color)}>{stat.value}</p>
                </div>
              ))}
            </motion.div>
          </motion.div>

          {/* Prompt */}
          <motion.div
            initial={{ opacity: 0, y: 24 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.3, duration: 0.5, ease }}
            className="mb-10"
          >
            <p className="font-mono text-[9px] font-medium uppercase tracking-[2px] text-muted-foreground/40 mb-4">Generated system prompt</p>
            <div className="rounded-2xl border border-border p-6">
              <div className="text-[13px] text-foreground leading-relaxed prose prose-sm prose-neutral dark:prose-invert max-w-none prose-p:my-2 prose-headings:mt-5 prose-headings:mb-2 prose-li:my-0.5 prose-ul:my-2 prose-ol:my-2 prose-strong:text-foreground">
                <Streamdown>{RESULT_PROMPT}</Streamdown>
              </div>
            </div>
          </motion.div>

          {/* Actions */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.6 }}
            className="flex items-center gap-3 pt-8 border-t border-border"
          >
            <Button >
              <HugeiconsIcon icon={Tick02Icon} size={14} data-icon="inline-start" />
              Apply to agent
            </Button>
            <Button variant="ghost" className="text-muted-foreground">Discard</Button>
          </motion.div>
        </div>
      </div>
    </div>
  )
}

// ─── Page ───────────────────────────────────────────────────────────────────

export default function ForgePage() {
  const params = useParams<{ id: string }>()
  const [activeNav, setActiveNav] = useState<NavId>("iteration-3")

  const { data: agent, isLoading: agentLoading } = $api.useQuery(
    "get",
    "/v1/agents/{id}",
    { params: { path: { id: params.id } } },
  )

  useEffect(() => {
    if (agent) {
      console.log("[forge] agent loaded", agent)
    }
  }, [agent])

  function renderContent() {
    if (activeNav === "context") return <ContextPanel />
    if (activeNav === "evals") return <EvalsPanel />
    if (activeNav === "results") return <ResultsPanel />

    const iterationNumber = parseInt(activeNav.split("-")[1])
    const iteration = ITERATIONS.find((iter) => iter.number === iterationNumber)
    if (iteration) return <IterationPanel iteration={iteration} />

    return null
  }

  return (
    <div className="flex h-[calc(100vh-54px)]">
      <Sidebar activeId={activeNav} onSelect={setActiveNav} />
      <div className="flex-1 overflow-hidden">
        <AnimatePresence mode="wait">
          <motion.div
            key={activeNav}
            className="h-full"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.12 }}
          >
            {renderContent()}
          </motion.div>
        </AnimatePresence>
      </div>
    </div>
  )
}
