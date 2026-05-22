"use client"

import { useMemo, useState } from "react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  CheckmarkCircle02Icon,
  CommandIcon,
  SearchIcon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"

const categories = [
  "All categories",
  "Research",
  "Product",
  "Engineering",
  "Sales",
  "Operations",
]

const skills = [
  {
    title: "Browser research",
    category: "Research",
    description:
      "Search the web, inspect pages, collect source material, and return concise research briefs.",
    tags: ["research", "browser", "citations", "summaries"],
    content: `## What it does

Browser research helps Hivy gather facts from public web pages, compare sources, and return compact notes with attribution.

## Useful for

- Market and competitor research
- Vendor comparisons
- Source-backed summaries
- Briefs before calls or planning sessions

## Output style

The skill returns a short brief with source links, key findings, and clear uncertainty when the available evidence is thin.`,
  },
  {
    title: "PR review",
    category: "Engineering",
    description:
      "Review pull requests for regressions, missing tests, auth gaps, and unclear implementation details.",
    tags: ["github", "code-review", "tests", "quality"],
    content: `## What it does

PR review inspects code changes and reports concrete risks before merge.

## Review focus

- Behavioral regressions
- Missing test coverage
- Auth and org-scope gaps
- Unclear control flow
- Risky edge cases

## Result

Hivy returns findings ordered by severity with file references and a short test recommendation.`,
  },
  {
    title: "Customer reply drafting",
    category: "Sales",
    description:
      "Turn context from conversations and account notes into clear, on-brand customer responses.",
    tags: ["email", "support", "crm", "writing"],
    content: `## What it does

Customer reply drafting turns support context, account details, and previous messages into ready-to-edit customer responses.

## Drafting rules

- Keep the answer direct
- Preserve a helpful tone
- Avoid promises the team has not confirmed
- Include the next action when one exists`,
  },
  {
    title: "Changelog writer",
    category: "Product",
    description:
      "Convert merged work into polished release notes with user-facing language and clean grouping.",
    tags: ["releases", "product", "copy", "updates"],
    content: `## What it does

Changelog writer turns shipped work into release notes that users can understand without reading implementation details.

## Sections

- New
- Improved
- Fixed
- Developer notes

## Example

\`Fixed a Slack connection issue that could leave reconnect attempts stuck after OAuth expired.\``,
  },
  {
    title: "Incident summary",
    category: "Operations",
    description:
      "Summarize timelines, decisions, owners, and follow-up work from incidents and support threads.",
    tags: ["ops", "timeline", "postmortem", "slack"],
    content: `## What it does

Incident summary converts noisy incident threads into a clean record of what happened and what remains.

## Captures

- Timeline
- Customer impact
- Root cause notes
- Decisions made
- Follow-up owners`,
  },
  {
    title: "Docs maintenance",
    category: "Product",
    description:
      "Find stale documentation, propose edits, and keep public docs aligned with shipped behavior.",
    tags: ["docs", "product", "accuracy", "maintenance"],
    content: `## What it does

Docs maintenance compares product behavior against documentation and proposes edits where the docs are stale or incomplete.

## Good inputs

- A feature area
- A changed route or API
- A support question that exposed confusion`,
  },
  {
    title: "Test plan generator",
    category: "Engineering",
    description:
      "Create targeted test plans for backend, frontend, integration, and manual verification flows.",
    tags: ["testing", "qa", "coverage", "verification"],
    content: `## What it does

Test plan generator creates focused verification plans for a change.

## Includes

- Narrow automated tests
- Broader regression checks
- Manual verification steps
- Known untested risk`,
  },
  {
    title: "Lead enrichment",
    category: "Sales",
    description:
      "Research accounts, identify buying signals, and prepare compact context for sales follow-up.",
    tags: ["sales", "accounts", "research", "pipeline"],
    content: `## What it does

Lead enrichment prepares account context before outreach or sales calls.

## Research areas

- Company profile
- Recent signals
- Likely pain points
- Relevant integrations
- Suggested opener`,
  },
  {
    title: "Meeting brief",
    category: "Operations",
    description:
      "Prepare agendas, participant context, decisions needed, and follow-up items for team meetings.",
    tags: ["meetings", "agenda", "notes", "planning"],
    content: `## What it does

Meeting brief prepares a compact read-ahead for a team meeting.

## Brief structure

1. Context
2. Decisions needed
3. Suggested agenda
4. Open questions
5. Follow-up template`,
  },
]

type Skill = (typeof skills)[number]

export default function SkillsPage() {
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState(categories[0])
  const [installed, setInstalled] = useState<string[]>([])
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null)

  const filteredSkills = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()

    return skills.filter((skill) => {
      const matchesCategory =
        category === "All categories" || skill.category === category
      const matchesQuery =
        normalizedQuery.length === 0 ||
        [skill.title, skill.description, skill.category, ...skill.tags]
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery)

      return matchesCategory && matchesQuery
    })
  }, [category, query])

  function toggleInstall(title: string) {
    setInstalled((current) =>
      current.includes(title)
        ? current.filter((item) => item !== title)
        : [...current, title]
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-7">
      <div className="flex flex-col gap-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="max-w-2xl">
            <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground md:text-4xl">
              Skills
            </h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              Install focused capabilities that help Hivy research, write,
              review, and operate across your workspace.
            </p>
          </div>

          <Button type="button" className="w-full sm:w-auto">
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            Create skill
          </Button>
        </div>

        <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative min-w-0 flex-1">
            <HugeiconsIcon
              icon={SearchIcon}
              size={16}
              className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search skills"
              className="h-11 rounded-md bg-card pl-9"
            />
          </div>

          <Select
            value={category}
            onValueChange={(value) => {
              if (value) setCategory(value)
            }}
          >
            <SelectTrigger className="h-11 w-full rounded-md border-border bg-card px-3 sm:w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              {categories.map((item) => (
                <SelectItem key={item} value={item}>
                  {item}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="flex flex-col gap-4">
        {filteredSkills.map((skill) => {
          const isInstalled = installed.includes(skill.title)

          return (
            <article
              role="button"
              tabIndex={0}
              key={skill.title}
              className="flex cursor-pointer gap-4 rounded-md border border-border bg-card p-5 text-left transition-colors hover:border-muted-foreground/25 hover:bg-muted/20 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 focus-visible:outline-none"
              onClick={() => setSelectedSkill(skill)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault()
                  setSelectedSkill(skill)
                }
              }}
            >
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                <HugeiconsIcon icon={CommandIcon} size={20} />
              </div>

              <div className="min-w-0 flex-1">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                  <h2 className="text-base font-semibold text-foreground">
                    {skill.title}
                  </h2>
                </div>

                <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
                  {skill.description}
                </p>

                <div className="mt-4 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
                  <div className="flex flex-wrap gap-2">
                    {skill.tags.map((tag) => (
                      <span
                        key={tag}
                        className="rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground"
                      >
                        #{tag}
                      </span>
                    ))}
                  </div>

                  <Button
                    type="button"
                    variant={isInstalled ? "outline" : "secondary"}
                    className={cn(
                      "w-full shrink-0 sm:w-32",
                      isInstalled && "text-primary"
                    )}
                    onClick={(event) => {
                      event.stopPropagation()
                      toggleInstall(skill.title)
                    }}
                  >
                    {isInstalled ? (
                      <HugeiconsIcon
                        icon={CheckmarkCircle02Icon}
                        size={16}
                        className="mr-2"
                        data-icon="inline-start"
                      />
                    ) : null}
                    {isInstalled ? "Installed" : "Install"}
                  </Button>
                </div>
              </div>
            </article>
          )
        })}
      </div>

      <SkillDialog
        skill={selectedSkill}
        installed={selectedSkill ? installed.includes(selectedSkill.title) : false}
        onOpenChange={(open) => {
          if (!open) setSelectedSkill(null)
        }}
        onInstall={() => {
          if (selectedSkill) toggleInstall(selectedSkill.title)
        }}
      />
    </div>
  )
}

function SkillDialog({
  skill,
  installed,
  onOpenChange,
  onInstall,
}: {
  skill: Skill | null
  installed: boolean
  onOpenChange: (open: boolean) => void
  onInstall: () => void
}) {
  return (
    <Dialog open={Boolean(skill)} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88dvh] overflow-hidden rounded-md sm:max-w-xl">
        {skill ? (
          <>
            <DialogHeader className="pr-10">
              <div className="flex flex-wrap items-center gap-2">
                <DialogTitle className="text-xl">{skill.title}</DialogTitle>
              </div>
              <DialogDescription>{skill.description}</DialogDescription>
            </DialogHeader>

            <div className="max-h-[52dvh] overflow-y-auto rounded-md border border-border bg-background p-5">
              <MarkdownContent content={skill.content} />
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant={installed ? "outline" : "secondary"}
                className={cn("w-full sm:w-auto", installed && "text-primary")}
                onClick={onInstall}
              >
                {installed ? (
                  <HugeiconsIcon
                    icon={CheckmarkCircle02Icon}
                    size={16}
                    className="mr-2"
                    data-icon="inline-start"
                  />
                ) : null}
                {installed ? "Installed" : "Install"}
              </Button>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function MarkdownContent({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        h2: ({ children }) => (
          <h2 className="mt-6 first:mt-0 font-heading text-lg font-medium tracking-[-0.01em] text-foreground">
            {children}
          </h2>
        ),
        p: ({ children }) => (
          <p className="mt-3 text-sm leading-6 text-muted-foreground">
            {children}
          </p>
        ),
        ul: ({ children }) => (
          <ul className="mt-3 list-disc space-y-1.5 pl-5 text-sm leading-6 text-muted-foreground">
            {children}
          </ul>
        ),
        ol: ({ children }) => (
          <ol className="mt-3 list-decimal space-y-1.5 pl-5 text-sm leading-6 text-muted-foreground">
            {children}
          </ol>
        ),
        code: ({ children }) => (
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
            {children}
          </code>
        ),
      }}
    >
      {content}
    </ReactMarkdown>
  )
}
