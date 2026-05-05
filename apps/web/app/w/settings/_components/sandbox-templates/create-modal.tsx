"use client"

import * as React from "react"
import { useState, useEffect, useCallback } from "react"
import ScrollToBottom from "react-scroll-to-bottom"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { ChoiceCard } from "@/app/w/agents/_components/create-agent/choice-card"
import { HugeiconsIcon } from "@hugeicons/react"
import { Tick02Icon } from "@hugeicons/core-free-icons"
import { toast } from "sonner"
import {
  useSandboxTemplate,
  useTriggerBuild,
  useRetryBuild,
  useCreateSandboxTemplate,
  type SandboxTemplate,
} from "@/hooks/use-sandbox-template"
import { BuildCommandsEditor } from "./build-commands-editor"

const SIZE_PRESETS = {
  small: { label: "Small", vcpu: 1, memory_gb: 2, disk_gb: 10 },
  medium: { label: "Medium", vcpu: 2, memory_gb: 4, disk_gb: 20 },
  large: { label: "Large", vcpu: 4, memory_gb: 8, disk_gb: 40 },
  xlarge: { label: "XLarge", vcpu: 8, memory_gb: 16, disk_gb: 80 },
} as const

type SizeName = keyof typeof SIZE_PRESETS

const SIZE_OPTIONS = Object.entries(SIZE_PRESETS) as [SizeName, (typeof SIZE_PRESETS)[SizeName]][]

const DEFAULT_COMMANDS_PLACEHOLDER = "# One command per line. Lines run with && between them.\napt-get install -y python3 python3-pip\npip3 install requests"

interface CreateSandboxTemplateModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: (template: SandboxTemplate) => void
}

export function CreateSandboxTemplateModal({ open, onOpenChange, onSuccess }: CreateSandboxTemplateModalProps) {
  const [name, setName] = useState("")
  const [buildCommandsText, setBuildCommandsText] = useState("")
  const [size, setSize] = useState<SizeName>("small")
  const [isBuilding, setIsBuilding] = useState(false)
  const [isRetrying, setIsRetrying] = useState(false)
  const [buildTemplateId, setBuildTemplateId] = useState<string | null>(null)

  const onSuccessRef = React.useRef(onSuccess)
  const onOpenChangeRef = React.useRef(onOpenChange)
  const hasShownSuccessRef = React.useRef(false)

  useEffect(() => {
    onSuccessRef.current = onSuccess
  }, [onSuccess])

  useEffect(() => {
    onOpenChangeRef.current = onOpenChange
  }, [onOpenChange])

  const resetForm = useCallback(() => {
    setName("")
    setBuildCommandsText("")
    setSize("small")
    setIsBuilding(false)
    setIsRetrying(false)
    setBuildTemplateId(null)
    hasShownSuccessRef.current = false
  }, [])

  const { data: template, isLoading } = useSandboxTemplate(
    isBuilding ? buildTemplateId : null
  )

  const triggerBuild = useTriggerBuild()
  const retryBuild = useRetryBuild()
  const createTemplate = useCreateSandboxTemplate()

  useEffect(() => {
    if (!template || !isBuilding || hasShownSuccessRef.current) return

    if (template.build_status === "ready") {
      hasShownSuccessRef.current = true
      toast.success("Sandbox template built successfully!")
      onSuccessRef.current?.(template)
      setTimeout(() => {
        onOpenChangeRef.current(false)
        resetForm()
      }, 1500)
    }
  }, [template, isBuilding, resetForm])

  function handleClose() {
    onOpenChange(false)
    resetForm()
  }

  // Lines that aren't blank and aren't comments. Each line is a separate
  // command joined with && server-side, so '#' lines are tossed locally.
  const filteredCommands = buildCommandsText
    .split("\n")
    .map((line) => line.replace(/\s+$/, ""))
    .filter((line) => line.trim() !== "" && !line.trim().startsWith("#"))

  async function handleCreateAndBuild() {
    if (!name.trim()) {
      toast.error("Name is required")
      return
    }
    if (filteredCommands.length === 0) {
      toast.error("At least one build command is required")
      return
    }

    try {
      const preset = SIZE_PRESETS[size]
      const createdTemplate = await createTemplate.mutateAsync({
        body: {
          name: name.trim(),
          build_commands: filteredCommands,
          vcpu: preset.vcpu,
          memory_gb: preset.memory_gb,
          disk_gb: preset.disk_gb,
        },
      })

      if (!createdTemplate.id) {
        toast.error("Failed to get template ID")
        return
      }

      triggerBuild.mutate(
        { params: { path: { id: createdTemplate.id } } },
        {
          onError: (err: unknown) => {
            toast.error(`Failed to trigger build: ${err}`)
          },
        }
      )

      setBuildTemplateId(createdTemplate.id)
      setIsBuilding(true)
    } catch (err) {
      toast.error("Failed to create template")
    }
  }

  function getStatusBadge(buildStatus?: string) {
    switch (buildStatus) {
      case "ready":
        return <Badge variant="default" className="bg-green-500/10 text-green-600 border-green-500/20">Ready</Badge>
      case "building":
        return <Badge variant="default" className="bg-blue-500/10 text-blue-600 border-blue-500/20">Building</Badge>
      case "failed":
        return <Badge variant="default" className="bg-red-500/10 text-red-600 border-red-500/20">Failed</Badge>
      default:
        return <Badge variant="secondary">Pending</Badge>
    }
  }

  function handleRetry() {
    if (!buildTemplateId) return
    const cmds = template?.build_commands ?? []
    setBuildCommandsText(cmds.join("\n"))
    setIsRetrying(true)
    setIsBuilding(false)
  }

  async function handleRetryBuild() {
    if (!buildTemplateId || retryBuild.isPending) return

    retryBuild.mutate(
      {
        params: { path: { id: buildTemplateId } },
        body: { build_commands: filteredCommands },
      },
      {
        onSuccess: () => {
          setIsRetrying(false)
          setIsBuilding(true)
        },
        onError: (err: unknown) => {
          toast.error(`Failed to retry build: ${err}`)
          setIsRetrying(false)
        },
      }
    )
  }

  const logs = template?.build_logs?.split("\n").filter(Boolean) ?? []

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-2xl max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>
            {isBuilding ? "Building Sandbox Template" : "Create Sandbox Template"}
          </DialogTitle>
          <DialogDescription>
            {isBuilding
              ? "Your template is being built. Watch the logs below."
              : "Create a custom sandbox template with your own build commands."}
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 overflow-hidden">
          {!isBuilding ? (
            <div className="space-y-4 py-4 max-h-[50vh] overflow-y-auto">
              {!isRetrying && (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="name">Template Name</Label>
                    <Input
                      id="name"
                      placeholder="my-custom-template"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">
                      A descriptive name for your template.
                    </p>
                  </div>
                  <div className="space-y-2">
                    <Label>Size</Label>
                    <div role="radiogroup" aria-label="Sandbox size" className="grid grid-cols-2 gap-2">
                      {SIZE_OPTIONS.map(([key, preset]) => {
                        const selected = size === key
                        return (
                          <ChoiceCard
                            key={key}
                            title={preset.label}
                            description={`${preset.vcpu} vCPU · ${preset.memory_gb} GB RAM · ${preset.disk_gb} GB disk`}
                            onClick={() => setSize(key)}
                            trailing={
                              selected ? (
                                <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-green-500">
                                  <HugeiconsIcon
                                    icon={Tick02Icon}
                                    size={10}
                                    strokeWidth={3}
                                    className="text-white"
                                  />
                                </span>
                              ) : (
                                <span className="h-4 w-4 shrink-0 rounded-full border border-muted-foreground/30" />
                              )
                            }
                          />
                        )
                      })}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      Resource allocation for sandboxes built from this template.
                    </p>
                  </div>
                </>
              )}

              <div className="space-y-2">
                <Label htmlFor="build-commands">Build Commands</Label>
                <BuildCommandsEditor
                  value={buildCommandsText}
                  onChange={setBuildCommandsText}
                  placeholder={DEFAULT_COMMANDS_PLACEHOLDER}
                  height="240px"
                />
                <p className="text-xs text-muted-foreground">
                  One command per line. Lines are joined with &&. Comments (#) and blanks are ignored.
                </p>
              </div>
            </div>
          ) : (
            <div className="space-y-4 py-4 h-full">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{name}</span>
                  {getStatusBadge(template?.build_status)}
                </div>
                {isLoading && (
                  <span className="text-xs text-muted-foreground">Loading...</span>
                )}
              </div>

              {template?.build_status === "failed" && template.build_error && (
                <div className="rounded-md bg-red-500/10 border border-red-500/20 p-3">
                  <p className="text-sm font-medium text-red-600">Build Error:</p>
                  <p className="text-xs text-red-600/80 mt-1">{template.build_error}</p>
                </div>
              )}

              <ScrollToBottom className="h-[300px] rounded-md bg-black border">
                <SyntaxHighlighter
                  language="bash"
                  style={oneDark}
                  customStyle={{
                    margin: 0,
                    padding: "1rem",
                    background: "transparent",
                    fontSize: "0.75rem",
                  }}
                  showLineNumbers
                >
                  {logs.length > 0 ? logs.join("\n") : "# Waiting for logs...\n"}
                </SyntaxHighlighter>
              </ScrollToBottom>
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t">
          {!isBuilding ? (
            <>
              {isRetrying ? (
                <>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setIsRetrying(false)
                      setBuildCommandsText("")
                    }}
                  >
                    Cancel
                  </Button>
                  <Button
                    onClick={handleRetryBuild}
                    loading={retryBuild.isPending}
                    disabled={retryBuild.isPending}
                  >
                    Retry Build
                  </Button>
                </>
              ) : (
                <>
                  <Button variant="outline" onClick={handleClose}>
                    Cancel
                  </Button>
                  <Button
                    onClick={handleCreateAndBuild}
                    loading={createTemplate.isPending || triggerBuild.isPending}
                    disabled={createTemplate.isPending || triggerBuild.isPending}
                  >
                    Build
                  </Button>
                </>
              )}
            </>
          ) : (
            <>
              {(template?.build_status === "failed" || template?.build_status === "ready") && (
                <Button
                  variant="outline"
                  onClick={handleRetry}
                >
                  Retry
                </Button>
              )}
              <Button
                variant="outline"
                onClick={handleClose}
                disabled={template?.build_status === "building"}
              >
                Close
              </Button>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
