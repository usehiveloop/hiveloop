import { SettingsShell } from "@/components/settings-shell"
import { ProfileSettings } from "../_components/profile-settings"

export default function Page() {
  return (
    <SettingsShell
      title="Profile"
      description="Your name, email, and avatar."
      dividers={false}
    >
      <ProfileSettings />
    </SettingsShell>
  )
}
