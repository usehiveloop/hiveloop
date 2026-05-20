export interface ChangelogAnnouncement {
  title: string
  date: string
  summary: string
  href?: string
}

export const changelogAnnouncements: ChangelogAnnouncement[] = [
  {
    title: "Hivy is now the default workspace employee",
    date: "2026-05-20",
    summary:
      "Every workspace now starts with Hivy, one managed employee with access to your connected tools and installed skills.",
  },
  {
    title: "Slack-first onboarding",
    date: "2026-05-20",
    summary:
      "New workspaces set up Slack first, invite Hivy into channels, and can add more tools immediately after setup.",
  },
  {
    title: "Skills moved into the main workspace",
    date: "2026-05-20",
    summary:
      "Skills now have a dedicated workspace page for browsing, creating, installing, and removing Hivy capabilities.",
  },
]

