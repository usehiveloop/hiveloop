import { SettingsShell } from "@/components/settings-shell"
import { SandboxTemplatesList } from "../_components/sandbox-templates"

export default function Page() {
  return (
    <SettingsShell
      title="Sandboxes"
      description="Reusable environments your agents can boot into."
      dividers={false}
    >
      <SandboxTemplatesList />
    </SettingsShell>
  )
}
