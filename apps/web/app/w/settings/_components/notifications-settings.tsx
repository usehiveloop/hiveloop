"use client"

export function NotificationsSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Email notifications</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Configure which events trigger email notifications.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">In-app notifications</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage your in-app notification preferences.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Agent alerts</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Get notified when agents complete runs, encounter errors, or need attention.
        </p>
      </div>
    </div>
  )
}
