export function LogoMark({ className = "h-6 w-6" }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 64 64"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      <path
        d="M 33 26 C 40 20, 52 18, 55 29 C 58 42, 44 53, 32 52 C 18 50, 9 37, 13 24"
        stroke="#ED462D"
        strokeWidth="7.5"
        fill="none"
        strokeLinecap="round"
      />
    </svg>
  )
}

export function Logo({ className = "h-8" }: { className?: string }) {
  return (
    <div className={`flex items-center gap-2.5 ${className}`}>
      <LogoMark className="h-full w-auto shrink-0" />
      <span className="font-semibold tracking-tight text-foreground text-lg font-wordmark">
        hiveloop
      </span>
    </div>
  )
}
