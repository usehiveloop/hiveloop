import { PageHeader } from "@/components/page-header"

export default function KnowledgePage() {
  return (
    <>
      <PageHeader title="Knowledge" />
      <div className="mx-auto flex w-full max-w-md flex-col items-center justify-center gap-2 px-6 py-24 text-center">
        <h1 className="text-lg font-medium text-foreground">Coming soon.</h1>
        <p className="text-sm text-muted-foreground">
          A place to manage knowledge sources for your agents.
        </p>
      </div>
    </>
  )
}
