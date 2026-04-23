import { IntegrationLogo } from "@/components/integration-logo"
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

export interface IntegrationSummary {
  provider: string
  name: string
  actions: string[]
}

interface IntegrationLogosProps {
  integrations: IntegrationSummary[]
  max?: number
  size?: number
  className?: string
}

export function IntegrationLogos({ integrations, max = 4, size = 20, className }: IntegrationLogosProps) {
  if (integrations.length === 0) return null

  const visible = integrations.slice(0, max)
  const overflow = integrations.length - max

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div className={cn("flex items-center justify-center cursor-default w-fit", className)}>
            {visible.map((integration, i) => (
              <div
                key={integration.provider}
                className="rounded-full border-2 border-background"
                style={{ marginLeft: i === 0 ? 0 : -(size * 0.3) }}
              >
                <IntegrationLogo provider={integration.provider} size={size} />
              </div>
            ))}
            {overflow > 0 && (
              <span
                className="flex items-center justify-center rounded-full border-2 border-background bg-muted text-2xs font-medium text-muted-foreground"
                style={{
                  marginLeft: -(size * 0.3),
                  width: size,
                  height: size,
                }}
              >
                +{overflow}
              </span>
            )}
          </div>
        }
      />
      <TooltipContent
        side="bottom"
        align="center"
        className="flex flex-col gap-2.5 max-w-xs p-3"
      >
        {integrations.map((integration) => (
          <div key={integration.provider} className="flex items-start gap-2">
            <IntegrationLogo provider={integration.provider} size={16} className="shrink-0 mt-0.5" />
            <div className="flex flex-col gap-0.5 min-w-0">
              <span className="text-xs font-medium">{integration.name}</span>
              {integration.actions.length > 0 && (
                <span className="text-2xs opacity-70 leading-relaxed">
                  {integration.actions.join(", ")}
                </span>
              )}
            </div>
          </div>
        ))}
      </TooltipContent>
    </Tooltip>
  )
}
