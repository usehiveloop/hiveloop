"use client"

import * as React from "react"
import dynamic from "next/dynamic"
import { EditorView } from "@codemirror/view"
import { markdown } from "@codemirror/lang-markdown"
import {
  materialDark,
  materialLight,
} from "@uiw/codemirror-theme-material"
import { useTheme } from "next-themes"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { SparklesIcon } from "@hugeicons/core-free-icons"

const CodeMirror = dynamic(
  () => import("@uiw/react-codemirror").then((mod) => mod.default),
  {
    ssr: false,
    loading: () => (
      <div className="h-72 w-full animate-pulse rounded-xl border border-input bg-muted/30" />
    ),
  }
)

const chromeOverrides = EditorView.theme({
  "&": { height: "100%", fontSize: "13px" },
  ".cm-content": {
    padding: "12px 14px",
    fontFamily:
      "var(--font-mono), ui-monospace, SFMono-Regular, Menlo, monospace",
  },
  ".cm-gutters": { display: "none" },
  ".cm-focused": { outline: "none" },
  ".cm-line": { padding: "0" },
  ".cm-scroller": { overflow: "auto" },
})

interface SystemPromptEditorProps {
  value: string
  onChange: (value: string) => void
  onEnhance?: () => void
  isEnhancing?: boolean
  placeholder?: string
}

export function SystemPromptEditor({
  value,
  onChange,
  onEnhance,
  isEnhancing,
  placeholder = "You are a helpful assistant that…",
}: SystemPromptEditorProps) {
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme === "dark"

  const extensions = React.useMemo(
    () => [markdown(), chromeOverrides, EditorView.lineWrapping],
    []
  )

  return (
    <div className="flex flex-col gap-2">
      <div className="overflow-hidden rounded-xl border border-input transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/30">
        <CodeMirror
          value={value}
          onChange={onChange}
          theme={isDark ? materialDark : materialLight}
          extensions={extensions}
          basicSetup={{
            lineNumbers: false,
            foldGutter: false,
            highlightActiveLine: false,
            highlightActiveLineGutter: false,
            indentOnInput: false,
            bracketMatching: false,
            autocompletion: false,
            searchKeymap: false,
          }}
          placeholder={placeholder}
          height="288px"
        />
      </div>

      <div className="flex justify-end">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 text-[12px] text-muted-foreground hover:text-foreground"
          onClick={onEnhance}
          disabled={!value.trim() || isEnhancing}
          loading={isEnhancing}
        >
          <HugeiconsIcon
            icon={SparklesIcon}
            strokeWidth={2}
            className="size-3.5"
            data-icon="inline-start"
          />
          Enhance prompt
        </Button>
      </div>
    </div>
  )
}
