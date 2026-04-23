"use client"

import { useState, useCallback, useRef, useEffect } from "react"
import dynamic from "next/dynamic"
import { AnimatePresence, motion } from "motion/react"
import type { OnMount } from "@monaco-editor/react"
import type { editor } from "monaco-editor"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons"
import { ProviderLogo } from "@/components/provider-logo"
import { cn } from "@/lib/utils"

const Editor = dynamic(() => import("@monaco-editor/react").then((mod) => mod.default), {
  ssr: false,
  loading: () => (
    <div className="flex-1 min-h-0 w-full rounded-xl border border-input bg-muted/30 animate-pulse" />
  ),
})

const PROMPT_PROVIDERS = [
  { id: "anthropic", name: "Anthropic", logo: "anthropic" },
  { id: "openai", name: "OpenAI", logo: "openai" },
  { id: "gemini", name: "Gemini", logo: "google" },
  { id: "glm", name: "GLM", logo: "zai" },
  { id: "kimi", name: "Kimi", logo: "moonshotai" },
  { id: "minimax", name: "MiniMax", logo: "minimax" },
] as const

export type PromptProviderId = (typeof PROMPT_PROVIDERS)[number]["id"]

interface ProviderPromptEditorProps {
  value: Record<string, string>
  onChange: (value: Record<string, string>) => void
  className?: string
}

export function ProviderPromptEditor({ value, onChange, className }: ProviderPromptEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const direction = useRef<1 | -1>(1)
  const valueRef = useRef(value)
  const onChangeRef = useRef(onChange)
  const [activeIndex, setActiveIndex] = useState(0)

  valueRef.current = value
  onChangeRef.current = onChange

  const activeProvider = PROMPT_PROVIDERS[activeIndex]
  const activeProviderId = activeProvider.id
  const activeValue = value[activeProviderId] ?? ""

  const filledCount = PROMPT_PROVIDERS.filter((provider) => (value[provider.id] ?? "").trim()).length

  const goLeft = useCallback(() => {
    direction.current = -1
    setActiveIndex((previous) => (previous - 1 + PROMPT_PROVIDERS.length) % PROMPT_PROVIDERS.length)
  }, [])

  const goRight = useCallback(() => {
    direction.current = 1
    setActiveIndex((previous) => (previous + 1) % PROMPT_PROVIDERS.length)
  }, [])

  const handleEditorChange = useCallback(
    (newValue: string | undefined) => {
      onChangeRef.current({ ...valueRef.current, [activeProviderId]: newValue ?? "" })
    },
    [activeProviderId],
  )

  const handleMount: OnMount = useCallback((mountedEditor) => {
    editorRef.current = mountedEditor
    mountedEditor.focus()
  }, [])

  useEffect(() => {
    if (editorRef.current) {
      const model = editorRef.current.getModel()
      if (model) {
        model.setValue(activeValue)
      }
    }
  }, [activeProviderId])

  return (
    <div className={cn("flex flex-col gap-3 flex-1 min-h-0", className)}>
      {/* Monaco editor */}
      <div className="flex-1 min-h-0 w-full rounded-xl border border-input bg-muted/30 overflow-hidden">
        <Editor
          defaultLanguage="markdown"
          defaultValue={activeValue}
          onChange={handleEditorChange}
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
          }}
          theme="vs-dark"
        />
      </div>

      {/* Provider switcher */}
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={goLeft}
          className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors shrink-0"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={14} className="text-muted-foreground" />
        </button>

        <div className="relative min-w-24 h-6 overflow-hidden shrink-0">
          <AnimatePresence mode="wait" custom={direction.current}>
            <motion.div
              key={activeProviderId}
              custom={direction.current}
              initial="enter"
              animate="center"
              exit="exit"
              variants={{
                enter: (dir: number) => ({ x: dir > 0 ? 40 : -40, opacity: 0 }),
                center: { x: 0, opacity: 1 },
                exit: (dir: number) => ({ x: dir > 0 ? -40 : 40, opacity: 0 }),
              }}
              transition={{ duration: 0.15, ease: "easeInOut" }}
              className="absolute inset-0 flex items-center gap-2 justify-center"
            >
              <ProviderLogo provider={activeProvider.logo} size={16} />
              <span className="text-xs font-medium truncate">
                {activeProvider.name}
              </span>
            </motion.div>
          </AnimatePresence>
        </div>

        <button
          type="button"
          onClick={goRight}
          className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors shrink-0"
        >
          <HugeiconsIcon icon={ArrowRight01Icon} size={14} className="text-muted-foreground" />
        </button>

        <div className="flex items-center gap-1 ml-auto">
          {PROMPT_PROVIDERS.map((provider, index) => (
            <button
              key={provider.id}
              type="button"
              onClick={() => {
                direction.current = index > activeIndex ? 1 : -1
                setActiveIndex(index)
              }}
              className={cn(
                "h-1.5 rounded-full transition-all",
                index === activeIndex
                  ? "w-4 bg-primary"
                  : (value[provider.id] ?? "").trim()
                    ? "w-1.5 bg-muted-foreground/50"
                    : "w-1.5 bg-muted-foreground/20",
              )}
            />
          ))}
        </div>

        {filledCount > 0 && (
          <p className="text-mini text-muted-foreground shrink-0">
            {filledCount}/{PROMPT_PROVIDERS.length}
          </p>
        )}
      </div>
    </div>
  )
}
