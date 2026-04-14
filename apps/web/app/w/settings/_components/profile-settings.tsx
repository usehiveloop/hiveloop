"use client"

export function ProfileSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Display name</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Your name as displayed to other members.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Email address</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          The email address associated with your account.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Avatar</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Upload a profile picture to personalize your account.
        </p>
      </div>
    </div>
  )
}
