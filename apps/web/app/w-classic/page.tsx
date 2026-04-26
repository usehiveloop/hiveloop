import Link from "next/link"

export default function ClassicWorkspacePage() {
  return (
    <div className="mx-auto flex max-w-md flex-col items-center justify-center gap-3 px-6 py-24 text-center">
      <p className="font-mono text-[11px] uppercase tracking-[1.5px] text-muted-foreground">
        Classic layout (archived)
      </p>
      <h1 className="text-lg font-medium text-foreground">
        This is the previous workspace layout.
      </h1>
      <p className="text-sm text-muted-foreground">
        Kept for reference. The active workspace lives at{" "}
        <Link href="/w" className="text-foreground underline-offset-4 hover:underline">
          /w
        </Link>
        .
      </p>
    </div>
  )
}
