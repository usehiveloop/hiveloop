import { Loader } from "@/components/loader"

interface PageLoaderProps {
  title?: string
  description?: string
}

export function PageLoader({ title, description }: PageLoaderProps) {
  return (
    <div className="flex items-center justify-center px-4 py-24">
      <Loader title={title} description={description} />
    </div>
  )
}
