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
import { cn } from "@/lib/utils"

const CodeMirror = dynamic(
  () => import("@uiw/react-codemirror").then((mod) => mod.default),
  {
    ssr: false,
    loading: () => (
      <div className="h-[280px] w-full animate-pulse rounded-xl border border-input bg-muted/30" />
    ),
  },
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

interface SkillContentEditorProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  height?: string
  className?: string
}

export function SkillContentEditor({
  value,
  onChange,
  placeholder,
  height = "280px",
  className,
}: SkillContentEditorProps) {
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme === "dark"

  const extensions = React.useMemo(
    () => [markdown(), chromeOverrides, EditorView.lineWrapping],
    [],
  )

  return (
    <div
      className={cn(
        "overflow-hidden rounded-xl border border-input transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/30",
        className,
      )}
    >
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
        height={height}
        style={{ height }}
      />
    </div>
  )
}
