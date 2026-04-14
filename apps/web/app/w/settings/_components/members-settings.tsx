"use client"

export function MembersSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Team members</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage who has access to this workspace.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Pending invitations</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          View and manage pending workspace invitations.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Roles</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Configure roles and permissions for workspace members.
        </p>
      </div>
    </div>
  )
}
