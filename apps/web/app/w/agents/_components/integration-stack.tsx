import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

export function IntegrationStack({ integrations }: { integrations: string[] }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div className="flex items-center cursor-default">
            {integrations.map((integration, i) => (
              // zIndex depends on the integration's index so earlier ones sit
              // on top as `-ml-1.5` overlaps the stack.
              <div
                key={integration}
                className="flex h-6 w-6 items-center justify-center rounded-full border-2 border-background bg-muted text-[8px] font-bold text-muted-foreground first:ml-0 -ml-1.5"
                style={{ zIndex: integrations.length - i }}
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
