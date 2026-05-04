"use client"

import { useEffect, useState } from "react"
import { useWatch } from "react-hook-form"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon, Loading03Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { Avatar, AvatarBadge, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { StepHeader } from "./step-header"
import { useOnboarding } from "./context"

type Stage = {
  id: string
  label: (name: string) => string
  /** ms this stage spends in flight before completing. */
  duration: number
}

const STAGES: Stage[] = [
  { id: "workspace", label: () => "Provisioning workspace", duration: 900 },
  { id: "compute", label: (n) => `Setting up ${n}'s computer`, duration: 1200 },
  { id: "chrome", label: (n) => `Installing Chrome for ${n}`, duration: 1500 },
  { id: "memory", label: () => "Initializing long-term memory", duration: 800 },
  { id: "skills", label: (n) => `Wiring ${n}'s default skills`, duration: 1000 },
]

export function ProvisioningStep() {
  const { form, goNext } = useOnboarding()
  const watchedName = useWatch({ control: form.control, name: "agentName" })
  const watchedAvatar = useWatch({ control: form.control, name: "agentAvatarUrl" })
  const agentName = watchedName?.trim() || "your AI employee"
  const agentAvatarUrl = watchedAvatar?.trim() || ""

  const [activeIndex, setActiveIndex] = useState(0)

  useEffect(() => {
    if (activeIndex >= STAGES.length) return
    const timer = setTimeout(
      () => setActiveIndex((idx) => idx + 1),
      STAGES[activeIndex].duration,
    )
    return () => clearTimeout(timer)
  }, [activeIndex])

  const isDone = activeIndex >= STAGES.length
  const progress = isDone ? 100 : Math.round((activeIndex / STAGES.length) * 100)

  return (
    <div className="w-full">
      <AnimatePresence mode="wait">
        {isDone ? (
          <motion.div
            key="done"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.22, ease: [0.22, 1, 0.36, 1] }}
            className="mx-auto flex w-full max-w-xl flex-col items-center gap-12 pt-6"
          >
            <DoneHeader agentName={agentName} agentAvatarUrl={agentAvatarUrl} />
            <Button onClick={goNext} className="flex h-11 items-center gap-2 px-5">
              Connect {agentName} to your workspace
              <HugeiconsIcon icon={ArrowRight01Icon} className="size-4" />
            </Button>
          </motion.div>
        ) : (
          <motion.div
            key="loading"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0, y: -8 }}
            transition={{ duration: 0.18 }}
            className="mx-auto flex w-full max-w-md flex-col gap-8"
          >
            <StepHeader
              title={`Setting up ${agentName}`}
              description="This takes a moment. We're provisioning your AI employee's workspace, tools, and memory."
            />

            <div className="flex flex-col gap-5">
              <ol className="flex flex-col gap-3">
                {STAGES.map((stage, idx) => {
                  const status =
                    idx < activeIndex ? "done" : idx === activeIndex ? "active" : "pending"
                  return (
                    <StageRow
                      key={stage.id}
                      label={stage.label(agentName)}
                      status={status}
                    />
                  )
                })}
              </ol>
              <Progress value={progress} />
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

function StageRow({
  label,
  status,
}: {
  label: string
  status: "done" | "active" | "pending"
}) {
  return (
    <li className="flex items-center gap-3">
      <span className="flex size-5 shrink-0 items-center justify-center">
        {status === "done" ? (
          <span className="flex size-5 items-center justify-center rounded-full bg-success/15 text-success">
            <HugeiconsIcon icon={Tick02Icon} className="size-3" strokeWidth={2.75} />
          </span>
        ) : status === "active" ? (
          <HugeiconsIcon
            icon={Loading03Icon}
            className="size-4 animate-spin text-primary"
            strokeWidth={2}
          />
        ) : (
          <span className="size-1.5 rounded-full bg-muted-foreground/30" />
        )}
      </span>
      <span
        className={
          status === "pending"
            ? "text-sm text-muted-foreground/70"
            : status === "done"
              ? "text-sm text-muted-foreground line-through decoration-muted-foreground/40"
              : "text-sm font-medium text-foreground"
        }
      >
        {label}
      </span>
    </li>
  )
}

function DoneHeader({
  agentName,
  agentAvatarUrl,
}: {
  agentName: string
  agentAvatarUrl: string
}) {
  const initial = agentName.charAt(0).toUpperCase() || "?"

  return (
    <header className="flex flex-col items-center gap-6 text-center">
      <motion.div
        initial={{ scale: 0.7, opacity: 0 }}
        animate={{ scale: 1, opacity: 1 }}
        transition={{ duration: 0.32, ease: [0.22, 1, 0.36, 1] }}
      >
        <Avatar className="size-24 rounded-md after:rounded-md">
          {agentAvatarUrl ? (
            <AvatarImage src={agentAvatarUrl} alt={agentName} className="rounded-md" />
          ) : null}
          <AvatarFallback
            className="rounded-md text-3xl font-semibold"
            style={{ fontFamily: "var(--font-sora), sans-serif" }}
          >
            {initial}
          </AvatarFallback>
          <AvatarBadge className="right-1 bottom-1 size-5 bg-success" />
        </Avatar>
      </motion.div>

      <div className="flex flex-col items-center gap-2.5">
        <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-success">
          <span className="relative flex size-1.5">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-success opacity-70" />
            <span className="relative inline-flex size-1.5 rounded-full bg-success" />
          </span>
          Online
        </div>
        <h1
          className="text-3xl font-semibold tracking-tight"
          style={{ fontFamily: "var(--font-sora), sans-serif" }}
        >
          Hi, I&apos;m {agentName}.
        </h1>
        <p className="max-w-xs text-sm leading-relaxed text-muted-foreground">
          Workspace, tools, and memory all wired up. Where should I get to work?
        </p>
      </div>
    </header>
  )
}
