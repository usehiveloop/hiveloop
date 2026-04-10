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
import { Textarea } from "@/components/ui/textarea"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { toast } from "sonner"
import {
  useSandboxTemplate,
  useTriggerBuild,
  createSandboxTemplate,
  type SandboxTemplate,
} from "@/hooks/use-sandbox-template"

interface CreateSandboxTemplateModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: (template: SandboxTemplate) => void
}

export function CreateSandboxTemplateModal({ open, onOpenChange, onSuccess }: CreateSandboxTemplateModalProps) {
  const [name, setName] = useState("")
  const [buildCommands, setBuildCommands] = useState("")
  const [isBuilding, setIsBuilding] = useState(false)
  const [buildTemplateId, setBuildTemplateId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
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
    setBuildCommands("")
    setIsBuilding(false)
    setBuildTemplateId(null)
    setError(null)
    hasShownSuccessRef.current = false
  }, [])

  const { data: template, isLoading } = useSandboxTemplate(
    isBuilding ? buildTemplateId : null
  )

  const triggerBuild = useTriggerBuild()

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
    } else if (template.build_status === "failed") {
      hasShownSuccessRef.current = true
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setError(template.build_error ?? "Build failed with unknown error")
    }
  }, [template, isBuilding, resetForm])

  function handleClose() {
    onOpenChange(false)
    resetForm()
  }

  async function handleCreateAndBuild() {
    if (!name.trim()) {
      toast.error("Name is required")
      return
    }
    if (!buildCommands.trim()) {
      toast.error("Build commands are required")
      return
    }

    try {
      const createdTemplate = await createSandboxTemplate({
        name: name.trim(),
        build_commands: buildCommands.trim(),
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
            <div className="space-y-4 py-4">
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
                <Label htmlFor="commands">Build Commands</Label>
                <Textarea
                  id="commands"
                  placeholder={"pip install numpy pandas\nnpm install -g my-cli"}
                  value={buildCommands}
                  onChange={(e) => setBuildCommands(e.target.value)}
                  className="font-mono text-sm min-h-[200px]"
                />
                <p className="text-xs text-muted-foreground">
                  One command per line. Each line runs in a separate shell layer.
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

              {error && (
                <div className="rounded-md bg-red-500/10 border border-red-500/20 p-3">
                  <p className="text-sm text-red-600">{error}</p>
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

              {template?.build_status === "failed" && template.build_error && (
                <div className="rounded-md bg-red-500/10 border border-red-500/20 p-3">
                  <p className="text-sm font-medium text-red-600">Build Error:</p>
                  <p className="text-xs text-red-600/80 mt-1">{template.build_error}</p>
                </div>
              )}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t">
          {!isBuilding ? (
            <>
              <Button variant="outline" onClick={handleClose}>
                Cancel
              </Button>
              <Button
                onClick={handleCreateAndBuild}
                loading={triggerBuild.isPending}
              >
                Create & Build
              </Button>
            </>
          ) : (
            <Button
              variant="outline"
              onClick={handleClose}
              disabled={template?.build_status === "building"}
            >
              Close
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
