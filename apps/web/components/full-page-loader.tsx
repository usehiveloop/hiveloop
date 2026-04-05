import { Loader } from "@/components/loader"

export function FullPageLoader() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <Loader description="Setting up your workspace" />
    </div>
  )
}
