import { Skeleton } from "@/components/ui/skeleton"

const ROW_COUNT = 6

export function AgentsSkeleton() {
  return (
    <div
      className="mx-auto w-full max-w-4xl px-6 py-10"
      aria-busy="true"
      aria-label="Loading agents"
    >
      {/* Search */}
      <div className="mb-6">
        <Skeleton className="h-9 w-full max-w-sm rounded-md" />
      </div>

      <div className="flex flex-col gap-2">
        {/* Column header strip — real text */}
        <div className="flex items-center gap-3 px-4 py-1 font-mono text-[10px] uppercase tracking-[1px] text-muted-foreground/50">
          <span className="min-w-0 flex-1">Name</span>
          <span className="w-24 shrink-0">Integrations</span>
          <span className="w-6 shrink-0" />
          <span className="w-8 shrink-0" />
        </div>

        {/* Rows */}
        {Array.from({ length: ROW_COUNT }).map((_, i) => (
          <div
            key={i}
            className="flex items-center gap-3 rounded-xl border border-border px-4 py-2.5"
          >
            <div className="flex min-w-0 flex-1 items-center gap-3">
              <Skeleton className="size-6 shrink-0 rounded-md" />
              <Skeleton
                className="h-4 rounded-md"
                style={{ width: `${40 + ((i * 13) % 30)}%` }}
              />
            </div>
            <div className="flex w-24 shrink-0 items-center">
              {[0, 1, 2].map((j) => (
                <Skeleton
                  key={j}
                  className="size-5 rounded-full border-2 border-background"
                  style={{ marginLeft: j === 0 ? 0 : -6 }}
                />
              ))}
            </div>
            <div className="flex w-6 shrink-0 justify-center">
              <Skeleton className="size-2 rounded-full" />
            </div>
            <div className="flex w-8 shrink-0 justify-center">
              <Skeleton className="size-7 rounded-md" />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
