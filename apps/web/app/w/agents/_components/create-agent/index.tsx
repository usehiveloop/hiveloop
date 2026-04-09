"use client"

import { useState, useCallback } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Dialog, DialogContent, DialogTrigger } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon } from "@hugeicons/core-free-icons"
import { CreateAgentProvider, useCreateAgent } from "./context"
import { StepChooseMode } from "./step-choose-mode"
import { StepSandboxType } from "./step-sandbox-type"
import { StepIntegrations } from "./step-integrations"
import { StepLlmKey } from "./step-llm-key"
import { StepBasics } from "./step-basics"
import { StepSystemPrompt } from "./step-system-prompt"
import { StepInstructions } from "./step-instructions"
import { StepForgeJudge } from "./step-forge-judge"
import { StepSummary } from "./step-summary"
import { StepTrigger } from "./step-trigger"
import { StepMarketplaceBrowse, StepMarketplaceDetail } from "./step-marketplace"
import type { CreationMode } from "./types"

function StepRouter() {
  const { step, direction } = useCreateAgent()

  const variants = {
    enter: (dir: number) => ({ x: dir > 0 ? 80 : -80, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (dir: number) => ({ x: dir > 0 ? -80 : 80, opacity: 0 }),
  }

  return (
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
          {step === "mode" && <StepChooseMode />}
          {step === "marketplace-browse" && <StepMarketplaceBrowse />}
          {step === "marketplace-detail" && <StepMarketplaceDetail />}
          {step === "sandbox" && <StepSandboxType />}
          {step === "integrations" && <StepIntegrations />}
          {step === "trigger" && <StepTrigger />}
          {step === "llm-key" && <StepLlmKey />}
          {step === "basics" && <StepBasics />}
          {step === "system-prompt" && <StepSystemPrompt />}
          {step === "instructions" && <StepInstructions />}
          {step === "forge-judge" && <StepForgeJudge />}
          {step === "summary" && <StepSummary />}
        </motion.div>
      </AnimatePresence>
    </div>
  )
}

interface CreateAgentDialogProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  initialMode?: CreationMode
}

export function CreateAgentDialog({ open: controlledOpen, onOpenChange, initialMode }: CreateAgentDialogProps) {
  const [internalOpen, setInternalOpen] = useState(false)

  const isControlled = controlledOpen !== undefined
  const open = isControlled ? controlledOpen : internalOpen

  const handleOpenChange = useCallback((nextOpen: boolean) => {
    if (isControlled) {
      onOpenChange?.(nextOpen)
    } else {
      setInternalOpen(nextOpen)
    }
  }, [isControlled, onOpenChange])

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {!isControlled && (
        <DialogTrigger
          render={
            <Button size="default">
              <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
              Create agent
            </Button>
          }
        />
      )}
      <DialogContent className="sm:max-w-md h-[780px] overflow-hidden flex flex-col">
        <CreateAgentProvider onClose={() => handleOpenChange(false)} initialMode={initialMode}>
          <StepRouter />
        </CreateAgentProvider>
      </DialogContent>
    </Dialog>
  )
}
