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
import { cn } from "@/lib/utils"

const CodeMirror = dynamic(
  () => import("@uiw/react-codemirror").then((mod) => mod.default),
  {
    ssr: false,
    loading: () => (
      <div className="h-[480px] w-full animate-pulse rounded-xl border border-input bg-muted/30" />
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

type Tab = "brief" | "prompt"

interface SystemPromptEditorProps {
  value: string
  onChange: (value: string) => void
  onEnhance?: (brief: string) => void
  isEnhancing?: boolean
}

export function SystemPromptEditor({
  value,
  onChange,
  onEnhance,
  isEnhancing,
}: SystemPromptEditorProps) {
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme === "dark"

  const [brief, setBrief] = React.useState("")
  const [activeTab, setActiveTab] = React.useState<Tab>("brief")

  React.useEffect(() => {
    if (isEnhancing) setActiveTab("prompt")
  }, [isEnhancing])

  const extensions = React.useMemo(
    () => [markdown(), chromeOverrides, EditorView.lineWrapping],
    []
  )

  const tabsDisabled = !!isEnhancing
  const promptReadOnly = !!isEnhancing

  const basicSetup = {
    lineNumbers: false,
    foldGutter: false,
    highlightActiveLine: false,
    highlightActiveLineGutter: false,
    indentOnInput: false,
    bracketMatching: false,
    autocompletion: false,
    searchKeymap: false,
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="overflow-hidden rounded-xl border border-input transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/30">
        {activeTab === "brief" ? (
          <CodeMirror
            value={brief}
            onChange={setBrief}
            theme={isDark ? materialDark : materialLight}
            extensions={extensions}
            basicSetup={basicSetup}
            placeholder="Describe what this agent should do, in your own words. The Enhance prompt button feeds this to the prompt writer."
            height="480px"
          />
        ) : (
          <CodeMirror
            value={value}
            onChange={onChange}
            theme={isDark ? materialDark : materialLight}
            extensions={extensions}
            readOnly={promptReadOnly}
            basicSetup={basicSetup}
            placeholder="The generated system prompt appears here. Click Enhance prompt to populate."
            height="480px"
          />
        )}
      </div>

      <div className="flex items-center justify-between">
        <div className="inline-flex items-center gap-0.5 rounded-md border border-input bg-muted/30 p-0.5">
          <TabButton
            active={activeTab === "brief"}
            disabled={tabsDisabled}
            onClick={() => setActiveTab("brief")}
          >
            Brief
          </TabButton>
          <TabButton
            active={activeTab === "prompt"}
            disabled={tabsDisabled}
            onClick={() => setActiveTab("prompt")}
          >
            System prompt
          </TabButton>
        </div>

        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 text-[12px] text-muted-foreground hover:text-foreground"
          onClick={() => onEnhance?.(brief)}
          disabled={isEnhancing || !brief.trim()}
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

function TabButton({
  active,
  disabled,
  onClick,
  children,
}: {
  active: boolean
  disabled?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        "rounded px-2.5 py-1 text-[12px] font-medium transition-colors",
        active
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:text-foreground",
        disabled && "cursor-not-allowed opacity-50 hover:text-muted-foreground"
      )}
    >
      {children}
    </button>
  )
}
