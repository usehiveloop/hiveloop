import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { SettingsShell } from "@/components/settings-shell"
import { Switch } from "@/components/ui/switch"

export default function Page() {
  return (
    <SettingsShell
      title="General"
      description="Workspace details visible to every member."
    >
      <section className="flex flex-col gap-2.5">
        <div>
          <Label className="text-[13px] font-medium">Workspace name</Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Used in URLs, invitations, and email subject lines.
          </p>
        </div>
        <Input defaultValue="Acme Inc" className="max-w-sm" />
      </section>

      <section className="flex flex-col gap-2.5">
        <div>
          <Label className="text-[13px] font-medium">Workspace URL</Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Changing this URL redirects existing links for 30 days.
          </p>
        </div>
        <div className="flex max-w-sm items-center rounded-md border border-input bg-transparent text-sm focus-within:ring-2 focus-within:ring-ring/50">
          <span className="select-none border-r border-input/60 px-3 py-2 text-muted-foreground">
            hiveloop.com/
          </span>
          <input
            defaultValue="acme"
            className="min-w-0 flex-1 bg-transparent py-2 pr-3 pl-2 outline-none placeholder:text-muted-foreground"
          />
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <div>
          <Label className="text-[13px] font-medium">Workspace logo</Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Square. PNG or SVG, at least 256 × 256.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex size-11 items-center justify-center rounded-md bg-muted font-mono text-[13px] font-medium text-muted-foreground">
            AI
          </div>
          <Button variant="outline" size="sm">
            Upload
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-muted-foreground hover:text-foreground"
          >
            Remove
          </Button>
        </div>
      </section>

      <section className="flex items-start justify-between gap-6">
        <div className="flex-1">
          <Label className="text-[13px] font-medium">Public agent gallery</Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Allow members to publish agents from this workspace to the public gallery.
          </p>
        </div>
        <Switch defaultChecked className="mt-0.5" />
      </section>

      <section className="flex items-start justify-between gap-6">
        <div className="flex-1">
          <Label className="text-[13px] font-medium">
            Require two-factor authentication
          </Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Members without 2FA enabled will be prompted on next sign-in.
          </p>
        </div>
        <Switch className="mt-0.5" />
      </section>

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
