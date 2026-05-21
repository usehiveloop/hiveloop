"use client"

import { useState } from "react"
import {
  Theme,
  ROSE_THEME,
  Button,
  Switch,
  Card,
  Badge,
  Ghost,
} from "../exploration-2/_components/design-system"

function Section({
  title,
  children,
  theme,
}: {
  title: string
  children: React.ReactNode
  theme: Theme
}) {
  return (
    <section className="mb-16">
      <h2
        className="font-recoleta mb-8 text-2xl font-medium tracking-tight"
        style={{ color: theme.text }}
      >
        {title}
      </h2>
      {children}
    </section>
  )
}

function SubSection({
  title,
  children,
  theme,
}: {
  title: string
  children: React.ReactNode
  theme: Theme
}) {
  return (
    <div className="mb-8">
      <h3
        className="mb-4 text-sm font-semibold uppercase tracking-wide"
        style={{ color: theme.muted }}
      >
        {title}
      </h3>
      {children}
    </div>
  )
}

export default function DesignSystemPage() {
  const theme = ROSE_THEME
  const [switchOn, setSwitchOn] = useState(true)
  const [switchOff, setSwitchOff] = useState(false)

  return (
    <main
      className="min-h-screen w-full px-6 py-16 sm:px-12 lg:px-24"
      style={{ backgroundColor: theme.bg }}
    >
      {/* Header */}
      <div className="mb-16">
        <h1
          className="font-recoleta text-4xl font-normal tracking-tight sm:text-5xl"
          style={{ color: theme.text }}
        >
          Hivy Design System
        </h1>
        <p className="mt-3 text-lg" style={{ color: theme.muted }}>
          Reusable components extracted from exploration-2
        </p>
      </div>

      {/* Buttons */}
      <Section title="Buttons" theme={theme}>
        <div className="grid gap-8">
          <SubSection title="Variants" theme={theme}>
            <div className="flex flex-wrap items-center gap-4">
              <Button variant="primary" theme={theme}>
                Primary
              </Button>
              <Button variant="secondary" theme={theme}>
                Secondary
              </Button>
              <Button variant="ghost" theme={theme}>
                Ghost
              </Button>
              <Button variant="outline" theme={theme}>
                Outline
              </Button>
            </div>
          </SubSection>

          <SubSection title="Sizes" theme={theme}>
            <div className="flex flex-wrap items-center gap-4">
              <Button variant="primary" size="sm" theme={theme}>
                Small
              </Button>
              <Button variant="primary" size="md" theme={theme}>
                Medium
              </Button>
              <Button variant="primary" size="lg" theme={theme}>
                Large
              </Button>
            </div>
          </SubSection>

          <SubSection title="As Link" theme={theme}>
            <div className="flex flex-wrap items-center gap-4">
              <Button variant="primary" theme={theme} href="#">
                Link Button
              </Button>
              <Button variant="secondary" theme={theme} href="#">
                Secondary Link
              </Button>
            </div>
          </SubSection>

          <SubSection title="Disabled" theme={theme}>
            <div className="flex flex-wrap items-center gap-4">
              <Button variant="primary" theme={theme} disabled>
                Disabled Primary
              </Button>
              <Button variant="secondary" theme={theme} disabled>
                Disabled Secondary
              </Button>
            </div>
          </SubSection>
        </div>
      </Section>

      {/* Switches */}
      <Section title="Switches" theme={theme}>
        <div className="grid gap-8">
          <SubSection title="States" theme={theme}>
            <div className="flex flex-col gap-4 max-w-xs">
              <div
                className="flex items-center justify-between rounded-lg border px-4 py-3"
                style={{
                  backgroundColor: theme.bg,
                  borderColor: theme.secondaryBorder,
                }}
              >
                <span className="text-sm font-medium" style={{ color: theme.text }}>
                  Notifications
                </span>
                <Switch
                  checked={switchOn}
                  onChange={setSwitchOn}
                  theme={theme}
                />
              </div>
              <div
                className="flex items-center justify-between rounded-lg border px-4 py-3"
                style={{
                  backgroundColor: theme.bg,
                  borderColor: theme.secondaryBorder,
                }}
              >
                <span className="text-sm font-medium" style={{ color: theme.text }}>
                  Dark mode
                </span>
                <Switch
                  checked={switchOff}
                  onChange={setSwitchOff}
                  theme={theme}
                />
              </div>
            </div>
          </SubSection>

          <SubSection title="With Labels" theme={theme}>
            <div className="flex flex-col gap-4 max-w-xs">
              <Switch
                checked={switchOn}
                onChange={setSwitchOn}
                theme={theme}
                label="Read access"
              />
              <Switch
                checked={switchOff}
                onChange={setSwitchOff}
                theme={theme}
                label="Write access"
              />
              <Switch
                checked={false}
                onChange={() => {}}
                theme={theme}
                label="Admin (disabled)"
                disabled
              />
            </div>
          </SubSection>
        </div>
      </Section>

      {/* Cards */}
      <Section title="Cards" theme={theme}>
        <div className="grid gap-8">
          <SubSection title="Variants" theme={theme}>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Card theme={theme}>
                <h4
                  className="font-recoleta text-lg font-medium"
                  style={{ color: theme.text }}
                >
                  Default Card
                </h4>
                <p className="mt-2 text-sm" style={{ color: theme.muted }}>
                  Standard card with border and background.
                </p>
              </Card>
              <Card theme={theme}>
                <h4
                  className="font-recoleta text-lg font-medium"
                  style={{ color: theme.text }}
                >
                  Standard Card
                </h4>
                <p className="mt-2 text-sm" style={{ color: theme.muted }}>
                  Standard card with border and background.
                </p>
              </Card>
              <Card theme={theme} padding="sm">
                <h4
                  className="font-recoleta text-lg font-medium"
                  style={{ color: theme.text }}
                >
                  Small Padding
                </h4>
                <p className="mt-2 text-sm" style={{ color: theme.muted }}>
                  Card with reduced padding.
                </p>
              </Card>
            </div>
          </SubSection>
        </div>
      </Section>

      {/* Badges */}
      <Section title="Badges" theme={theme}>
        <div className="grid gap-8">
          <SubSection title="Variants" theme={theme}>
            <div className="flex flex-wrap items-center gap-4">
              <Badge theme={theme}>Default</Badge>
              <Badge theme={theme} variant="outline">
                Outline
              </Badge>
              <Badge theme={theme} variant="dot">
                With dot
              </Badge>
            </div>
          </SubSection>
        </div>
      </Section>

      {/* Ghost Icon */}
      <Section title="Ghost Icon" theme={theme}>
        <div className="grid gap-8">
          <SubSection title="Sizes" theme={theme}>
            <div className="flex flex-wrap items-end gap-6">
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.pillFrom} bgColor={theme.bg} size={32} />
                <span className="text-xs" style={{ color: theme.muted }}>32px</span>
              </div>
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.pillFrom} bgColor={theme.bg} size={48} />
                <span className="text-xs" style={{ color: theme.muted }}>48px</span>
              </div>
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.pillFrom} bgColor={theme.bg} size={64} />
                <span className="text-xs" style={{ color: theme.muted }}>64px</span>
              </div>
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.pillFrom} bgColor={theme.bg} size={96} />
                <span className="text-xs" style={{ color: theme.muted }}>96px</span>
              </div>
            </div>
          </SubSection>

          <SubSection title="Colors" theme={theme}>
            <div className="flex flex-wrap items-end gap-6">
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.pillFrom} bgColor={theme.bg} size={48} />
                <span className="text-xs" style={{ color: theme.muted }}>pillFrom</span>
              </div>
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.pillVia} bgColor={theme.bg} size={48} />
                <span className="text-xs" style={{ color: theme.muted }}>pillVia</span>
              </div>
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.pillTo} bgColor={theme.bg} size={48} />
                <span className="text-xs" style={{ color: theme.muted }}>pillTo</span>
              </div>
              <div className="flex flex-col items-center gap-2">
                <Ghost color={theme.text} bgColor={theme.bg} size={48} />
                <span className="text-xs" style={{ color: theme.muted }}>text</span>
              </div>
            </div>
          </SubSection>
        </div>
      </Section>

      {/* Compositions */}
      <Section title="Compositions" theme={theme}>
        <div className="grid gap-8">
          <SubSection title="Card with Actions" theme={theme}>
            <div className="max-w-md">
              <Card theme={theme}>
                <div className="flex items-start justify-between">
                  <div>
                    <h4
                      className="font-recoleta text-lg font-medium"
                      style={{ color: theme.text }}
                    >
                      Integration Settings
                    </h4>
                    <p className="mt-1 text-sm" style={{ color: theme.muted }}>
                      Manage your connected apps and permissions.
                    </p>
                  </div>
                  <Badge theme={theme}>Active</Badge>
                </div>
                <div className="mt-6 flex flex-col gap-3">
                  <Switch
                    checked={switchOn}
                    onChange={setSwitchOn}
                    theme={theme}
                    label="Google Sheets"
                  />
                  <Switch
                    checked={switchOff}
                    onChange={setSwitchOff}
                    theme={theme}
                    label="Slack"
                  />
                </div>
                <div className="mt-6 flex gap-3">
                  <Button variant="primary" size="sm" theme={theme}>
                    Save
                  </Button>
                  <Button variant="ghost" size="sm" theme={theme}>
                    Cancel
                  </Button>
                </div>
              </Card>
            </div>
          </SubSection>

          <SubSection title="Feature Card" theme={theme}>
            <div className="max-w-sm">
              <Card theme={theme} padding="lg">
                <div className="mb-4 inline-flex items-center justify-center rounded-xl p-3"
                  style={{ backgroundColor: theme.pillFrom + "15" }}
                >
                  <Ghost color={theme.pillFrom} bgColor={theme.bg} size={32} />
                </div>
                <h4
                  className="font-recoleta text-xl font-medium"
                  style={{ color: theme.text }}
                >
                  Your AI Coworker
                </h4>
                <p className="mt-2 text-sm leading-relaxed" style={{ color: theme.muted }}>
                  Hivy connects to your tools, understands your work, and completes tasks across your team.
                </p>
                <div className="mt-6">
                  <Button variant="secondary" size="sm" theme={theme} href="#">
                    Learn more
                  </Button>
                </div>
              </Card>
            </div>
          </SubSection>
        </div>
      </Section>
    </main>
  )
}
