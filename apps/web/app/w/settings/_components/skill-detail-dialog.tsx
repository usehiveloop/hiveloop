"use client"

import { useState, useMemo, useEffect } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { cn } from "@/lib/utils"
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  File01Icon,
  Folder01Icon,
  CodeIcon,
  TextIcon,
} from "@hugeicons/core-free-icons"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism"
import { SkillContentEditor } from "./skill-content-editor"

type SkillRow = components["schemas"]["skillResponse"]

interface SkillFile {
  path: string
  body: string
}

interface FileTreeNode {
  name: string
  path: string
  children: FileTreeNode[]
  file?: SkillFile
}

function buildFileTree(files: SkillFile[]): FileTreeNode[] {
  const root: FileTreeNode[] = []

  for (const file of files) {
    const parts = file.path.split("/")
    let current = root

    for (let partIndex = 0; partIndex < parts.length; partIndex++) {
      const part = parts[partIndex]
      const isLast = partIndex === parts.length - 1

      let existing = current.find((node) => node.name === part)
      if (!existing) {
        existing = {
          name: part,
          path: parts.slice(0, partIndex + 1).join("/"),
          children: [],
          file: isLast ? file : undefined,
        }
        current.push(existing)
      }

      if (!isLast) {
        current = existing.children
      }
    }
  }

  return sortTree(root)
}

function sortTree(nodes: FileTreeNode[]): FileTreeNode[] {
  return nodes.sort((first, second) => {
    const firstIsDir = first.children.length > 0
    const secondIsDir = second.children.length > 0
    if (firstIsDir && !secondIsDir) return -1
    if (!firstIsDir && secondIsDir) return 1
    return first.name.localeCompare(second.name)
  }).map((node) => ({
    ...node,
    children: sortTree(node.children),
  }))
}

function isMarkdownFile(path: string): boolean {
  return path.endsWith(".md") || path.endsWith(".mdx")
}

function getLanguageFromPath(path: string): string {
  const extension = path.split(".").pop()?.toLowerCase() ?? ""
  const languageMap: Record<string, string> = {
    sh: "bash",
    bash: "bash",
    zsh: "bash",
    js: "javascript",
    ts: "typescript",
    tsx: "typescript",
    jsx: "javascript",
    py: "python",
    rb: "ruby",
    go: "go",
    rs: "rust",
    yaml: "yaml",
    yml: "yaml",
    json: "json",
    toml: "toml",
    md: "markdown",
    mdx: "markdown",
  }
  return languageMap[extension] ?? "text"
}

interface FileTreeItemProps {
  node: FileTreeNode
  depth: number
  selectedPath: string
  onSelect: (path: string) => void
}

function FileTreeItem({ node, depth, selectedPath, onSelect }: FileTreeItemProps) {
  const [expanded, setExpanded] = useState(true)
  const isDir = node.children.length > 0
  const isSelected = node.path === selectedPath

  if (isDir) {
    return (
      <div>
        <button
          type="button"
          onClick={() => setExpanded(!expanded)}
          className="flex items-center gap-2 w-full px-2 py-1.5 text-left text-xs text-muted-foreground hover:text-foreground hover:bg-muted/50 rounded-lg transition-colors cursor-pointer"
          style={{ paddingLeft: `${depth * 12 + 8}px` }}
        >
          <HugeiconsIcon icon={Folder01Icon} size={14} className="shrink-0 text-muted-foreground" />
          <span className="truncate">{node.name}</span>
        </button>
        {expanded && node.children.map((child) => (
          <FileTreeItem
            key={child.path}
            node={child}
            depth={depth + 1}
            selectedPath={selectedPath}
            onSelect={onSelect}
          />
        ))}
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={() => onSelect(node.path)}
      className={cn(
        "flex items-center gap-2 w-full px-2 py-1.5 text-left text-xs rounded-lg transition-colors cursor-pointer",
        isSelected
          ? "bg-primary/10 text-foreground"
          : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
      )}
      style={{ paddingLeft: `${depth * 12 + 8}px` }}
    >
      <HugeiconsIcon icon={File01Icon} size={14} className="shrink-0" />
      <span className="truncate">{node.name}</span>
    </button>
  )
}

interface MarkdownViewerProps {
  content: string
}

function MarkdownViewer({ content }: MarkdownViewerProps) {
  return (
    <div className="prose prose-sm dark:prose-invert max-w-none prose-headings:font-heading prose-h1:text-xl prose-h2:text-lg prose-h3:text-base prose-p:text-sm prose-p:leading-relaxed prose-li:text-sm prose-code:text-xs prose-code:before:content-none prose-code:after:content-none prose-pre:bg-[#282c34] prose-pre:rounded-xl prose-table:text-sm prose-th:text-xs prose-td:text-xs">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          code({ className, children, ...props }) {
            const match = /language-(\w+)/.exec(className ?? "")
            const codeString = String(children).replace(/\n$/, "")

            if (match) {
              return (
                <SyntaxHighlighter
                  style={oneDark}
                  language={match[1]}
                  PreTag="div"
                  customStyle={{ fontSize: "12px", borderRadius: "12px", margin: 0 }}
                >
                  {codeString}
                </SyntaxHighlighter>
              )
            }

            return (
              <code className={cn("rounded bg-muted px-1.5 py-0.5 text-xs font-mono", className)} {...props}>
                {children}
              </code>
            )
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}

interface RawViewerProps {
  content: string
  language: string
}

function RawViewer({ content, language }: RawViewerProps) {
  return (
    <SyntaxHighlighter
      style={oneDark}
      language={language}
      showLineNumbers
      customStyle={{ fontSize: "12px", borderRadius: "12px", margin: 0, overflow: "auto", maxWidth: "100%" }}
      lineNumberStyle={{ minWidth: "2.5em", paddingRight: "1em", color: "#636d83" }}
      codeTagProps={{ style: { whiteSpace: "pre-wrap", wordBreak: "break-all" } }}
    >
      {content}
    </SyntaxHighlighter>
  )
}

interface SkillDetailDialogProps {
  skill: SkillRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
  initialEditing?: boolean
}

export function SkillDetailDialog({
  skill,
  open,
  onOpenChange,
  initialEditing = false,
}: SkillDetailDialogProps) {
  const queryClient = useQueryClient()
  const [selectedPath, setSelectedPath] = useState("SKILL.md")
  const [viewMode, setViewMode] = useState<"rendered" | "raw">(
    initialEditing ? "raw" : "rendered",
  )
  const [isEditingContent, setIsEditingContent] = useState(initialEditing)
  const [editedFiles, setEditedFiles] = useState<Record<string, string>>({})

  const { data, isLoading } = $api.useQuery(
    "get",
    "/v1/skills/{id}",
    { params: { path: { id: skill?.id ?? "" } } },
    { enabled: open && !!skill?.id },
  )

  const detail = data as components["schemas"]["skillDetailResponse"] | undefined
  const updateContent = $api.useMutation("put", "/v1/skills/{id}/content")

  const files = useMemo((): SkillFile[] => {
    if (!detail?.bundle) return []
    const result: SkillFile[] = []

    if (detail.bundle.content) {
      result.push({ path: "SKILL.md", body: detail.bundle.content })
    }

    for (const reference of detail.bundle.references ?? []) {
      if (reference.path && reference.body) {
        result.push({ path: reference.path, body: reference.body })
      }
    }

    return result
  }, [detail])

  const fileTree = useMemo(() => buildFileTree(files), [files])

  const selectedFile = files.find((file) => file.path === selectedPath) ?? files[0]
  const selectedBody =
    selectedFile && editedFiles[selectedFile.path] !== undefined
      ? editedFiles[selectedFile.path]
      : selectedFile?.body ?? ""
  const isMarkdown = selectedFile ? isMarkdownFile(selectedFile.path) : false
  const language = selectedFile ? getLanguageFromPath(selectedFile.path) : "text"
  const hasEdits = Object.keys(editedFiles).length > 0

  useEffect(() => {
    if (!open) return
    setSelectedPath("SKILL.md")
    setViewMode(initialEditing ? "raw" : "rendered")
    setIsEditingContent(initialEditing)
    setEditedFiles({})
  }, [open, skill?.id, initialEditing])

  function exitEditing() {
    setIsEditingContent(false)
    setEditedFiles({})
  }

  function handleEditorChange(nextValue: string) {
    if (!selectedFile) return
    setEditedFiles((prev) => ({ ...prev, [selectedFile.path]: nextValue }))
  }

  function handleSaveContent() {
    if (!skill?.id || !detail?.bundle) return

    const skillMdContent =
      editedFiles["SKILL.md"] ?? detail.bundle.content ?? ""
    const references = (detail.bundle.references ?? []).map((reference) => ({
      path: reference.path ?? "",
      body:
        reference.path && editedFiles[reference.path] !== undefined
          ? editedFiles[reference.path]
          : reference.body ?? "",
    }))

    updateContent.mutate(
      {
        params: { path: { id: skill.id } },
        body: {
          bundle: {
            id: detail.bundle.id ?? skill.slug,
            title: detail.bundle.title ?? skill.name,
            description: detail.bundle.description ?? skill.description ?? "",
            content: skillMdContent,
            parameters_schema: detail.bundle.parameters_schema,
            manifest: detail.bundle.manifest,
            references,
          },
        },
      },
      {
        onSuccess: () => {
          toast.success("Skill content updated")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills/{id}"] })
          exitEditing()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to update skill content"))
        },
      },
    )
  }

  function handleDialogOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      exitEditing()
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent
        showCloseButton
        className="max-w-[100vw] h-dvh rounded-none p-0 gap-0 overflow-hidden md:max-w-5xl md:h-[80vh] md:rounded-4xl"
      >
        <DialogTitle className="sr-only">{skill?.name ?? "Skill"}</DialogTitle>

        {isLoading ? (
          <div className="absolute inset-0 flex">
            <div className="w-56 shrink-0 border-r border-border p-3 space-y-2">
              <Skeleton className="h-5 w-24" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-2/3" />
            </div>
            <div className="flex-1 p-6 space-y-3">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-32 w-full" />
            </div>
          </div>
        ) : files.length === 0 ? (
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="text-center">
              <p className="text-sm font-medium text-foreground">No content</p>
              <p className="text-xs text-muted-foreground mt-1">This skill has no hydrated content yet.</p>
            </div>
          </div>
        ) : (
          <div className="absolute inset-0 flex">
            <div className="w-56 shrink-0 border-r border-border flex flex-col overflow-hidden">
              <div className="h-16 shrink-0 flex items-center px-3 border-b border-border">
                <div className="min-w-0">
                  <p className="text-xs font-medium text-foreground truncate">{skill?.name}</p>
                  {skill?.description && (
                    <p className="text-[11px] text-muted-foreground truncate">{skill.description}</p>
                  )}
                </div>
              </div>
              <div className="flex-1 overflow-y-auto p-2">
                {fileTree.map((node) => (
                  <FileTreeItem
                    key={node.path}
                    node={node}
                    depth={0}
                    selectedPath={selectedFile?.path ?? ""}
                    onSelect={setSelectedPath}
                  />
                ))}
              </div>
              {isEditingContent && (
                <div className="shrink-0 border-t border-border p-3">
                  <Button
                    size="sm"
                    className="w-full"
                    loading={updateContent.isPending}
                    disabled={!hasEdits}
                    onClick={handleSaveContent}
                  >
                    Save changes
                  </Button>
                </div>
              )}
            </div>

            <div className="flex-1 flex flex-col overflow-hidden min-w-0">
              <div className="h-16 shrink-0 flex items-center justify-between px-4 pr-24 border-b border-border gap-3">
                <p className="text-xs font-mono text-muted-foreground truncate">
                  {selectedFile?.path}
                </p>
                {isMarkdown && (
                  <div className="flex items-center gap-1 bg-muted rounded-full p-0.5 shrink-0 ml-3">
                    <Button
                      variant="ghost"
                      className={cn(
                        "px-2.5 rounded-full text-[11px]",
                        viewMode === "rendered" && "bg-background shadow-sm",
                      )}
                      onClick={() => setViewMode("rendered")}
                      disabled={isEditingContent}
                    >
                      <HugeiconsIcon icon={TextIcon} size={12} />
                      Preview
                    </Button>
                    <Button
                      variant="ghost"
                      className={cn(
                        "px-2.5 rounded-full text-[11px]",
                        viewMode === "raw" && "bg-background shadow-sm",
                      )}
                      onClick={() => setViewMode("raw")}
                    >
                      <HugeiconsIcon icon={CodeIcon} size={12} />
                      Source
                    </Button>
                  </div>
                )}
              </div>

              {isEditingContent && viewMode === "raw" && selectedFile ? (
                <div className="flex-1 relative min-h-0">
                  <div className="absolute inset-0 p-6">
                    <SkillContentEditor
                      value={selectedBody}
                      onChange={handleEditorChange}
                      height="100%"
                      className="h-full"
                    />
                  </div>
                </div>
              ) : (
                <div className="flex-1 overflow-y-auto overflow-x-hidden p-6">
                  {selectedFile && isMarkdown && viewMode === "rendered" ? (
                    <MarkdownViewer content={selectedBody} />
                  ) : selectedFile ? (
                    <RawViewer content={selectedBody} language={language} />
                  ) : null}
                </div>
              )}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
