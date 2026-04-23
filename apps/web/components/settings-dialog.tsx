"use client"

import * as React from "react"
import { useState } from "react"
import { motion } from "motion/react"
import { cn } from "@/lib/utils"
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  UserCircleIcon,
  UserGroupIcon,
  CreditCardIcon,
  Notification01Icon,
  Key01Icon,
  ShieldKeyIcon,
  Settings01Icon,
  ArrowDown01Icon,
  ArtificialIntelligence01Icon,
  ContainerIcon,
  BookOpen01Icon,
} from "@hugeicons/core-free-icons"

import { GeneralSettings } from "@/app/w/settings/_components/general-settings"
import { ProfileSettings } from "@/app/w/settings/_components/profile-settings"
import { MembersSettings } from "@/app/w/settings/_components/members-settings"
import { BillingSettings } from "@/app/w/settings/_components/billing-settings"
import { NotificationsSettings } from "@/app/w/settings/_components/notifications-settings"
import { LlmKeysSettings } from "@/app/w/settings/_components/llm-keys-settings"
import { ApiKeysSettings } from "@/app/w/settings/_components/api-keys-settings"
import { SkillsSettings } from "@/app/w/settings/_components/skills-settings"
import { SecuritySettings } from "@/app/w/settings/_components/security-settings"
import { SandboxTemplatesList } from "@/app/w/settings/_components/sandbox-templates"

interface SettingsItem {
  id: string
  label: string
  icon: React.ComponentProps<typeof HugeiconsIcon>["icon"]
}

interface SettingsGroup {
  label: string
  items: SettingsItem[]
}

const settingsGroups: SettingsGroup[] = [
  {
    label: "Workspace",
    items: [
      { id: "general", label: "General", icon: Settings01Icon },
      { id: "members", label: "Members", icon: UserGroupIcon },
      { id: "billing", label: "Billing", icon: CreditCardIcon },
    ],
  },
  {
    label: "Account",
    items: [
      { id: "profile", label: "Profile", icon: UserCircleIcon },
      { id: "notifications", label: "Notifications", icon: Notification01Icon },
      { id: "security", label: "Security", icon: ShieldKeyIcon },
    ],
  },
  {
    label: "Developer",
    items: [
      { id: "llm-keys", label: "LLM Keys", icon: ArtificialIntelligence01Icon },
      { id: "api-keys", label: "API Keys", icon: Key01Icon },
      { id: "skills", label: "Skills", icon: BookOpen01Icon },
      { id: "sandboxes", label: "Sandboxes", icon: ContainerIcon },
    ],
  },
]

const allItems = settingsGroups.flatMap((group) => group.items)

const sectionComponents: Record<string, React.ComponentType> = {
  general: GeneralSettings,
  profile: ProfileSettings,
  members: MembersSettings,
  billing: BillingSettings,
  notifications: NotificationsSettings,
  "llm-keys": LlmKeysSettings,
  "api-keys": ApiKeysSettings,
  skills: SkillsSettings,
  security: SecuritySettings,
  sandboxes: SandboxTemplatesList,
}

interface SettingsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialSection?: string
}

export function SettingsDialog({ open, onOpenChange, initialSection }: SettingsDialogProps) {
  const [activeSection, setActiveSection] = useState(initialSection ?? "general")

  React.useEffect(() => {
    if (open && initialSection) {
      setActiveSection(initialSection)
    }
  }, [open, initialSection])

  const ActiveComponent = sectionComponents[activeSection]
  const activeLabel = allItems.find((item) => item.id === activeSection)?.label ?? "Settings"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton
        className="max-w-[100vw] h-dvh rounded-none p-0 gap-0 overflow-hidden md:max-w-5xl md:h-160 md:rounded-4xl"
      >
        <DialogTitle className="sr-only md:hidden">Settings</DialogTitle>
        <div className="flex flex-col md:flex-row h-full">
          <div className="flex md:hidden shrink-0 flex-col border-b border-border">
            <div className="flex items-center px-4 pt-4 pb-2">
              <h2 className="font-mono uppercase text-muted-foreground text-sm font-medium">Settings</h2>
            </div>
            <div className="px-4 pb-3 my-4">
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={<Button variant="outline" className="w-full justify-between" />}
                >
                  <span className="flex items-center gap-2">
                    <HugeiconsIcon icon={allItems.find((item) => item.id === activeSection)?.icon ?? Settings01Icon} size={14} />
                    {activeLabel}
                  </span>
                  <HugeiconsIcon icon={ArrowDown01Icon} size={14} className="text-muted-foreground" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="min-w-[calc(100vw-2rem)]">
                  <DropdownMenuRadioGroup
                    value={activeSection}
                    onValueChange={(value) => setActiveSection(value)}
                  >
                    {settingsGroups.map((group, index) => (
                      <DropdownMenuGroup key={group.label}>
                        <DropdownMenuLabel>{group.label}</DropdownMenuLabel>
                        {group.items.map((item) => (
                          <DropdownMenuRadioItem key={item.id} value={item.id}>
                            <HugeiconsIcon icon={item.icon} size={14} />
                            {item.label}
                          </DropdownMenuRadioItem>
                        ))}
                        {index < settingsGroups.length - 1 && <DropdownMenuSeparator />}
                      </DropdownMenuGroup>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          <nav className="hidden md:flex w-52 shrink-0 flex-col gap-4 border-r border-border bg-muted/30 p-3">
            {settingsGroups.map((group) => (
              <div key={group.label} className="flex flex-col gap-1">
                <p className="px-2.5 pb-1 font-mono text-2xs uppercase tracking-medium text-muted-foreground">
                  {group.label}
                </p>
                {group.items.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setActiveSection(item.id)}
                    className={cn(
                      "relative flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-sm transition-colors",
                      activeSection === item.id
                        ? "text-foreground"
                        : "text-muted-foreground hover:bg-background/50 hover:text-foreground"
                    )}
                  >
                    {activeSection === item.id && (
                      <motion.div
                        layoutId="settings-nav-active"
                        className="absolute inset-0 rounded-xl bg-background shadow-sm ring-1 ring-border"
                        transition={{ type: "spring", bounce: 0.15, duration: 0.4 }}
                      />
                    )}
                    <span className="relative flex items-center gap-2.5">
                      <HugeiconsIcon icon={item.icon} size={16} />
                      {item.label}
                    </span>
                  </button>
                ))}
              </div>
            ))}
          </nav>

          <div className="flex flex-1 flex-col overflow-hidden">
            <div className="hidden md:block shrink-0 border-b border-border px-6 py-4">
              <h2 className="font-heading text-base font-medium">{activeLabel}</h2>
            </div>
            <div className="flex-1 overflow-y-auto px-4 py-4 md:px-6 md:py-5">
              <ActiveComponent />
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
