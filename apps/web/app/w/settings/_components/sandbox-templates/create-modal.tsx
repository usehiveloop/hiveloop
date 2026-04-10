"use client"

import * as React from "react"
import { useState, useEffect, useRef, useCallback } from "react"
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
import { ScrollArea } from "@/components/ui/scroll-area"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { toast } from "sonner"
import { useBuildStream } from "@/hooks/use-build-stream"
import type { components } from "@/lib/api/schema"

type SandboxTemplate = components["schemas"]["sandboxTemplateResponse"]

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
  const scrollRef = useRef<HTMLDivElement>(null)
  const onSuccessRef = useRef(onSuccess)
  const onOpenChangeRef = useRef(onOpenChange)

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
  }, [])

  const { connected, connecting, error, logs, status } = useBuildStream(
    isBuilding ? buildTemplateId : null
  )

  const createMutation = $api.useMutation("post", "/v1/sandbox-templates")
  const buildMutation = $api.useMutation("post", "/v1/sandbox-templates/{id}/build")

  useEffect(() => {
    if (status?.status === "ready") {
      toast.success("Sandbox template built successfully!")
      onSuccessRef.current?.({
        id: buildTemplateId!,
        name,
        build_commands: buildCommands,
        build_status: "ready",
        config: {},
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      })
      setTimeout(() => {
        onOpenChangeRef.current(false)
        resetForm()
      }, 1500)
    } else if (status?.status === "failed") {
      toast.error(`Build failed: ${status.message}`)
      setTimeout(() => setIsBuilding(false), 0)
    }
  }, [status, buildTemplateId, name, buildCommands, resetForm])

  useEffect(() => {
    if (logs.length > 0 && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [logs])

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
      const result = await createMutation.mutateAsync({
        body: {
          name: name.trim(),
          build_commands: buildCommands.trim(),
        },
      })

      const template = result as SandboxTemplate
      if (!template.id) {
        toast.error("Failed to get template ID")
        return
      }
      setBuildTemplateId(template.id)

      const buildResult = await buildMutation.mutateAsync({
        params: { path: { id: template.id } },
      })

      const buildResponse = buildResult as { stream_url?: string }
      if (!buildResponse.stream_url) {
        toast.error("Failed to get stream URL")
        return
      }

      setIsBuilding(true)
    } catch (err) {
      toast.error(extractErrorMessage(err, "Failed to create template"))
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
                  disabled={createMutation.isPending}
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
                  disabled={createMutation.isPending}
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
                  {getStatusBadge(status?.status)}
                </div>
                {connecting && (
                  <span className="text-xs text-muted-foreground">Connecting...</span>
                )}
                {connected && (
                  <span className="text-xs text-green-600">Streaming</span>
                )}
              </div>

              {error && (
                <div className="rounded-md bg-red-500/10 border border-red-500/20 p-3">
                  <p className="text-sm text-red-600">{error}</p>
                </div>
              )}

              <ScrollArea className="h-[300px] rounded-md bg-muted/50 p-4">
                <div className="space-y-1 font-mono text-xs">
                  {logs.map((log) => (
                    <div key={log.line} className="text-foreground/80">
                      {log.line}
                    </div>
                  ))}
                  {logs.length === 0 && !error && (
                    <div className="text-muted-foreground">Waiting for logs...</div>
                  )}
                </div>
              </ScrollArea>

              {status?.status === "failed" && status.message && (
                <div className="rounded-md bg-red-500/10 border border-red-500/20 p-3">
                  <p className="text-sm font-medium text-red-600">Build Error:</p>
                  <p className="text-xs text-red-600/80 mt-1">{status.message}</p>
                </div>
              )}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t">
          {!isBuilding ? (
            <>
              <Button variant="outline" onClick={handleClose} disabled={createMutation.isPending}>
                Cancel
              </Button>
              <Button
                onClick={handleCreateAndBuild}
                loading={createMutation.isPending || buildMutation.isPending}
              >
                Create & Build
              </Button>
            </>
          ) : (
            <Button
              variant="outline"
              onClick={handleClose}
              disabled={status?.status === "building" && connected}
            >
              Close
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
