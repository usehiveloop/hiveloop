import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  MoreHorizontalIcon,
  Edit02Icon,
  Copy01Icon,
  Delete02Icon,
  BrainIcon,
  SparklesIcon,
  Settings01Icon,
} from "@hugeicons/core-free-icons"
import { ProviderIcon } from "@/components/provider-icon"

type AgentHeaderProps = {
  name: string
  provider: string
  model: string
  sandboxType: string
  memoryEnabled: boolean
  status: string
  onStartConversation?: () => void
  onEdit?: () => void
}

export function AgentHeader({ name, provider, model, sandboxType, memoryEnabled, onStartConversation, onEdit }: AgentHeaderProps) {
  return (
    <div className="flex flex-col gap-4 mb-8">
      {/* Top: name + actions */}
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2.5 min-w-0">
          <span className="h-2 w-2 rounded-full bg-green-500 shrink-0" />
          <h1 className="font-heading text-xl font-semibold text-foreground truncate">{name}</h1>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          <Button variant="outline" size="sm" className="hidden sm:inline-flex" onClick={onEdit}>
            <HugeiconsIcon icon={Edit02Icon} size={14} data-icon="inline-start" />
            Edit
          </Button>
          <Button size="sm" onClick={onStartConversation} className="hidden sm:inline-flex">
            Start run
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
              <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" sideOffset={4}>
              <DropdownMenuGroup>
                <DropdownMenuItem className="sm:hidden" onClick={onEdit}>
                  <HugeiconsIcon icon={Edit02Icon} size={16} className="text-muted-foreground" />
                  Edit
                </DropdownMenuItem>
                <DropdownMenuItem className="sm:hidden" onClick={onStartConversation}>
                  <HugeiconsIcon icon={SparklesIcon} size={16} className="text-muted-foreground" />
                  Start run
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <HugeiconsIcon icon={SparklesIcon} size={16} className="text-muted-foreground" />
                  Run Forge
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <HugeiconsIcon icon={Copy01Icon} size={16} className="text-muted-foreground" />
                  Duplicate
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <HugeiconsIcon icon={Settings01Icon} size={16} className="text-muted-foreground" />
                  Settings
                </DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive">
                <HugeiconsIcon icon={Delete02Icon} size={16} />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Meta: model, sandbox, memory — wraps on mobile */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
        <span className="flex items-center gap-1.5 font-mono text-xs">
          <ProviderIcon provider={provider} />
          {model}
        </span>
        <span className="hidden sm:inline">·</span>
        <span>{sandboxType}</span>
        {memoryEnabled && (
          <>
            <span className="hidden sm:inline">·</span>
            <span className="flex items-center gap-1 text-primary">
              <HugeiconsIcon icon={BrainIcon} size={13} />
              Memory
            </span>
          </>
        )}
      </div>
    </div>
  )
}
