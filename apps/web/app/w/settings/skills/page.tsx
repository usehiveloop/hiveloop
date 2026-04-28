import { SettingsShell } from "@/components/settings-shell"
import { SkillsSettings } from "../_components/skills-settings"

export default function Page() {
  return (
    <SettingsShell
      title="Skills"
      description="Reusable capabilities you can attach to agents."
      dividers={false}
    >
      <SkillsSettings />
    </SettingsShell>
  )
}
