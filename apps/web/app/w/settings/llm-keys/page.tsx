import { SettingsShell } from "@/components/settings-shell"
import { LlmKeysSettings } from "../_components/llm-keys-settings"

export default function Page() {
  return (
    <SettingsShell
      title="LLM keys"
      description="Provider credentials your agents use to call models."
      dividers={false}
    >
      <LlmKeysSettings />
    </SettingsShell>
  )
}
