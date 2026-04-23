"use client"

import { useState, useRef } from "react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon, Delete02Icon, FileEditIcon } from "@hugeicons/core-free-icons"

interface EnvVar {
  key: string
  value: string
}

interface EnvVarsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  agentName: string
}

function parseEnvPaste(text: string): EnvVar[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0 && !line.startsWith("#"))
    .map((line) => {
      const separatorIndex = line.indexOf("=")
      if (separatorIndex === -1) return null
      const key = line.slice(0, separatorIndex).trim()
      const rawValue = line.slice(separatorIndex + 1).trim()
      const value = rawValue.replace(/^["']|["']$/g, "")
      return key ? { key, value } : null
    })
    .filter((entry): entry is EnvVar => entry !== null)
}

export function EnvVarsDialog({ open, onOpenChange, agentName }: EnvVarsDialogProps) {
  const [envVars, setEnvVars] = useState<EnvVar[]>([{ key: "", value: "" }])
  const [pasteMode, setPasteMode] = useState(false)
  const [pasteText, setPasteText] = useState("")
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  function addRow() {
    setEnvVars([...envVars, { key: "", value: "" }])
  }

  function removeRow(index: number) {
    setEnvVars(envVars.filter((_, idx) => idx !== index))
  }

  function updateRow(index: number, field: "key" | "value", newValue: string) {
    setEnvVars(envVars.map((row, idx) => (idx === index ? { ...row, [field]: newValue } : row)))
  }

  function applyPaste() {
    const parsed = parseEnvPaste(pasteText)
    if (parsed.length === 0) return

    const hasOnlyEmptyRow = envVars.length === 1 && !envVars[0].key && !envVars[0].value
    setEnvVars(hasOnlyEmptyRow ? parsed : [...envVars, ...parsed])
    setPasteText("")
    setPasteMode(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Environment variables</DialogTitle>
          <DialogDescription>
            Configure environment variables for <strong>{agentName}</strong>. These are injected into the sandbox at creation time.
          </DialogDescription>
        </DialogHeader>

        {pasteMode ? (
          <div className="flex flex-col gap-3">
            <Textarea
              ref={textareaRef}
              placeholder={"DATABASE_URL=postgres://localhost:5432/mydb\nAPI_KEY=sk-abc123\nNODE_ENV=production"}
              value={pasteText}
              onChange={(event) => setPasteText(event.target.value)}
              className="min-h-32 font-mono text-xs"
              autoFocus
            />
            <p className="text-mini text-muted-foreground">
              Paste KEY=VALUE pairs, one per line. Lines starting with # are ignored.
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="outline" size="sm" onClick={() => { setPasteMode(false); setPasteText("") }}>
                Cancel
              </Button>
              <Button size="sm" onClick={applyPaste} disabled={!pasteText.trim()}>
                Apply
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2 px-1 text-2xs font-mono uppercase tracking-small text-muted-foreground/50">
              <span className="flex-1">Key</span>
              <span className="flex-1">Value</span>
              <span className="w-8 shrink-0" />
            </div>

            {envVars.map((envVar, index) => (
              <div key={index} className="flex items-center gap-2">
                <Input
                  placeholder="API_KEY"
                  value={envVar.key}
                  onChange={(event) => updateRow(index, "key", event.target.value)}
                  className="flex-1 font-mono text-xs"
                />
                <Input
                  placeholder="value"
                  type="password"
                  value={envVar.value}
                  onChange={(event) => updateRow(index, "value", event.target.value)}
                  className="flex-1 font-mono text-xs"
                />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => removeRow(index)}
                  disabled={envVars.length === 1}
                >
                  <HugeiconsIcon icon={Delete02Icon} size={14} className="text-muted-foreground" />
                </Button>
              </div>
            ))}

            <div className="flex items-center gap-1">
              <Button variant="ghost" size="sm" onClick={addRow}>
                <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
                Add variable
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setPasteMode(true)}>
                <HugeiconsIcon icon={FileEditIcon} size={14} data-icon="inline-start" />
                Paste .env
              </Button>
            </div>
          </div>
        )}

        {!pasteMode && (
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button>Save variables</Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}
