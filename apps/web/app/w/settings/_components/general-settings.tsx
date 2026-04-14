"use client"

export function GeneralSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Workspace name</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          The name of your workspace visible to all members.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Workspace URL</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Your workspace&apos;s unique URL identifier.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Timezone</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Set the default timezone for your workspace.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground text-destructive">Delete workspace</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Permanently delete this workspace and all its data. This action cannot be undone.
        </p>
      </div>
    </div>
  )
}
