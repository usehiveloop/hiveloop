"use client"

import { useState, useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { $api } from "@/lib/api/hooks"
import { RecipeEditor } from "../recipe-editor"
import { recipeToYaml, parseRecipeYaml } from "./yaml-parser"
import { recipeTemplates, getBaseProvider } from "./templates"
import type { RecipeTemplate } from "./templates"
import type { ContextActionConfig, TriggerConditionsConfig } from "./types"

interface ContextConfigViewProps {
  provider: string
  triggerDisplayNames: string[]
  triggerKeys: string[]
  refs: Record<string, string>
  contextActions: ContextActionConfig[]
  conditions?: TriggerConditionsConfig
  prompt: string
  onConfirm: (actions: ContextActionConfig[], conditions: TriggerConditionsConfig | undefined, prompt: string) => void
  onBack: () => void
}

export function ContextConfigView({
  provider,
  triggerDisplayNames,
  triggerKeys,
  refs,
  contextActions,
  conditions,
  prompt,
  onConfirm,
  onBack,
}: ContextConfigViewProps) {
  const [yamlText, setYamlText] = useState(() => recipeToYaml(contextActions, conditions, prompt, triggerKeys, refs))
  const [parseError, setParseError] = useState<string | null>(null)
  const [showTemplates, setShowTemplates] = useState(false)

  const templates = recipeTemplates[getBaseProvider(provider)] ?? []

  function applyTemplate(template: RecipeTemplate) {
    setYamlText(template.yaml)
    setShowTemplates(false)
    setParseError(null)
  }

  const { data: schemaPathsData } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/schema-paths",
    { params: { path: { id: provider } } },
    { enabled: !!provider },
  )

  const { data: readActionsData } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/actions",
    { params: { path: { id: provider }, query: { access: "read" } } },
    { enabled: !!provider },
  )

  const refNames = useMemo(() => Object.keys(refs), [refs])

  const actionPaths = useMemo(() => {
    const rawActions = (schemaPathsData)?.actions ?? {}
    const result: Record<string, Array<{ path: string; type: string }>> = {}
    for (const [actionKey, actionData] of Object.entries(rawActions)) {
      const paths = (actionData)?.paths as Array<{ path: string; type: string }> | undefined
      if (paths) result[actionKey] = paths
    }
    return result
  }, [schemaPathsData])

  const actionKeysForEditor = useMemo(() => {
    if (!readActionsData || !Array.isArray(readActionsData)) return []
    return readActionsData.map((action) => ({
      key: action.key ?? "",
      displayName: action.display_name ?? "",
      access: action.access ?? "",
      resourceType: action.resource_type ?? "",
    }))
  }, [readActionsData])

  function handleConfirm() {
    const parsed = parseRecipeYaml(yamlText)
    if (!parsed) {
      setParseError("Could not parse YAML. Check your formatting.")
      return
    }
    setParseError(null)
    onConfirm(parsed.context, parsed.conditions, parsed.prompt)
  }

  const refsList = refNames.map((refName) => `$refs.${refName}`).join(", ")

  return (
    <>
      <DialogHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
              <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
            </button>
            <DialogTitle>Trigger recipe</DialogTitle>
          </div>
          {templates.length > 0 && (
            <button
              type="button"
              onClick={() => setShowTemplates(true)}
              className="text-[11px] font-medium text-primary hover:text-primary/80 transition-colors px-2 py-1 rounded-md hover:bg-primary/5"
            >
              Templates
            </button>
          )}
        </div>
        <DialogDescription className="mt-2">
          Define the context to gather and the prompt for your agent.
        </DialogDescription>
      </DialogHeader>

      {/* Template picker modal */}
      <Dialog open={showTemplates} onOpenChange={setShowTemplates}>
        <DialogContent showCloseButton>
          <DialogHeader>
            <DialogTitle>Recipe templates</DialogTitle>
            <DialogDescription>Choose a starter recipe to pre-fill the editor.</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            {templates.map((template) => (
              <button
                key={template.name}
                type="button"
                onClick={() => applyTemplate(template)}
                className="flex flex-col gap-0.5 w-full rounded-xl bg-muted/50 hover:bg-muted p-3 text-left transition-colors cursor-pointer border border-transparent hover:border-primary/20"
              >
                <p className="text-sm font-medium text-foreground">{template.name}</p>
                <p className="text-[12px] text-muted-foreground">{template.description}</p>
              </button>
            ))}
          </div>
        </DialogContent>
      </Dialog>

      <div className="flex flex-col gap-3 mt-4 flex-1 min-h-0">
        <RecipeEditor
          value={yamlText}
          onChange={(newValue) => { setYamlText(newValue); setParseError(null) }}
          refNames={refNames}
          actionPaths={actionPaths}
          actionKeys={actionKeysForEditor}
        />

        {parseError && (
          <p className="text-xs text-destructive px-1">{parseError}</p>
        )}

        <div className="rounded-lg bg-muted/50 px-3 py-2 shrink-0">
          <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider mb-1">Available refs</p>
          <p className="text-[11px] font-mono text-muted-foreground leading-relaxed break-all">{refsList}</p>
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={handleConfirm} className="w-full">
          Confirm trigger
        </Button>
      </div>
    </>
  )
}
