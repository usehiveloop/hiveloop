"use client"

import { useState, useRef } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Dialog, DialogContent, DialogTrigger } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon } from "@hugeicons/core-free-icons"
import { StepChooseMode } from "./step-choose-mode"
import { StepSandboxType } from "./step-sandbox-type"
import { StepIntegrations } from "./step-integrations"
import { StepLlmKey } from "./step-llm-key"
import { StepBasics } from "./step-basics"
import { StepSystemPrompt } from "./step-system-prompt"
import { StepInstructions } from "./step-instructions"
import { StepForgeJudge } from "./step-forge-judge"
import { StepSummary } from "./step-summary"
import { StepMarketplaceBrowse, StepMarketplaceDetail } from "./step-marketplace"
import { scratchSteps, forgeSteps, marketplaceSteps } from "./types"
import type { CreationMode, Step } from "./types"

export function CreateAgentDialog() {
  const [step, setStep] = useState<Step>("mode")
  const [mode, setMode] = useState<CreationMode | null>(null)
  const [open, setOpen] = useState(false)
  const [selectedIntegrations, setSelectedIntegrations] = useState<Set<string>>(new Set())
  const [selectedKeyId, setSelectedKeyId] = useState<string | null>(null)
  const [judgeKeyId, setJudgeKeyId] = useState<string | null>(null)
  const [judgeModel, setJudgeModel] = useState<string | null>(null)
  const [selectedMarketplaceAgent, setSelectedMarketplaceAgent] = useState<string | null>(null)
  const direction = useRef<1 | -1>(1)

  const currentSteps = mode === "marketplace" ? marketplaceSteps : mode === "forge" ? forgeSteps : scratchSteps

  function goTo(next: Step) {
    direction.current = currentSteps.indexOf(next) > currentSteps.indexOf(step) ? 1 : -1
    setStep(next)
  }

  function toggleIntegration(connectionId: string) {
    setSelectedIntegrations((prev) => {
      const next = new Set(prev)
      if (next.has(connectionId)) {
        next.delete(connectionId)
      } else {
        next.add(connectionId)
      }
      return next
    })
  }

  function reset() {
    setStep("mode")
    setMode(null)
    setSelectedIntegrations(new Set())
    setSelectedKeyId(null)
    setJudgeKeyId(null)
    setJudgeModel(null)
    setSelectedMarketplaceAgent(null)
  }

  const variants = {
    enter: (directionValue: number) => ({ x: directionValue > 0 ? 80 : -80, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (directionValue: number) => ({ x: directionValue > 0 ? -80 : 80, opacity: 0 }),
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) reset()
      }}
    >
      <DialogTrigger
        render={
          <Button size="default">
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            Create agent
          </Button>
        }
      />
      <DialogContent className="sm:max-w-md h-[780px] overflow-hidden flex flex-col">
        <div className="flex-1 min-h-0 flex flex-col">
          <AnimatePresence mode="wait" custom={direction.current}>
            <motion.div
              key={step}
              custom={direction.current}
              variants={variants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.2, ease: "easeInOut" as const }}
              className="flex-1 flex flex-col min-h-0"
            >
              {step === "mode" && (
                <StepChooseMode
                  onSelect={(selectedMode) => {
                    setMode(selectedMode)
                    direction.current = 1
                    if (selectedMode === "marketplace") {
                      setStep("marketplace-browse")
                    } else {
                      setStep("sandbox")
                    }
                  }}
                />
              )}
              {step === "marketplace-browse" && (
                <StepMarketplaceBrowse
                  onBack={() => goTo("mode")}
                  onSelect={(slug) => {
                    setSelectedMarketplaceAgent(slug)
                    goTo("marketplace-detail")
                  }}
                />
              )}
              {step === "marketplace-detail" && selectedMarketplaceAgent && (
                <StepMarketplaceDetail
                  slug={selectedMarketplaceAgent}
                  onBack={() => goTo("marketplace-browse")}
                  onInstall={() => setOpen(false)}
                />
              )}
              {step === "sandbox" && (
                <StepSandboxType
                  onBack={() => goTo("mode")}
                  onSelect={() => goTo("integrations")}
                />
              )}
              {step === "integrations" && (
                <StepIntegrations
                  selectedIntegrations={selectedIntegrations}
                  onToggleIntegration={toggleIntegration}
                  onBack={() => goTo("sandbox")}
                  onNext={() => goTo("llm-key")}
                />
              )}
              {step === "llm-key" && (
                <StepLlmKey
                  selectedKey={selectedKeyId}
                  onSelect={(id) => {
                    setSelectedKeyId(id)
                    goTo("basics")
                  }}
                  onBack={() => goTo("integrations")}
                />
              )}
              {step === "basics" && (
                <StepBasics
                  selectedKeyId={selectedKeyId}
                  onBack={() => goTo("llm-key")}
                  onSubmit={() => {
                    if (mode === "scratch") {
                      goTo("system-prompt")
                    } else {
                      goTo("forge-judge")
                    }
                  }}
                />
              )}
              {step === "forge-judge" && (
                <StepForgeJudge
                  selectedKeyId={selectedKeyId}
                  judgeKeyId={judgeKeyId}
                  onSelectKey={(id) => {
                    setJudgeKeyId(id)
                    setJudgeModel(null)
                  }}
                  judgeModel={judgeModel}
                  onSelectModel={setJudgeModel}
                  onBack={() => goTo("basics")}
                  onNext={() => goTo("summary")}
                  onSkip={() => goTo("summary")}
                />
              )}
              {step === "system-prompt" && (
                <StepSystemPrompt
                  onBack={() => goTo("basics")}
                  onNext={() => goTo("instructions")}
                />
              )}
              {step === "instructions" && (
                <StepInstructions
                  onBack={() => goTo("system-prompt")}
                  onNext={() => goTo("summary")}
                />
              )}
              {step === "summary" && (
                <StepSummary
                  mode={mode!}
                  selectedKeyId={selectedKeyId}
                  selectedIntegrations={selectedIntegrations}
                  onBack={() => {
                    if (mode === "scratch") goTo("instructions")
                    else goTo("forge-judge")
                  }}
                  onSubmit={() => setOpen(false)}
                />
              )}
            </motion.div>
          </AnimatePresence>
        </div>
      </DialogContent>
    </Dialog>
  )
}
