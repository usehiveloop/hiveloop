import { cn } from "@/lib/utils";

interface TableSkeletonProps {
  columns: { width?: string; align?: "left" | "right" }[];
  rows?: number;
  className?: string;
}

function ShimmerBlock({ className }: { className?: string }) {
  return <div className={cn("animate-pulse rounded bg-secondary", className)} />;
}

export function TableSkeleton({ columns, rows = 5, className }: TableSkeletonProps) {
  return (
    <>
      {/* Mobile skeleton */}
      <div className={cn("flex flex-col gap-3 md:hidden", className)}>
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} className="flex flex-col gap-3 border border-border bg-card p-4">
            <div className="flex items-start justify-between">
              <div className="flex flex-col gap-1.5">
                <ShimmerBlock className="h-3.5 w-32" />
                <ShimmerBlock className="h-3 w-24" />
              </div>
              <ShimmerBlock className="h-5 w-14 rounded-full" />
            </div>
            <div className="flex items-center gap-3">
              <ShimmerBlock className="h-5 w-16 rounded-full" />
              <ShimmerBlock className="h-3 w-20" />
            </div>
            <div className="flex items-center justify-between">
              <ShimmerBlock className="h-3 w-16" />
              <ShimmerBlock className="h-3 w-24" />
            </div>
          </div>
        ))}
      </div>

      {/* Desktop skeleton */}
      <div className={cn("hidden md:block", className)}>
        {/* Header */}
        <div className="flex border-b border-border px-4 py-2.5">
          {columns.map((col, i) => (
            <div key={i} style={col.width ? { width: col.width } : { flex: 1 }} className="px-4">
              <ShimmerBlock className="h-3 w-16" />
            </div>
          ))}
        </div>
        {/* Rows */}
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} className="flex border-b border-border px-4 py-3">
            {columns.map((col, j) => (
              <div key={j} style={col.width ? { width: col.width } : { flex: 1 }} className="flex items-center px-4">
                <ShimmerBlock
                  className={cn(
                    "h-3.5",
                    j === 0 ? "w-28" : j === columns.length - 1 ? "w-12" : "w-16",
                  )}
                />
              </div>
            ))}
          </div>
        ))}
      </div>
    </>
  );
}
