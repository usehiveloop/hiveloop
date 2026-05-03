"use client"

import { Button } from "@/components/ui/button"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { useCreateAgent } from "./context"
import type { TriggerConfig } from "./types"
import { Tick02Icon } from "@hugeicons/core-free-icons"
import { HugeiconsIcon } from "@hugeicons/react"

interface ShortcutDef {
  label: string
  triggerKeys: string[]
  triggerDisplayNames: string[]
}

const GITHUB_APP_SHORTCUTS: ShortcutDef[] = [
  {
    label: "Pull request opened",
    triggerKeys: ["pull_request.opened"],
    triggerDisplayNames: ["Pull request opened"],
  },
  {
    label: "Issue opened",
    triggerKeys: ["issues.opened"],
    triggerDisplayNames: ["Issue opened"],
  },
  {
    label: "Issue commented",
    triggerKeys: ["issue_comment.created"],
    triggerDisplayNames: ["Issue comment created"],
  },
]

const GITHUB_APP_CODE_REVIEWS_SHORTCUTS: ShortcutDef[] = [
  {
    label: "Pull request opened",
    triggerKeys: ["pull_request.opened"],
    triggerDisplayNames: ["Pull request opened"],
  },
  {
    label: "Pull request review requested",
    triggerKeys: ["pull_request.review_requested"],
    triggerDisplayNames: ["Pull request review requested"],
  },
]

function isShortcutAlreadyAdded(
  triggers: TriggerConfig[],
  connectionId: string,
  shortcut: ShortcutDef,
): boolean {
  return triggers.some(
    (t) =>
      t.connectionId === connectionId &&
      t.triggerKeys.length === shortcut.triggerKeys.length &&
      shortcut.triggerKeys.every((k) => t.triggerKeys.includes(k)),
  )
}

export function GitHubTriggerShortcuts() {
  const { selectedIntegrations, triggers, addTrigger } = useCreateAgent()
  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []
  const connectionsById = new Map(
    connections.filter((c): c is typeof c & { id: string } => !!c.id).map((c) => [c.id, c]),
  )

  let githubAppConnectionId: string | null = null
  let githubAppCodeReviewsConnectionId: string | null = null

  for (const id of selectedIntegrations) {
    const conn = connectionsById.get(id)
    if (!conn) continue
    if (conn.provider === "github-app" && !githubAppConnectionId) {
      githubAppConnectionId = id
    }
    if (conn.provider === "github-app-code-reviews" && !githubAppCodeReviewsConnectionId) {
      githubAppCodeReviewsConnectionId = id
    }
  }

  if (!githubAppConnectionId && !githubAppCodeReviewsConnectionId) {
    return null
  }

  function handleShortcutClick(connectionId: string, shortcut: ShortcutDef) {
    const conn = connectionsById.get(connectionId)
    if (!conn || !conn.provider) return
    if (isShortcutAlreadyAdded(triggers, connectionId, shortcut)) return

    addTrigger({
      triggerType: "webhook",
      connectionId,
      connectionName: conn.provider,
      provider: conn.provider,
      triggerKeys: shortcut.triggerKeys,
      triggerDisplayNames: shortcut.triggerDisplayNames,
      conditions: null,
    })
  }

  function renderShortcuts(connectionId: string, shortcuts: ShortcutDef[]) {
    const conn = connectionsById.get(connectionId)
    if (!conn || !conn.provider) return null

    return (
      <div className="flex flex-wrap gap-2">
        {shortcuts.map((shortcut) => {
          const added = isShortcutAlreadyAdded(triggers, connectionId, shortcut)
          return (
            <Button
              key={shortcut.label}
              variant="outline"
              size="sm"
              disabled={added}
              onClick={() => handleShortcutClick(connectionId, shortcut)}
              className="flex items-center gap-2"
            >
              <IntegrationLogo provider={conn.provider!} size={16} />
              <span className="text-xs">{shortcut.label}</span>
              {added && (
                <HugeiconsIcon icon={Tick02Icon} size={14} className="text-emerald-500" />
              )}
            </Button>
          )
        })}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {githubAppConnectionId && renderShortcuts(githubAppConnectionId, GITHUB_APP_SHORTCUTS)}
      {githubAppCodeReviewsConnectionId &&
        renderShortcuts(githubAppCodeReviewsConnectionId, GITHUB_APP_CODE_REVIEWS_SHORTCUTS)}
    </div>
  )
}
