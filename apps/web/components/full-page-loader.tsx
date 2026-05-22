import { AuthGhostLogo } from "@/components/auth-ghost-logo"

interface FullPageLoaderProps {
  title?: string
  description?: string
}

export function FullPageLoader({
  title,
  description = "Setting up your workspace",
}: FullPageLoaderProps) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <AuthGhostLogo
        title={title}
        description={description}
        logoClassName="h-16 w-16"
      />
    </div>
  )
}
