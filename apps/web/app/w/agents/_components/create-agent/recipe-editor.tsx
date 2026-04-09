"use client"

import { useRef, useEffect, useCallback } from "react"
import dynamic from "next/dynamic"
import type { OnMount, BeforeMount, Monaco } from "@monaco-editor/react"
import type { editor, IDisposable } from "monaco-editor"

const Editor = dynamic(() => import("@monaco-editor/react").then((mod) => mod.default), {
  ssr: false,
  loading: () => (
    <div className="flex-1 min-h-0 w-full rounded-xl border border-input bg-muted/30 animate-pulse" />
  ),
})

interface RecipeEditorProps {
  value: string
  onChange: (value: string) => void
  refNames: string[]
  actionPaths: Record<string, Array<{ path: string; type: string }>>
  actionKeys: Array<{ key: string; displayName: string; access: string; resourceType: string }>
}

export function RecipeEditor({ value, onChange, refNames, actionPaths, actionKeys }: RecipeEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const disposablesRef = useRef<IDisposable[]>([])

  const handleBeforeMount: BeforeMount = useCallback((monaco: Monaco) => {
    for (const disposable of disposablesRef.current) disposable.dispose()
    disposablesRef.current = []

    const disposable = monaco.languages.registerCompletionItemProvider("yaml", {
      triggerCharacters: ["$", ".", "{", ":", " "],
      provideCompletionItems(model: editor.ITextModel, position: any) {
        const word = model.getWordUntilPosition(position)
        const lineContent = model.getLineContent(position.lineNumber)
        const textBeforeCursor = lineContent.slice(0, position.column - 1)
        const range = {
          startLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endLineNumber: position.lineNumber,
          endColumn: word.endColumn,
        }

        const suggestions: any[] = []

        for (const refName of refNames) {
          suggestions.push({
            label: `$refs.${refName}`,
            kind: monaco.languages.CompletionItemKind.Variable,
            insertText: `$refs.${refName}`,
            detail: "trigger ref",
            range,
            sortText: `0_${refName}`,
          })
        }

        for (const [actionKey, paths] of Object.entries(actionPaths)) {
          for (const schemaPath of paths) {
            suggestions.push({
              label: `$${actionKey}.${schemaPath.path}`,
              kind: monaco.languages.CompletionItemKind.Field,
              insertText: `$${actionKey}.${schemaPath.path}`,
              detail: schemaPath.type,
              range,
              sortText: `1_${actionKey}_${schemaPath.path}`,
            })
          }
        }

        if (textBeforeCursor.includes("{{")) {
          for (const refName of refNames) {
            suggestions.push({
              label: `{{$refs.${refName}}}`,
              kind: monaco.languages.CompletionItemKind.Snippet,
              insertText: `$refs.${refName}}}`,
              detail: "template ref",
              range,
              sortText: `2_${refName}`,
            })
          }
          for (const [actionKey, paths] of Object.entries(actionPaths)) {
            for (const schemaPath of paths) {
              suggestions.push({
                label: `{{$${actionKey}.${schemaPath.path}}}`,
                kind: monaco.languages.CompletionItemKind.Snippet,
                insertText: `$${actionKey}.${schemaPath.path}}}`,
                detail: schemaPath.type,
                range,
                sortText: `3_${actionKey}_${schemaPath.path}`,
              })
            }
          }
        }

        // Action key completions — triggered after "action:" on the same line.
        if (textBeforeCursor.match(/action:\s*$/)) {
          for (const action of actionKeys) {
            suggestions.push({
              label: action.key,
              kind: monaco.languages.CompletionItemKind.Function,
              insertText: action.key,
              detail: `${action.displayName} (${action.access})`,
              documentation: action.resourceType ? `resource: ${action.resourceType}` : undefined,
              range,
              sortText: `0_${action.key}`,
            })
          }
          return { suggestions }
        }

        // Ref name completions — triggered after "ref:" on the same line.
        if (textBeforeCursor.match(/ref:\s*$/)) {
          const resourceTypes = new Set(actionKeys.map((action) => action.resourceType).filter(Boolean))
          for (const resourceType of resourceTypes) {
            suggestions.push({
              label: resourceType,
              kind: monaco.languages.CompletionItemKind.Enum,
              insertText: resourceType,
              detail: "resource type",
              range,
              sortText: `0_${resourceType}`,
            })
          }
          return { suggestions }
        }

        // Operator completions — triggered after "operator:" on the same line.
        if (textBeforeCursor.match(/operator:\s*$/)) {
          const operators = [
            { label: "equals", detail: "Exact match" },
            { label: "not_equals", detail: "Not equal" },
            { label: "one_of", detail: "Matches any in list" },
            { label: "not_one_of", detail: "Matches none in list" },
            { label: "contains", detail: "String contains substring" },
            { label: "not_contains", detail: "String does not contain" },
            { label: "matches", detail: "Regex match" },
            { label: "exists", detail: "Field exists (no value needed)" },
            { label: "not_exists", detail: "Field does not exist" },
          ]
          for (const operator of operators) {
            suggestions.push({
              label: operator.label,
              kind: monaco.languages.CompletionItemKind.EnumMember,
              insertText: operator.label,
              detail: operator.detail,
              range,
              sortText: `0_${operator.label}`,
            })
          }
          return { suggestions }
        }

        // Match mode completions — triggered after "match:" on the same line.
        if (textBeforeCursor.match(/match:\s*$/)) {
          for (const mode of ["all", "any"]) {
            suggestions.push({
              label: mode,
              kind: monaco.languages.CompletionItemKind.EnumMember,
              insertText: mode,
              detail: mode === "all" ? "All conditions must match (AND)" : "Any condition can match (OR)",
              range,
              sortText: `0_${mode}`,
            })
          }
          return { suggestions }
        }

        const yamlKeywords = [
          { label: "conditions:", detail: "Filter conditions section" },
          { label: "context:", detail: "Context actions section" },
          { label: "instructions: |", detail: "Instructions sent to agent when trigger fires" },
          { label: "match:", detail: "Condition match mode (all/any)" },
          { label: "rules:", detail: "Condition rules list" },
          { label: "- path:", detail: "Condition payload path" },
          { label: "operator:", detail: "Condition operator" },
          { label: "value:", detail: "Condition value" },
          { label: "- as:", detail: "Context action name" },
          { label: "action:", detail: "Catalog action key" },
          { label: "ref:", detail: "Resource ref for auto-params" },
          { label: "params:", detail: "Custom parameters" },
          { label: "optional: true", detail: "Continue on failure" },
          { label: "only_when:", detail: "Only run for specific events" },
        ]
        for (const keyword of yamlKeywords) {
          suggestions.push({
            label: keyword.label,
            kind: monaco.languages.CompletionItemKind.Keyword,
            insertText: keyword.label,
            detail: keyword.detail,
            range,
            sortText: `9_${keyword.label}`,
          })
        }

        return { suggestions }
      },
    })

    disposablesRef.current.push(disposable)
  }, [refNames, actionPaths])

  const handleMount: OnMount = useCallback((mountedEditor) => {
    editorRef.current = mountedEditor
    mountedEditor.focus()
  }, [])

  // Clean up disposables on unmount.
  useEffect(() => {
    return () => {
      for (const disposable of disposablesRef.current) disposable.dispose()
      disposablesRef.current = []
    }
  }, [])

  return (
    <div className="flex-1 min-h-0 w-full rounded-xl border border-input bg-muted/30 overflow-hidden">
      <Editor
        defaultLanguage="yaml"
        value={value}
        onChange={(newValue) => onChange(newValue ?? "")}
        beforeMount={handleBeforeMount}
        onMount={handleMount}
        options={{
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          fontSize: 12,
          fontFamily: "var(--font-mono, ui-monospace, monospace)",
          lineNumbers: "off",
          glyphMargin: false,
          folding: false,
          lineDecorationsWidth: 0,
          lineNumbersMinChars: 0,
          renderLineHighlight: "none",
          overviewRulerLanes: 0,
          hideCursorInOverviewRuler: true,
          overviewRulerBorder: false,
          scrollbar: {
            vertical: "auto",
            horizontal: "hidden",
            verticalScrollbarSize: 6,
          },
          padding: { top: 12, bottom: 12 },
          wordWrap: "on",
          tabSize: 2,
          insertSpaces: true,
          autoIndent: "full",
          quickSuggestions: { other: true, strings: true, comments: false },
          suggestOnTriggerCharacters: true,
          acceptSuggestionOnEnter: "on",
          tabCompletion: "on",
          wordBasedSuggestions: "off",
        }}
        theme="vs-dark"
      />
    </div>
  )
}
