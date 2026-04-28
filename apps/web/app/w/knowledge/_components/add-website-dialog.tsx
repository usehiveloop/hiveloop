"use client"

import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
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
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"

interface AddWebsiteDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const DEFAULT_MAX_PAGES = 500
const PAGE_PRICE_CREDITS = 1

export function AddWebsiteDialog({ open, onOpenChange }: AddWebsiteDialogProps) {
  const [name, setName] = useState("")
  const [url, setUrl] = useState("")
  const [maxPages, setMaxPages] = useState<number>(DEFAULT_MAX_PAGES)
  const [respectRobots, setRespectRobots] = useState(true)
  const queryClient = useQueryClient()
  const createSource = $api.useMutation("post", "/v1/rag/sources")

  useEffect(() => {
    if (!open) {
      setName("")
      setUrl("")
      setMaxPages(DEFAULT_MAX_PAGES)
      setRespectRobots(true)
    }
  }, [open])

  const submit = async () => {
    const trimmedURL = url.trim()
    const trimmedName = name.trim() || trimmedURL
    if (!trimmedURL) {
      toast.error("Enter a URL")
      return
    }
    if (!Number.isFinite(maxPages) || maxPages <= 0) {
      toast.error("Max pages must be a positive number")
      return
    }
    try {
      await createSource.mutateAsync({
        body: {
          kind: "WEBSITE",
          name: trimmedName,
          access_type: "PUBLIC",
          config: {
            url: trimmedURL,
            max_pages: maxPages,
            respect_robots: respectRobots,
          },
        },
      })
      toast.success("Website added — crawl starting")
      await queryClient.invalidateQueries({ queryKey: ["get", "/v1/rag/sources"] })
      onOpenChange(false)
    } catch (err) {
      toast.error(extractErrorMessage(err, "Failed to add website"))
    }
  }

  const estimatedCredits = Math.max(0, Math.floor(maxPages)) * PAGE_PRICE_CREDITS

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add a website</DialogTitle>
          <DialogDescription>
            Crawls the URL and indexes each page as markdown. The crawl follows links from the seed URL and includes sitemap entries.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="website-name">Name</Label>
            <Input
              id="website-name"
              placeholder="Acme docs"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="website-url">URL</Label>
            <Input
              id="website-url"
              placeholder="https://docs.acme.com"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              autoFocus
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="website-max-pages">Max pages</Label>
            <Input
              id="website-max-pages"
              type="number"
              min={1}
              value={maxPages}
              onChange={(e) => setMaxPages(parseInt(e.target.value, 10) || 0)}
            />
            <p className="text-xs text-muted-foreground">
              Hard cap on pages crawled. Costs up to{" "}
              <span className="font-medium text-foreground">
                {estimatedCredits.toLocaleString()} credits
              </span>{" "}
              ({PAGE_PRICE_CREDITS} per page).
            </p>
          </div>

          <div className="flex items-center justify-between">
            <div>
              <Label htmlFor="website-robots">Respect robots.txt</Label>
              <p className="text-xs text-muted-foreground">
                Skip URLs disallowed by the site.
              </p>
            </div>
            <Switch
              id="website-robots"
              checked={respectRobots}
              onCheckedChange={setRespectRobots}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={createSource.isPending}
          >
            Cancel
          </Button>
          <Button onClick={submit} disabled={createSource.isPending}>
            {createSource.isPending ? "Starting crawl…" : "Start crawl"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
