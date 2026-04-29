"use client"

import { useEffect, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { toast } from "sonner"
import { PageHeader } from "@/components/page-header"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  ArrowUp02Icon,
  CodeSquareIcon,
  Search01Icon,
  SparklesIcon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

type AgentKey = "coder" | "researcher"

interface AgentDef {
  key: AgentKey
  label: string
  blurb: string
  avatar: string
  icon: typeof CodeSquareIcon
  placeholder: string
}

const AGENTS: AgentDef[] = [
  {
    key: "coder",
    label: "Code companion",
    blurb: "Clones any GitHub repo, navigates the code, edits files, opens a PR.",
    avatar: "https://api.dicebear.com/9.x/notionists/svg?seed=Sawyer&backgroundColor=ffd5dc,c0aede",
    icon: CodeSquareIcon,
    placeholder:
      "Paste a repo URL or describe what you want changed. The agent clones, edits, and pushes a branch.",
  },
  {
    key: "researcher",
    label: "Deep research",
    blurb: "Browses the web, synthesizes sources, returns a structured report.",
    avatar: "https://api.dicebear.com/9.x/notionists/svg?seed=Harper&backgroundColor=b6e3f4,d1d4f9",
    icon: Search01Icon,
    placeholder:
      "Ask a research question. The agent browses, cross-references sources, and writes a structured report.",
  },
]

const PROMPTS: Record<AgentKey, string[]> = {
  coder: [
    "Clone vercel/next.js and tell me what's confusing in the README for a new contributor",
    "Find the slowest test in shadcn-ui/ui and propose a fix",
    "Open a draft PR adding a CONTRIBUTING.md to a small repo of your choosing",
    "Summarize how reconciliation handles fragments in facebook/react, with file references",
  ],
  researcher: [
    "Top 10 AI coding agents launched in 2026 with pricing and seat counts",
    "European YC-backed dev tools founded after 2024, sorted by funding",
    "Compare Daytona, e2b, and Vercel Sandbox pricing for 1000 hours per month",
    "Summarize the last week of news about open-weight LLMs",
  ],
}

export default function WorkspaceHome() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [agent, setAgent] = useState<AgentKey>("coder")
  const [draft, setDraft] = useState("")
  const [agentOpen, setAgentOpen] = useState(false)

  useEffect(() => {
    if (searchParams.get("checkout") === "success") {
      toast.success("Subscription activated! You're on the Pro plan.")
      router.replace("/w")
    }
  }, [searchParams, router])

  const selectedAgent = AGENTS.find((a) => a.key === agent)!

  return (
    <>
      <PageHeader title="Home" />
      <div className="mx-auto w-full max-w-3xl px-6 pt-16 pb-24">
        <div className="mb-8">
          <h1 className="font-heading text-[28px] font-medium leading-tight tracking-tight text-foreground">
            What do you want shipped first?
          </h1>
          <p className="mt-2 text-[14px] text-muted-foreground">
            Two starter agents. No setup, no integrations. Pick one, give it a
            task, watch it work.
          </p>
        </div>

        <div className="rounded-2xl border border-border bg-background transition-colors focus-within:border-foreground/30">
          <textarea
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder={selectedAgent.placeholder}
            className="block h-[150px] w-full resize-none bg-transparent px-4 pt-4 text-[14.5px] text-foreground outline-none placeholder:text-muted-foreground/70"
          />
          <div className="flex items-center justify-between gap-2 px-3 pt-1 pb-3">
            <Popover open={agentOpen} onOpenChange={setAgentOpen}>
              <PopoverTrigger
                render={
                  <button
                    type="button"
                    className="flex items-center gap-2 rounded-full border border-border/70 py-1 pl-1 pr-3 text-[12.5px] text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
                  >
                    <img
                      src={selectedAgent.avatar}
                      alt=""
                      className="h-6 w-6 shrink-0 rounded-full"
                    />
                    <span className="font-medium text-foreground">
                      {selectedAgent.label}
                    </span>
                    <HugeiconsIcon
                      icon={ArrowDown01Icon}
                      size={12}
                      className="opacity-60"
                    />
                  </button>
                }
              />
              <PopoverContent align="start" className="w-[340px] p-1.5">
                <div className="px-2.5 pt-2 pb-1.5 text-[10.5px] font-medium uppercase tracking-wide text-muted-foreground">
                  Choose an agent
                </div>
                <div className="space-y-0.5">
                  {AGENTS.map((a) => {
                    const isActive = a.key === agent
                    return (
                      <button
                        key={a.key}
                        type="button"
                        onClick={() => {
                          setAgent(a.key)
                          setAgentOpen(false)
                        }}
                        className={`flex w-full items-start gap-3 rounded-lg px-2.5 py-2.5 text-left transition-colors ${
                          isActive ? "bg-muted/60" : "hover:bg-muted/40"
                        }`}
                      >
                        <img
                          src={a.avatar}
                          alt=""
                          className="h-9 w-9 shrink-0 rounded-full"
                        />
                        <div className="min-w-0 flex-1">
                          <div className="text-[13.5px] font-medium text-foreground">
                            {a.label}
                          </div>
                          <div className="mt-0.5 text-[11.5px] leading-relaxed text-muted-foreground">
                            {a.blurb}
                          </div>
                        </div>
                        <HugeiconsIcon
                          icon={Tick02Icon}
                          size={13}
                          className={`mt-1 shrink-0 text-primary ${
                            isActive ? "" : "opacity-0"
                          }`}
                        />
                      </button>
                    )
                  })}
                </div>
              </PopoverContent>
            </Popover>
            <button
              type="button"
              disabled={!draft.trim()}
              className="flex h-9 w-9 items-center justify-center rounded-full bg-primary text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-30"
            >
              <HugeiconsIcon icon={ArrowUp02Icon} size={16} />
            </button>
          </div>
        </div>

        <div className="mt-8">
          <p className="mb-3 text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground/80">
            Try one of these
          </p>
          <div className="flex flex-col">
            {PROMPTS[agent].map((prompt, index) => (
              <button
                key={`${agent}-${index}`}
                type="button"
                onClick={() => setDraft(prompt)}
                className="group flex items-center gap-3 rounded-lg px-2 py-2.5 text-left transition-colors hover:bg-muted/40"
              >
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground transition-colors group-hover:bg-primary/10 group-hover:text-primary">
                  <HugeiconsIcon icon={SparklesIcon} size={11} />
                </span>
                <span className="flex-1 truncate text-[13px] text-foreground/90">
                  {prompt}
                </span>
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  size={13}
                  className="shrink-0 text-muted-foreground/0 transition-all group-hover:translate-x-0.5 group-hover:text-muted-foreground"
                />
              </button>
            ))}
          </div>
        </div>

        <div className="mt-14 flex items-center justify-center text-[11.5px] text-muted-foreground/70">
          When you're ready, build your own agent and connect your own tools.
        </div>
      </div>
    </>
  )
}
