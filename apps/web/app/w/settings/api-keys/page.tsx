import { SettingsShell } from "@/components/settings-shell"
import { ApiKeysSettings } from "../_components/api-keys-settings"

export default function Page() {
  return (
    <SettingsShell
      title="API keys"
      description="Tokens for programmatic access to this workspace."
      dividers={false}
    >
      <ApiKeysSettings />
    </SettingsShell>
  )
}
