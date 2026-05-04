"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon, Refresh01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { WhatsappLogo } from "./whatsapp-logo"
import { StepHeader } from "./step-header"
import { useOnboarding } from "./context"

export function ConfigureWhatsappStep() {
  const { goBack, goNext } = useOnboarding()

  return (
    <div className="mx-auto flex w-full max-w-md flex-col gap-8">
      <StepHeader
        hero={<WhatsappLogo size={48} />}
        title="Connect WhatsApp"
        description="Scan the QR code with the WhatsApp app on the phone you want your AI employee to use."
      />

      <div className="flex flex-col items-center gap-6">
        <div className="w-full rounded-2xl border border-border bg-background p-5">
          <PlaceholderQR className="aspect-square w-full" />
        </div>

        <ol className="flex w-full flex-col gap-4 text-sm">
          <Instruction n={1}>Open WhatsApp on your phone.</Instruction>
          <Instruction n={2}>
            Tap <span className="font-medium text-foreground">Settings → Linked Devices → Link a Device</span>.
          </Instruction>
          <Instruction n={3}>Point your camera at the QR code.</Instruction>
          <Instruction n={4}>Wait a few seconds for the link to confirm.</Instruction>
        </ol>
      </div>

      <div className="flex items-center justify-center">
        <Button variant="ghost" size="sm" className="gap-2 text-muted-foreground">
          <HugeiconsIcon icon={Refresh01Icon} className="size-4" />
          Refresh QR code
        </Button>
      </div>

      <div className="flex items-center justify-between">
        <Button variant="ghost" onClick={goBack} className="gap-2">
          <HugeiconsIcon icon={ArrowLeft01Icon} className="size-4" />
          Back
        </Button>
        <Button onClick={goNext} className="gap-2">
          I&apos;ve scanned it
          <HugeiconsIcon icon={ArrowRight01Icon} className="size-4" />
        </Button>
      </div>
    </div>
  )
}

function Instruction({ n, children }: { n: number; children: React.ReactNode }) {
  return (
    <li className="flex gap-3">
      <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
        {n}
      </span>
      <span className="pt-0.5 text-muted-foreground">{children}</span>
    </li>
  )
}

function PlaceholderQR({ className }: { className?: string }) {
  const cells = 21
  const filled = (x: number, y: number) => ((x * 31 + y * 17 + x * y) % 5) < 2

  return (
    <svg viewBox={`0 0 ${cells} ${cells}`} className={className} aria-label="WhatsApp pairing QR placeholder">
      <rect width={cells} height={cells} fill="white" />
      {Array.from({ length: cells }).flatMap((_, y) =>
        Array.from({ length: cells }).map((_, x) =>
          filled(x, y) ? <rect key={`${x}-${y}`} x={x} y={y} width={1} height={1} fill="black" /> : null,
        ),
      )}
      {[
        [0, 0],
        [cells - 7, 0],
        [0, cells - 7],
      ].map(([cx, cy], i) => (
        <g key={i}>
          <rect x={cx} y={cy} width={7} height={7} fill="black" />
          <rect x={cx + 1} y={cy + 1} width={5} height={5} fill="white" />
          <rect x={cx + 2} y={cy + 2} width={3} height={3} fill="black" />
        </g>
      ))}
    </svg>
  )
}
