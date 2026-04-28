import { SettingsShell } from "@/components/settings-shell"
import { NotificationsSettings } from "../_components/notifications-settings"

export default function Page() {
  return (
    <SettingsShell
      title="Notifications"
      description="Where and when we ping you."
      dividers={false}
    >
      <NotificationsSettings />
    </SettingsShell>
  )
}
