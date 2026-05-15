import Image from "next/image"
import { cn } from "@/lib/utils"

const LOGO_BASE = "https://connections.usehiveloop.com/images/template-logos"

const LOGO_PROVIDER_ALIASES: Record<string, string> = {
  bugsink: "sentry",
  "github-app-code-reviews": "github-app",
}

export function integrationLogoURL(provider: string): string {
  const aliased = LOGO_PROVIDER_ALIASES[provider] ?? provider
  return `${LOGO_BASE}/${aliased}.svg`
}

const sizeClasses: Record<number, string> = {
  16: "size-4",
  20: "size-5",
  24: "size-6",
  28: "size-7",
  32: "size-8",
  40: "size-10",
  48: "size-12",
}

interface IntegrationLogoProps {
  provider: string
  size?: number
  className?: string
}

export function IntegrationLogo({ provider, size = 32, className }: IntegrationLogoProps) {
  const sizeClass = sizeClasses[size] ?? "size-8"

  return (
    <div className={cn("shrink-0 rounded-md bg-white p-0.5", sizeClass, className)}>
      <Image
        src={integrationLogoURL(provider)}
        alt={provider}
        width={size}
        height={size}
        className="size-full object-contain"
      />
    </div>
  )
}
