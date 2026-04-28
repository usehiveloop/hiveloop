import { SettingsShell } from "@/components/settings-shell"
import { SecuritySettings } from "../_components/security-settings"

export default function Page() {
  return (
    <SettingsShell
      title="Security"
      description="Two-factor auth, active sessions, and audit log."
      dividers={false}
    >
      <SecuritySettings />
    </SettingsShell>
  )
}
