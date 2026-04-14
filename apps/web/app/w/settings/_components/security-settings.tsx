"use client"

export function SecuritySettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Two-factor authentication</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Add an extra layer of security to your account.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Active sessions</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          View and manage your active sessions across devices.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Audit log</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Review a log of actions taken in your workspace.
        </p>
      </div>
    </div>
  )
}
