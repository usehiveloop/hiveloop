import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

export function IntegrationStack({ integrations }: { integrations: string[] }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div className="flex items-center cursor-default">
            {integrations.map((integration, i) => (
              <div
                key={integration}
                className="flex h-6 w-6 items-center justify-center rounded-full border-2 border-background bg-muted text-[8px] font-bold text-muted-foreground"
                style={{
                  marginLeft: i > 0 ? "-6px" : 0,
                  zIndex: integrations.length - i,
                }}
              >
                {integration[0]}
              </div>
            ))}
          </div>
        }
      />
      <TooltipContent>{integrations.join(", ")}</TooltipContent>
    </Tooltip>
  )
}
