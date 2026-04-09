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
}

export function RecipeEditor({ value, onChange, refNames, actionPaths }: RecipeEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const disposablesRef = useRef<IDisposable[]>([])

  const handleBeforeMount: BeforeMount = useCallback((monaco: Monaco) => {
    for (const disposable of disposablesRef.current) disposable.dispose()
    disposablesRef.current = []

    const disposable = monaco.languages.registerCompletionItemProvider("yaml", {
      triggerCharacters: ["$", ".", "{"],
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

        const yamlKeywords = [
          { label: "context:", detail: "Context actions section" },
          { label: "prompt: |", detail: "Prompt template section" },
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
