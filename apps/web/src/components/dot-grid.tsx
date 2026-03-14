import { cn } from "@/lib/utils";

export function DotGrid({ rows, cols, className }: { rows: number; cols: number; className?: string }) {
  return (
    <div className={cn("flex flex-col gap-5 opacity-25", className)}>
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex gap-5">
          {Array.from({ length: cols }).map((_, c) => (
            <div key={c} className="size-1.5 shrink-0 bg-primary" />
          ))}
        </div>
      ))}
    </div>
  );
}
