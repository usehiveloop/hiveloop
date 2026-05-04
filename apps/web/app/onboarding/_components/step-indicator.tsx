import { cn } from "@/lib/utils"

export function StepIndicator({ total, currentIndex }: { total: number; currentIndex: number }) {
  return (
    <ol className="flex items-center gap-2" aria-label="Onboarding progress">
      {Array.from({ length: total }).map((_, idx) => {
        const isDone = idx < currentIndex
        const isActive = idx === currentIndex
        return (
          <li
            key={idx}
            aria-current={isActive ? "step" : undefined}
            className={cn(
              "h-1.5 rounded-full transition-all",
              isActive && "w-10 bg-primary",
              isDone && "w-6 bg-primary/70",
              !isActive && !isDone && "w-6 bg-muted",
            )}
          />
        )
      })}
    </ol>
  )
}
