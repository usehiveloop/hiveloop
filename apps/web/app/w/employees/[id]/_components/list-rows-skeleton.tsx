import { Skeleton } from "@/components/ui/skeleton"

export function ListRowsSkeleton() {
  return (
    <div className="grid gap-2" aria-busy="true">
      {Array.from({ length: 3 }).map((_, index) => (
        <Skeleton key={index} className="h-[58px] rounded-xl" />
      ))}
    </div>
  )
}
