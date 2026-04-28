import { Button } from "@/components/ui/button"
import { SettingsShell } from "@/components/settings-shell"
import { WorkspaceLogoField } from "./_components/workspace-logo-field"
import { WorkspaceNameField } from "./_components/workspace-name-field"

export default function Page() {
  return (
    <SettingsShell
      title="General"
      description="Workspace details visible to every member."
    >
      <WorkspaceNameField />

      <WorkspaceLogoField />

      <section>
        <h2 className="text-[13px] font-medium">Delete workspace</h2>
        <div className="mt-2 flex items-start justify-between gap-6">
          <p className="text-[12px] text-muted-foreground">
            Permanently remove this workspace, all agents, and run history. This
            cannot be undone.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="shrink-0 border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
          >
            Delete workspace
          </Button>
        </div>
      </section>
    </SettingsShell>
  )
}
