"use client"

import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { GitBranchIcon, File01Icon } from "@hugeicons/core-free-icons"

export function CreateSkillDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient()
  const [sourceType, setSourceType] = useState<"inline" | "git">("inline")
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [repoUrl, setRepoUrl] = useState("")
  const [repoSubpath, setRepoSubpath] = useState("")
  const [repoRef, setRepoRef] = useState("main")
  const createSkill = $api.useMutation("post", "/v1/skills")

  function reset() {
    setSourceType("inline")
    setName("")
    setDescription("")
    setRepoUrl("")
    setRepoSubpath("")
    setRepoRef("main")
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!name.trim()) return

    const body: Record<string, unknown> = {
      name: name.trim(),
      description: description.trim() || undefined,
      source_type: sourceType,
    }

    if (sourceType === "git") {
      if (!repoUrl.trim()) {
        toast.error("Repository URL is required for git skills")
        return
      }
      body.repo_url = repoUrl.trim()
      body.repo_subpath = repoSubpath.trim() || undefined
      body.repo_ref = repoRef.trim() || "main"
    } else {
      body.bundle = {
        id: name.trim().toLowerCase().replace(/\s+/g, "-"),
        title: name.trim(),
        description: description.trim(),
        content: "",
        references: [],
      }
    }

    createSkill.mutate(
      { body: body as never },
      {
        onSuccess: () => {
          toast.success("Skill created")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          handleOpenChange(false)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to create skill"))
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton className="sm:max-w-md max-h-[90dvh] overflow-y-auto">
        <DialogTitle>Create skill</DialogTitle>

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <Tabs
            value={sourceType}
            onValueChange={(value) => setSourceType(value as "inline" | "git")}
          >
            <TabsList className="w-full">
              <TabsTrigger value="inline">
                <HugeiconsIcon icon={File01Icon} size={14} />
                Inline
              </TabsTrigger>
              <TabsTrigger value="git">
                <HugeiconsIcon icon={GitBranchIcon} size={14} />
                Git
              </TabsTrigger>
            </TabsList>

            <TabsContent value="inline">
              <div className="flex flex-col gap-4 pt-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-name-inline">Name</Label>
                  <Input
                    id="skill-name-inline"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="e.g. Browser automation"
                    required
                    autoFocus
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-description-inline">Description</Label>
                  <Input
                    id="skill-description-inline"
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    placeholder="What this skill does..."
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent value="git">
              <div className="flex flex-col gap-4 pt-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-name-git">Name</Label>
                  <Input
                    id="skill-name-git"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="e.g. Browser automation"
                    required
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-description-git">Description</Label>
                  <Input
                    id="skill-description-git"
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    placeholder="What this skill does..."
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-repo-url">Repository URL</Label>
                  <Input
                    id="skill-repo-url"
                    value={repoUrl}
                    onChange={(event) => setRepoUrl(event.target.value)}
                    placeholder="https://github.com/org/repo"
                    required
                  />
                </div>
                <div className="flex gap-3">
                  <div className="flex flex-col gap-2 flex-1">
                    <Label htmlFor="skill-repo-subpath">Subpath</Label>
                    <Input
                      id="skill-repo-subpath"
                      value={repoSubpath}
                      onChange={(event) => setRepoSubpath(event.target.value)}
                      placeholder="skills/my-skill"
                    />
                  </div>
                  <div className="flex flex-col gap-2 w-24">
                    <Label htmlFor="skill-repo-ref">Ref</Label>
                    <Input
                      id="skill-repo-ref"
                      value={repoRef}
                      onChange={(event) => setRepoRef(event.target.value)}
                      placeholder="main"
                    />
                  </div>
                </div>
              </div>
            </TabsContent>
          </Tabs>

          <Button type="submit" className="w-full" loading={createSkill.isPending} disabled={!name.trim()}>
            Create skill
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}
