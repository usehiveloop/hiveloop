import type { ContextActionConfig, TriggerConditionConfig, TriggerConditionsConfig } from "./types"

export interface ParsedRecipe {
  conditions?: TriggerConditionsConfig
  context: ContextActionConfig[]
  prompt: string
}

export function recipeToYaml(
  actions: ContextActionConfig[],
  conditions: TriggerConditionsConfig | undefined,
  prompt: string,
  triggerKeys: string[],
  refs: Record<string, string>,
): string {
  const lines: string[] = []

  if (conditions && conditions.rules.length > 0) {
    lines.push("conditions:")
    lines.push(`  match: ${conditions.match}`)
    lines.push("  rules:")
    for (const rule of conditions.rules) {
      lines.push(`    - path: ${rule.path}`)
      lines.push(`      operator: ${rule.operator}`)
      if (Array.isArray(rule.value)) {
        lines.push(`      value: [${rule.value.join(", ")}]`)
      } else if (typeof rule.value === "boolean") {
        lines.push(`      value: ${rule.value}`)
      } else {
        const stringValue = String(rule.value)
        if (stringValue.includes(" ") || stringValue.includes("@")) {
          lines.push(`      value: "${stringValue}"`)
        } else {
          lines.push(`      value: ${stringValue}`)
        }
      }
    }
    lines.push("")
  } else {
    lines.push("# conditions:")
    lines.push("#   match: all")
    lines.push("#   rules:")
    lines.push('#     - path: comment.body')
    lines.push('#       operator: contains')
    lines.push('#       value: "@zira"')
    lines.push("")
  }

  if (actions.length === 0) {
    lines.push("context:")
    lines.push("  # - as: issue")
    lines.push("  #   action: issues_get")
    lines.push("  #   ref: issue")
  } else {
    lines.push("context:")
    for (const action of actions) {
      lines.push(`  - as: ${action.as}`)
      lines.push(`    action: ${action.action}`)
      if (action.ref) lines.push(`    ref: ${action.ref}`)
      if (action.params && Object.keys(action.params).length > 0) {
        lines.push("    params:")
        for (const [key, value] of Object.entries(action.params)) {
          const stringValue = String(value)
          if (stringValue.includes("{{") || stringValue.includes(" ")) {
            lines.push(`      ${key}: "${stringValue}"`)
          } else {
            lines.push(`      ${key}: ${stringValue}`)
          }
        }
      }
      if (action.optional) lines.push("    optional: true")
    }
  }

  lines.push("")

  if (prompt) {
    lines.push("instructions: |")
    for (const promptLine of prompt.split("\n")) {
      lines.push(`  ${promptLine}`)
    }
  } else {
    const eventNames = triggerKeys
      .map((key) => key.split(".").map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(" "))
      .join(" / ")
    const refNames = Object.keys(refs)
    const hasIssue = refNames.includes("issue_number")
    const hasPR = refNames.includes("pull_number")

    lines.push("instructions: |")
    lines.push(`  An event was triggered in $refs.repository.`)
    lines.push("")
    lines.push(`  ## Triggering Event`)
    lines.push(`  ${eventNames}`)
    lines.push("")
    if (hasIssue) {
      lines.push("  ## Issue")
      lines.push("  **{{$issue.title}}**")
      lines.push("  {{$issue.body}}")
      lines.push("")
    }
    if (hasPR) {
      lines.push("  ## Pull Request")
      lines.push("  **{{$pr.title}}**")
      lines.push("  {{$pr.body}}")
      lines.push("")
    }
    lines.push("  Analyze the event and take appropriate action.")
  }

  return lines.join("\n") + "\n"
}

export function parseRecipeYaml(yamlText: string): ParsedRecipe | null {
  try {
    const actions: ContextActionConfig[] = []
    const lines = yamlText.split("\n")
    let currentAction: Partial<ContextActionConfig> | null = null
    let inParams = false
    let section: "none" | "conditions" | "context" | "prompt" = "none"
    let inPromptBlock = false
    const promptLines: string[] = []

    let conditionsMatch: "all" | "any" = "all"
    const conditionRules: TriggerConditionConfig[] = []
    let currentRule: Partial<TriggerConditionConfig> | null = null
    let inRules = false

    function flushRule() {
      if (currentRule?.path && currentRule?.operator) {
        conditionRules.push({ path: currentRule.path, operator: currentRule.operator, value: currentRule.value ?? null })
      }
      currentRule = null
    }

    function flushAction() {
      if (currentAction?.as && currentAction?.action) {
        actions.push({ as: currentAction.as, action: currentAction.action, actionDisplayName: currentAction.action, ref: currentAction.ref, params: currentAction.params, optional: currentAction.optional })
      }
      currentAction = null
      inParams = false
    }

    for (const rawLine of lines) {
      const line = rawLine.trimEnd()
      const trimmed = line.trim()

      if (trimmed === "conditions:") { flushRule(); flushAction(); section = "conditions"; inPromptBlock = false; inRules = false; continue }
      if (trimmed === "context:") { flushRule(); flushAction(); section = "context"; inPromptBlock = false; continue }
      if (trimmed.startsWith("instructions:") || trimmed.startsWith("prompt:")) {
        flushRule(); flushAction()
        section = "prompt"
        inPromptBlock = true
        const colonIndex = trimmed.indexOf(":")
        const inlineValue = trimmed.slice(colonIndex + 1).trim()
        if (inlineValue && inlineValue !== "|") promptLines.push(inlineValue)
        continue
      }

      if (section === "prompt" && inPromptBlock) {
        if (line.startsWith("  ")) { promptLines.push(line.slice(2)); continue }
        if (trimmed === "") { promptLines.push(""); continue }
        inPromptBlock = false
        continue
      }

      if (section === "conditions") {
        if (trimmed === "" || trimmed.startsWith("#")) continue
        if (trimmed.startsWith("match:")) { conditionsMatch = trimmed.replace("match:", "").trim() as "all" | "any"; continue }
        if (trimmed === "rules:") { inRules = true; continue }
        if (inRules) {
          if (trimmed.startsWith("- path:")) {
            flushRule()
            currentRule = { path: trimmed.replace("- path:", "").trim() }
            continue
          }
          if (currentRule) {
            if (trimmed.startsWith("operator:")) {
              currentRule.operator = trimmed.replace("operator:", "").trim()
            } else if (trimmed.startsWith("value:")) {
              let rawValue = trimmed.replace("value:", "").trim()
              if ((rawValue.startsWith('"') && rawValue.endsWith('"')) || (rawValue.startsWith("'") && rawValue.endsWith("'"))) {
                rawValue = rawValue.slice(1, -1)
              }
              if (rawValue.startsWith("[") && rawValue.endsWith("]")) {
                currentRule.value = rawValue.slice(1, -1).split(",").map((item) => item.trim())
              } else if (rawValue === "true") {
                currentRule.value = true
              } else if (rawValue === "false") {
                currentRule.value = false
              } else {
                currentRule.value = rawValue
              }
            }
          }
        }
        continue
      }

      if (section !== "context") continue
      if (trimmed === "" || trimmed.startsWith("#")) continue

      if (trimmed.startsWith("- as:")) {
        flushAction()
        currentAction = { as: trimmed.replace("- as:", "").trim() }
        continue
      }

      if (!currentAction) continue

      if (trimmed.startsWith("action:")) { currentAction.action = trimmed.replace("action:", "").trim(); inParams = false }
      else if (trimmed.startsWith("ref:")) { currentAction.ref = trimmed.replace("ref:", "").trim(); inParams = false }
      else if (trimmed.startsWith("optional:")) { currentAction.optional = trimmed.replace("optional:", "").trim() === "true"; inParams = false }
      else if (trimmed === "params:") { currentAction.params = {}; inParams = true }
      else if (inParams && trimmed.includes(":")) {
        const colonIndex = trimmed.indexOf(":")
        const paramKey = trimmed.slice(0, colonIndex).trim()
        let paramValue = trimmed.slice(colonIndex + 1).trim()
        if ((paramValue.startsWith('"') && paramValue.endsWith('"')) || (paramValue.startsWith("'") && paramValue.endsWith("'"))) {
          paramValue = paramValue.slice(1, -1)
        }
        if (!currentAction.params) currentAction.params = {}
        currentAction.params[paramKey] = paramValue
      }
    }

    flushRule()
    flushAction()
    while (promptLines.length > 0 && promptLines[promptLines.length - 1].trim() === "") promptLines.pop()

    const conditions = conditionRules.length > 0 ? { match: conditionsMatch, rules: conditionRules } : undefined

    return { conditions, context: actions, prompt: promptLines.join("\n") }
  } catch {
    return null
  }
}
