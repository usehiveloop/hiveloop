"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export default function SettingsGeneralPage() {
  return (
    <div className="flex flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
      {/* Workspace Name */}
      <div className="flex flex-col gap-4 border border-border bg-card p-5">
        <div className="flex flex-col gap-1">
          <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
            Workspace Name
          </span>
          <p className="text-[13px] text-muted-foreground">
            The display name for this workspace.
          </p>
        </div>
        <Input defaultValue="Acme Corp" className="max-w-sm text-[13px]" />
        <div className="flex justify-end">
          <Button size="lg">Save</Button>
        </div>
      </div>

      {/* Workspace ID */}
      <div className="flex flex-col gap-4 border border-border bg-card p-5">
        <div className="flex flex-col gap-1">
          <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
            Workspace ID
          </span>
          <p className="text-[13px] text-muted-foreground">
            Used to identify this workspace in API calls.
          </p>
        </div>
        <Input
          readOnly
          value="ws_acme_prod_a8f2e3"
          className="max-w-sm font-mono text-[13px] text-muted-foreground"
        />
      </div>

      {/* Danger Zone */}
      <div className="flex flex-col gap-4 border border-destructive/20 bg-card p-5">
        <div className="flex flex-col gap-1">
          <span className="text-[11px] font-semibold uppercase tracking-wider text-destructive">
            Danger Zone
          </span>
          <p className="text-[13px] text-muted-foreground">
            Permanently delete this workspace and all associated data. This action cannot be undone.
          </p>
        </div>
        <div className="flex justify-end">
          <Button variant="destructive" size="lg">
            Delete Workspace
          </Button>
        </div>
      </div>
    </div>
  );
}
