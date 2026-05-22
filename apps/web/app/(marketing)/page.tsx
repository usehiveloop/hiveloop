"use client"

import { useState, useRef, useEffect } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { AnimatePresence, motion } from "motion/react"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Badge } from "@/components/ui/badge"
import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
} from "@/components/ui/navigation-menu"
import {
  GithubIcon,
  SlackIcon,
  FigmaIcon,
  VercelIcon,
  StripeIcon,
  GoogleDriveIcon,
  GoogleExcelIcon,
  TrelloIcon,
  SentryIcon,
  AwsIcon,
  PostgresIcon,
  GoogleCloudIcon,
  GoogleChromeIcon,
  TiktokIcon,
  ClickupIcon,
  InstagramIcon,
  PosthogIcon,
  GoogleAnalyticsIcon,
  GoogleCalendarIcon,
  PhotoshopIcon,
  GoogleSlidesIcon,
} from "@/components/icons"
import { MarketingFooter } from "./_components/footer"

/* ─────────────────────────── Navbar ─────────────────────────── */

function Navbar() {
  const pathname = usePathname()
  const isActive = (href: string) => pathname === href

  return (
    <NavigationMenu viewport={false} className="hidden items-center md:flex">
      <nav className="flex h-11 items-center rounded-full border border-[var(--nav-border)] bg-[var(--nav-bg)] px-2 backdrop-blur-lg">
        <NavigationMenuList>
          <NavigationMenuItem>
            <NavigationMenuLink
              href="#"
              className={isActive("/") ? "bg-black/[0.03] text-foreground dark:bg-white/[0.05]" : undefined}
            >
              Product
            </NavigationMenuLink>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuTrigger>Resources</NavigationMenuTrigger>
            <NavigationMenuContent>
              <ul className="grid w-44 gap-0.5 p-1.5">
                <li>
                  <NavigationMenuLink href="#">Blog</NavigationMenuLink>
                </li>
                <li>
                  <NavigationMenuLink href="#">Changelog</NavigationMenuLink>
                </li>
                <li>
                  <NavigationMenuLink href="#">Docs</NavigationMenuLink>
                </li>
              </ul>
            </NavigationMenuContent>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuTrigger>Solutions</NavigationMenuTrigger>
            <NavigationMenuContent>
              <ul className="grid w-44 gap-0.5 p-1.5">
                <li>
                  <NavigationMenuLink href="#">Use cases</NavigationMenuLink>
                </li>
                <li>
                  <NavigationMenuLink href="#">Integrations</NavigationMenuLink>
                </li>
              </ul>
            </NavigationMenuContent>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuLink
              asChild
              className={isActive("/pricing") ? "bg-black/[0.03] text-foreground dark:bg-white/[0.05]" : undefined}
            >
              <Link href="/pricing">Pricing</Link>
            </NavigationMenuLink>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuLink
              href="https://github.com/usehivy/hivy"
              target="_blank"
              rel="noopener noreferrer"
              className="ml-1 inline-flex flex-row items-center gap-1.5"
            >
              <GithubIcon size={16} />
              <span>2.4k</span>
            </NavigationMenuLink>
          </NavigationMenuItem>
        </NavigationMenuList>
      </nav>
    </NavigationMenu>
  )
}

/* ─────────────────────────── Announcement Pill ─────────────────────────── */

function AnnouncementPill() {
  return (
    <Badge variant="dot">
      <span className="relative flex h-2 w-2">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--pill-from)] opacity-75" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--pill-from)]" />
      </span>
      Meet Hivy — your AI coworker for busy teams
    </Badge>
  )
}

/* ─────────────────────────── Ghost ─────────────────────────── */

function Ghost({
  color,
  bgColor,
  size = 64,
  className = "",
}: {
  color: string
  bgColor: string
  size?: number
  className?: string
}) {
  return (
    <svg
      viewBox="0 0 640 640"
      width={size}
      height={size}
      style={{ color }}
      fill="currentColor"
      className={className}
    >
      <path
        d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z"
        fill="currentColor"
      />
      <ellipse cx="318.5" cy="282" rx="45.5" ry="101" fill={bgColor} />
      <ellipse cx="457.5" cy="282" rx="45.5" ry="101" fill={bgColor} />
      <path
        d="M 80 550 C 40 600, 0 620, -60 650 C -120 680, -140 720, -180 750 C -220 780, -240 820, -260 850 C -280 880, -300 920, -340 950"
        fill="none"
        stroke="currentColor"
        strokeWidth="54"
        strokeLinecap="round"
      />
    </svg>
  )
}

/* ─────────────────────────── Hero ─────────────────────────── */

const scatteredIconMap: Record<string, React.ReactNode> = {
  github: <GithubIcon size={20} />,
  slack: <SlackIcon size={20} />,
  aws: <AwsIcon size={20} />,
  figma: <FigmaIcon size={20} />,
  vercel: <VercelIcon size={20} />,
  stripe: <StripeIcon size={20} />,
  postgres: <PostgresIcon size={20} />,
  "google-cloud": <GoogleCloudIcon size={20} />,
  sentry: <SentryIcon size={20} />,
  trello: <TrelloIcon size={20} />,
  "google-chrome": <GoogleChromeIcon size={20} />,
  tiktok: <TiktokIcon size={20} />,
  clickup: <ClickupIcon size={20} />,
  instagram: <InstagramIcon size={20} />,
  "google-excel": <GoogleExcelIcon size={20} />,
  "google-drive": <GoogleDriveIcon size={20} />,
  posthog: <PosthogIcon size={20} />,
  "google-analytics": <GoogleAnalyticsIcon size={20} />,
  "google-calendar": <GoogleCalendarIcon size={20} />,
  photoshop: <PhotoshopIcon size={20} />,
  "google-slides": <GoogleSlidesIcon size={20} />,
}

function ScatteredIcons() {
  const icons: {
    name: string
    x: number
    y: number
    size: number
    delay: string
    drift: number
    opacity: number
  }[] = [
    {
      name: "github",
      x: 27,
      y: 30,
      size: 36,
      delay: "0s",
      drift: 7,
      opacity: 0.8,
    },
    {
      name: "postgres",
      x: 38,
      y: 22,
      size: 36,
      delay: "0.35s",
      drift: 5,
      opacity: 0.8,
    },
    {
      name: "slack",
      x: 62,
      y: 23,
      size: 36,
      delay: "0.75s",
      drift: 6,
      opacity: 0.8,
    },
    {
      name: "figma",
      x: 75,
      y: 33,
      size: 36,
      delay: "1.15s",
      drift: 5,
      opacity: 0.8,
    },
    {
      name: "vercel",
      x: 34,
      y: 40,
      size: 36,
      delay: "1.55s",
      drift: 6,
      opacity: 0.8,
    },
    {
      name: "aws",
      x: 20,
      y: 46,
      size: 36,
      delay: "0.2s",
      drift: 6,
      opacity: 0.8,
    },
    {
      name: "sentry",
      x: 17,
      y: 58,
      size: 36,
      delay: "0.55s",
      drift: 7,
      opacity: 0.8,
    },
    {
      name: "google-chrome",
      x: 23,
      y: 68,
      size: 36,
      delay: "0.95s",
      drift: 5,
      opacity: 0.8,
    },
    {
      name: "posthog",
      x: 36,
      y: 76,
      size: 36,
      delay: "1.45s",
      drift: 6,
      opacity: 0.8,
    },
    {
      name: "stripe",
      x: 80,
      y: 44,
      size: 36,
      delay: "0.3s",
      drift: 5,
      opacity: 0.8,
    },
    {
      name: "trello",
      x: 83,
      y: 57,
      size: 36,
      delay: "0.7s",
      drift: 7,
      opacity: 0.8,
    },
    {
      name: "google-drive",
      x: 78,
      y: 68,
      size: 36,
      delay: "1.05s",
      drift: 5,
      opacity: 0.8,
    },
    {
      name: "google-cloud",
      x: 69,
      y: 76,
      size: 36,
      delay: "1.65s",
      drift: 6,
      opacity: 0.8,
    },
    {
      name: "google-analytics",
      x: 43,
      y: 82,
      size: 36,
      delay: "1.35s",
      drift: 5,
      opacity: 0.8,
    },
    {
      name: "tiktok",
      x: 60,
      y: 84,
      size: 36,
      delay: "1.8s",
      drift: 7,
      opacity: 0.8,
    },
    {
      name: "instagram",
      x: 50,
      y: 89,
      size: 36,
      delay: "0.5s",
      drift: 5,
      opacity: 0.8,
    },
    {
      name: "clickup",
      x: 57,
      y: 74,
      size: 36,
      delay: "1.95s",
      drift: 6,
      opacity: 0.8,
    },
  ]

  return (
    <div
      aria-hidden="true"
      className="pointer-events-none absolute top-1/2 left-1/2 z-0 hidden h-[clamp(560px,58vw,760px)] w-[min(1500px,calc(100vw-32px))] -translate-x-1/2 -translate-y-[45%] lg:block"
    >
      {icons.map((icon) => (
        <div
          key={`${icon.name}-${icon.x}-${icon.y}`}
          className="absolute"
          style={{
            left: `${icon.x}%`,
            top: `${icon.y}%`,
            transform: "translate(-50%, -50%)",
            opacity: icon.opacity,
          }}
        >
          <div
            className="flex items-center justify-center rounded-[40%] border border-black/[0.04] bg-white/80 shadow-sm backdrop-blur-sm dark:border-white/[0.08] dark:bg-white"
            style={{
              width: icon.size,
              height: icon.size,
              animation:
                "hero-icon-drift 5.8s cubic-bezier(0.22, 1, 0.36, 1) infinite",
              animationDelay: icon.delay,
              ["--hero-icon-drift" as string]: `${icon.drift}px`,
            }}
          >
            {scatteredIconMap[icon.name]}
          </div>
        </div>
      ))}
    </div>
  )
}

function HeroContent() {
  return (
    <div className="relative isolate flex flex-col items-center text-center">
      <ScatteredIcons />

      <div className="ghost-container animate-ghost-float absolute -top-24 left-1/2 z-10 -translate-x-1/2 cursor-pointer">
        <svg
          viewBox="0 0 640 640"
          width="64"
          height="64"
          className="text-muted-foreground drop-shadow-[0_0_16px_rgba(139,140,246,0.35)]"
          fill="currentColor"
        >
          <g>
            <path
              d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z"
              fill="currentColor"
            />
            <g className="ghost-eye">
              <ellipse
                cx="318.5"
                cy="282"
                rx="45.5"
                ry="101"
                fill="var(--background)"
              />
            </g>
            <g className="ghost-eye">
              <ellipse
                cx="457.5"
                cy="282"
                rx="45.5"
                ry="101"
                fill="var(--background)"
              />
            </g>
            <path
              className="ghost-tail"
              d="M 80 550 C 40 600, 0 620, -60 650 C -120 680, -140 720, -180 750 C -220 780, -240 820, -260 850 C -280 880, -300 920, -340 950"
              fill="none"
              stroke="currentColor"
              strokeWidth="54"
              strokeLinecap="round"
            />
          </g>
        </svg>
      </div>

      <div className="relative z-10 mb-6">
        <AnnouncementPill />
      </div>
      <h1 className="relative z-10 max-w-3xl font-heading text-5xl leading-none font-normal tracking-[-0.02em] text-foreground md:text-6xl lg:text-7xl">
        Your AI coworker
        <br />
        that gets work done
      </h1>
      <p className="relative z-10 mt-5 max-w-lg text-base leading-[1.6] text-muted-foreground sm:text-lg">
        Hivy connects to your tools, understands your work, and completes tasks
        across your team — from follow-ups and reports to pull requests and
        project updates.
      </p>
      <div className="relative z-10 mt-7 flex flex-col items-center gap-3 sm:flex-row">
        <Button size="lg" asChild>
          <a href="#">Hire hivy</a>
        </Button>
        <Button variant="secondary" size="lg" asChild>
          <a href="#">Talk to Sales</a>
        </Button>
      </div>
    </div>
  )
}

function TrustedLogos() {
  const logos = ["stripe", "notion", "linear", "vercel", "figma", "slack"]
  return (
    <div className="w-full max-w-5xl px-6 pt-2 pb-8">
      <div className="flex flex-wrap items-center justify-center gap-x-10 gap-y-5 sm:gap-x-14 lg:gap-x-20">
        {logos.map((name) => (
          <span
            key={name}
            className="font-heading text-2xl font-normal tracking-tight text-foreground opacity-40 transition-opacity duration-300 select-none hover:opacity-80 sm:text-2xl"
          >
            {name}
          </span>
        ))}
      </div>
    </div>
  )
}

/* ─────────────────────────── Features Bento ─────────────────────────── */

function WorksWhereYouWorkVisual() {
  const orbit = [
    { name: "slack", angle: -90, icon: <SlackIcon size={16} /> },
    { name: "figma", angle: -45, icon: <FigmaIcon size={16} /> },
    { name: "github", angle: 0, icon: <GithubIcon size={20} /> },
    { name: "drive", angle: 45, icon: <GoogleDriveIcon size={16} /> },
    { name: "sheets", angle: 90, icon: <GoogleExcelIcon size={16} /> },
    { name: "trello", angle: 135, icon: <TrelloIcon size={16} /> },
    { name: "sentry", angle: 180, icon: <SentryIcon size={16} /> },
    { name: "vercel", angle: -135, icon: <VercelIcon size={16} /> },
  ]

  return (
    <div className="relative h-full w-full overflow-hidden">
      <div
        className="absolute inset-0 opacity-[0.05]"
        style={{
          background:
            "radial-gradient(circle at 50% 50%, var(--pill-from), transparent 70%)",
        }}
      />
      {[0, 1, 2].map((delay) => (
        <div
          key={delay}
          className="absolute top-1/2 left-1/2 h-20 w-20 -translate-x-1/2 -translate-y-1/2"
        >
          <div
            className="h-full w-full rounded-full border opacity-0"
            style={{
              borderColor: "var(--pill-from)",
              animation: `pulse-ring 3s ease-out ${delay}s infinite`,
            }}
          />
        </div>
      ))}
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
        <Ghost
          color="var(--muted-foreground)"
          bgColor="var(--secondary)"
          size={40}
        />
      </div>
      {orbit.map((item) => (
        <div
          key={item.name}
          className="absolute top-1/2 left-1/2"
          style={{
            transform: `translate(-50%, -50%) rotate(${item.angle}deg) translateX(120px) rotate(${-item.angle}deg)`,
          }}
        >
          <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-black/[0.04] bg-white/80 shadow-sm backdrop-blur-sm dark:border-white/[0.08] dark:bg-white">
            {item.icon}
          </div>
        </div>
      ))}
    </div>
  )
}

function RemembersEverythingVisual() {
  const brainX = 165
  const brainY = 25

  const nodes = [
    { id: 1, x: 35, y: 12, type: "person" as const, delay: "0s" },
    { id: 2, x: 25, y: 25, type: "generic" as const, delay: "0.8s" },
    { id: 3, x: 40, y: 38, type: "doc" as const, delay: "1.6s" },
  ]

  const curvePaths = [
    `M${nodes[0].x},${nodes[0].y} C${nodes[0].x + 40},${nodes[0].y - 8} ${brainX - 40},${brainY - 8} ${brainX},${brainY}`,
    `M${nodes[1].x},${nodes[1].y} C${nodes[1].x + 45},${nodes[1].y} ${brainX - 45},${brainY} ${brainX},${brainY}`,
    `M${nodes[2].x},${nodes[2].y} C${nodes[2].x + 40},${nodes[2].y + 8} ${brainX - 40},${brainY + 8} ${brainX},${brainY}`,
  ]

  return (
    <div className="relative h-full w-full overflow-hidden">
      <div
        className="absolute inset-0 opacity-[0.05]"
        style={{
          background:
            "radial-gradient(circle at 85% 50%, var(--pill-from), transparent 70%)",
        }}
      />
      <svg viewBox="0 0 200 50" className="h-full w-full">
        {nodes.map((node, i) => (
          <g key={`line-${node.id}`}>
            <path
              d={curvePaths[i]}
              fill="none"
              stroke="var(--border)"
              strokeWidth="0.75"
              opacity="0.5"
            />
            <circle r="1.5" fill="var(--pill-from)" opacity="0.8">
              <animateMotion
                dur={`${2 + ((node.id * 0.4) % 1.5)}s`}
                repeatCount="indefinite"
                path={curvePaths[i]}
              />
            </circle>
          </g>
        ))}
        {nodes.map((node) => (
          <g
            key={node.id}
            className="animate-node-fade-in"
            style={{ animationDelay: node.delay }}
          >
            {node.type === "person" && (
              <>
                <circle
                  cx={node.x}
                  cy={node.y}
                  r="5"
                  fill="var(--secondary)"
                  stroke="var(--pill-from)"
                  strokeWidth="1"
                />
                <circle
                  cx={node.x}
                  cy={node.y - 1.2}
                  r="1.5"
                  fill="var(--muted-foreground)"
                  opacity="0.4"
                />
                <path
                  d={`M${node.x - 2},${node.y + 2.5} Q${node.x},${node.y + 0.8} ${node.x + 2},${node.y + 2.5}`}
                  stroke="var(--muted-foreground)"
                  strokeWidth="0.75"
                  fill="none"
                  opacity="0.4"
                />
              </>
            )}
            {node.type === "doc" && (
              <>
                <rect
                  x={node.x - 4}
                  y={node.y - 5.5}
                  width="8"
                  height="11"
                  rx="1.5"
                  fill="var(--secondary)"
                  stroke="var(--pill-to)"
                  strokeWidth="0.75"
                />
                <line
                  x1={node.x - 2}
                  y1={node.y - 2.5}
                  x2={node.x + 2}
                  y2={node.y - 2.5}
                  stroke="var(--muted-foreground)"
                  strokeWidth="0.5"
                  opacity="0.3"
                />
                <line
                  x1={node.x - 2}
                  y1={node.y}
                  x2={node.x + 0.5}
                  y2={node.y}
                  stroke="var(--muted-foreground)"
                  strokeWidth="0.5"
                  opacity="0.3"
                />
              </>
            )}
            {node.type === "generic" && (
              <>
                <circle
                  cx={node.x}
                  cy={node.y}
                  r="3"
                  fill="var(--secondary)"
                  stroke="var(--pill-via)"
                  strokeWidth="0.75"
                />
                <circle
                  cx={node.x}
                  cy={node.y}
                  r="1.2"
                  fill="var(--pill-from)"
                  opacity="0.5"
                />
              </>
            )}
          </g>
        ))}
        <g className="animate-node-fade-in">
          <circle
            cx={brainX}
            cy={brainY}
            r="9"
            fill="var(--secondary)"
            stroke="var(--pill-from)"
            strokeWidth="1"
            opacity="0.9"
          />
        </g>
      </svg>
    </div>
  )
}

function SecureByDefaultVisual() {
  const [selected, setSelected] = useState("sheets")
  const [perms, setPerms] = useState<Record<string, Record<string, boolean>>>({
    sheets: {
      "Read cells": true,
      "List sheets": true,
      "Download CSV": true,
      "View formulas": true,
      "Export PDF": true,
      "Check history": true,
      "Write cells": true,
      "Append rows": true,
      "Clone sheet": true,
      "Create sheet": true,
      "Rename sheet": true,
      "Format cells": true,
      "Delete sheet": false,
      "Share sheet": false,
      "Change permissions": false,
      "Delete rows": false,
      "Merge cells": false,
      "Protect range": false,
    },
    slack: {
      "Read messages": true,
      "List channels": true,
      "Search history": true,
      "View files": true,
      "See members": true,
      "Check status": true,
      "Send messages": true,
      "Create channels": true,
      "Upload files": true,
      "React to posts": true,
      "Start threads": true,
      "Set reminders": true,
      "Delete channels": false,
      "Invite users": false,
      "Manage webhooks": false,
      "Delete messages": false,
      "Archive channels": false,
      "Admin settings": false,
    },
    github: {
      "Read repos": true,
      "View issues": true,
      "List branches": true,
      "See PRs": true,
      "Read commits": true,
      "View wiki": true,
      "Create PRs": true,
      "Open issues": true,
      "Push commits": true,
      "Comment on PRs": true,
      "Label issues": true,
      "Request review": true,
      "Delete repos": false,
      "Manage secrets": false,
      "Admin access": false,
      "Force push": false,
      "Delete branches": false,
      "Modify rules": false,
    },
  })

  const connections = [
    {
      id: "sheets",
      name: "Google Sheets",
      icon: <GoogleExcelIcon size={18} />,
    },
    { id: "slack", name: "Slack", icon: <SlackIcon size={18} /> },
    { id: "github", name: "GitHub", icon: <GithubIcon size={18} /> },
  ]

  const currentPerms = perms[selected]
  const readKeys = Object.keys(currentPerms).slice(0, 6)
  const writeKeys = Object.keys(currentPerms).slice(6, 12)
  const adminKeys = Object.keys(currentPerms).slice(12, 18)

  const toggle = (key: string) => {
    setPerms((prev) => ({
      ...prev,
      [selected]: { ...prev[selected], [key]: !prev[selected][key] },
    }))
  }

  const renderRow = (key: string) => {
    const on = currentPerms[key]
    return (
      <div
        key={key}
        className="flex items-center justify-between rounded-lg border bg-background px-3 py-2"
        style={{
          borderColor: on
            ? "var(--border)"
            : "color-mix(in srgb, var(--border) 60%, transparent)",
        }}
      >
        <span
          className="text-sm font-medium"
          style={{
            color: on ? "var(--foreground)" : "var(--muted-foreground)",
          }}
        >
          {key}
        </span>
        <Switch checked={on} onCheckedChange={() => toggle(key)} />
      </div>
    )
  }

  return (
    <div className="flex h-full w-full flex-col px-6 pt-4 pb-5">
      <div className="mb-5 flex gap-3">
        {connections.map((c) => (
          <button
            key={c.id}
            type="button"
            onClick={() => setSelected(c.id)}
            className="flex items-center gap-2.5 rounded-xl px-3.5 py-2.5 text-sm font-medium transition-all"
            style={{
              backgroundColor:
                selected === c.id
                  ? "color-mix(in srgb, var(--pill-from) 7%, transparent)"
                  : "var(--secondary)",
              border: `1px solid ${selected === c.id ? "color-mix(in srgb, var(--pill-from) 25%, transparent)" : "var(--border)"}`,
              color:
                selected === c.id
                  ? "var(--foreground)"
                  : "var(--muted-foreground)",
              boxShadow:
                selected === c.id
                  ? "0 0 0 3px color-mix(in srgb, var(--pill-from) 8%, transparent)"
                  : "none",
            }}
          >
            {c.icon}
            <span>{c.name}</span>
          </button>
        ))}
      </div>
      <AnimatePresence mode="wait">
        <motion.div
          key={selected}
          className="grid flex-1 grid-cols-3 gap-4"
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -8 }}
          transition={{ duration: 0.2, ease: "easeOut" }}
        >
          <div className="flex flex-col gap-2">
            <div className="mb-1 flex items-center gap-2">
              <span className="h-2 w-2 rounded-full bg-green-500" />
              <span className="text-sm font-semibold text-foreground">
                Read
              </span>
            </div>
            {readKeys.map(renderRow)}
          </div>
          <div className="flex flex-col gap-2">
            <div className="mb-1 flex items-center gap-2">
              <span className="h-2 w-2 rounded-full bg-[var(--pill-from)]" />
              <span className="text-sm font-semibold text-foreground">
                Write
              </span>
            </div>
            {writeKeys.map(renderRow)}
          </div>
          <div className="flex flex-col gap-2">
            <div className="mb-1 flex items-center gap-2">
              <span className="h-2 w-2 rounded-full bg-red-500" />
              <span className="text-sm font-semibold text-foreground">
                Admin
              </span>
            </div>
            {adminKeys.map(renderRow)}
          </div>
        </motion.div>
      </AnimatePresence>
    </div>
  )
}

function FeaturesBento() {
  const features = [
    {
      title: "Works where you work",
      description:
        "Slack, Google Sheets, GitHub, Google Meet — Hivy lives inside the tools your team already uses. No new tabs, no context switching.",
      span: "md:row-span-2",
      height: "min-h-72 md:min-h-0",
      textPosition: "bottom" as const,
    },
    {
      title: "Remembers everything",
      description:
        "Hivy builds a living memory of your business, your teammates, and your decisions — so nothing important is ever lost.",
      span: "",
      height: "min-h-64",
      textPosition: "top" as const,
    },
    {
      title: "Automates the boring stuff",
      description:
        "Set unlimited recurring tasks on any schedule. Reports, follow-ups, reminders — Hivy handles it all while you focus on what matters.",
      span: "",
      height: "min-h-64",
      textPosition: "overlay" as const,
    },
    {
      title: "Secure by default",
      description:
        "Granular permissions let you control exactly what Hivy can and cannot do. Your data stays yours, always.",
      span: "md:col-span-2",
      height: "min-h-56",
      textPosition: "top" as const,
    },
  ]

  return (
    <section className="relative z-10 -mt-24 w-full max-w-5xl px-4 py-8 sm:py-8 md:px-0">
      <div className="mb-12 text-center sm:mb-16">
        <h2 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
          Built for the way you work
        </h2>
        <p className="mx-auto mt-4 max-w-lg text-base leading-[1.6] text-muted-foreground">
          Four reasons teams choose Hivy as their AI coworker.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 md:gap-5">
        {features.map((f, i) => (
          <div
            key={i}
            className={`group relative flex flex-col overflow-hidden rounded-2xl border border-border bg-secondary transition-all duration-300 ${f.span} ${f.height}`}
          >
            {f.textPosition === "top" && (
              <div className="px-6 pt-6 pb-2">
                <h3 className="font-heading text-lg leading-snug font-medium tracking-tight text-foreground sm:text-xl">
                  {f.title}
                </h3>
                <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
                  {f.description}
                </p>
              </div>
            )}
            <div
              className={`relative overflow-hidden ${i === 0 || i === 1 || f.textPosition === "overlay" ? "flex-1" : i === 3 ? "h-[412px]" : "h-44"}`}
            >
              {i === 0 ? (
                <WorksWhereYouWorkVisual />
              ) : i === 1 ? (
                <RemembersEverythingVisual />
              ) : i === 3 ? (
                <SecureByDefaultVisual />
              ) : (
                <>
                  <div
                    className="absolute inset-0 opacity-[0.06]"
                    style={{
                      background:
                        "linear-gradient(135deg, var(--pill-from), var(--pill-to))",
                    }}
                  />
                  <div className="flex h-full w-full items-center justify-center">
                    <span className="text-xs font-medium text-muted-foreground opacity-40">
                      Graphic placeholder
                    </span>
                  </div>
                </>
              )}
            </div>
            {f.textPosition === "bottom" && (
              <div className="px-6 pt-4 pb-6">
                <h3 className="font-heading text-lg leading-snug font-medium tracking-tight text-foreground sm:text-xl">
                  {f.title}
                </h3>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                  {f.description}
                </p>
              </div>
            )}
            {f.textPosition === "overlay" && (
              <div className="absolute right-0 bottom-0 left-0 px-6 pt-8 pb-5 backdrop-blur-sm">
                <h3 className="font-heading text-lg leading-snug font-medium tracking-tight text-foreground sm:text-xl">
                  {f.title}
                </h3>
                <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
                  {f.description}
                </p>
              </div>
            )}
          </div>
        ))}
      </div>
    </section>
  )
}

/* ─────────────────────────── Slack Chat ─────────────────────────── */

function SlackChatSection() {
  const messages = [
    {
      sender: "user",
      name: "Alex",
      text: "Hey team, can someone pull the Q3 numbers from the Sheets?",
      time: "9:41 AM",
    },
    {
      sender: "hivy",
      name: "hivy",
      text: "On it — pulling Q3 revenue, expenses, and growth metrics from the master sheet now.",
      time: "9:41 AM",
    },
    {
      sender: "user",
      name: "Alex",
      text: "Also draft a follow-up email to the investors?",
      time: "9:42 AM",
    },
    {
      sender: "hivy",
      name: "hivy",
      text: "Draft ready in your Gmail drafts. I used last quarter's template and plugged in the new numbers.",
      time: "9:42 AM",
    },
    {
      sender: "user",
      name: "Alex",
      text: "You're a lifesaver 🙏",
      time: "9:43 AM",
    },
    {
      sender: "hivy",
      name: "hivy",
      text: "Anytime. I'll also create a GitHub issue to track the follow-up tasks.",
      time: "9:43 AM",
    },
  ]

  const channels = ["general", "engineering", "design", "announcements"]
  const dms = ["Sarah", "hivy", "Mike"]

  return (
    <section className="relative z-10 w-full max-w-5xl px-4 py-16 sm:py-24 md:px-0">
      <div className="mb-10 text-center sm:mb-14">
        <h2 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
          Say hello to hivy, your new coworker
        </h2>
        <p className="mx-auto mt-4 max-w-lg text-base leading-[1.6] text-muted-foreground">
          Hivy lives in your Slack, hears your requests, and gets things done —
          just like a teammate who never sleeps.
        </p>
      </div>

      <div
        className="rounded-3xl p-8 sm:p-12"
        style={{
          background:
            "linear-gradient(135deg, color-mix(in srgb, var(--pill-from) 30%, transparent), color-mix(in srgb, var(--pill-via) 20%, transparent), color-mix(in srgb, var(--pill-to) 30%, transparent))",
        }}
      >
        <div
          className="flex overflow-hidden rounded-xl shadow-2xl"
          style={{ height: 600 }}
        >
          <div
            className="flex w-52 shrink-0 flex-col"
            style={{ backgroundColor: "#3F0E40" }}
          >
            <div className="flex items-center gap-2 px-4 py-3">
              <div className="flex h-6 w-6 items-center justify-center rounded bg-white/10">
                <svg viewBox="0 0 127 127" className="h-3.5 w-3.5">
                  <path
                    d="M27.2 80.0c0 7.3-5.9 13.2-13.2 13.2S.8 87.3.8 80c0-7.3 5.9-13.2 13.2-13.2h13.2v13.2zm6.6 0c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2v33c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V80z"
                    fill="#E01E5A"
                  />
                  <path
                    d="M47.0 27.0c-7.3 0-13.2-5.9-13.2-13.2S39.7.6 47.0.6s13.2 5.9 13.2 13.2v13.2H47.0zm0 6.7c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H13.5C6.2 60.1.3 54.2.3 46.9s5.9-13.2 13.2-13.2h33.5z"
                    fill="#36C5F0"
                  />
                  <path
                    d="M99.9 46.9c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2-5.9 13.2-13.2 13.2H99.9V46.9zm-6.6 0c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V13.5C66.9 6.2 72.8.3 80.1.3s13.2 5.9 13.2 13.2v33.4z"
                    fill="#2EB67D"
                  />
                  <path
                    d="M80.1 99.8c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2-13.2-5.9-13.2-13.2V99.8h13.2zm0-6.6c-7.3 0-13.2-5.9-13.2-13.2s5.9-13.2 13.2-13.2h33.5c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H80.1z"
                    fill="#ECB22E"
                  />
                </svg>
              </div>
              <div>
                <div className="text-sm font-bold text-white">Acme Inc</div>
                <div className="text-xs text-white/50">hivy workspace</div>
              </div>
            </div>
            <div className="px-3 py-2">
              <div className="mb-1 px-2 text-xs font-semibold tracking-wide text-white/40 uppercase">
                Channels
              </div>
              {channels.map((c) => (
                <div
                  key={c}
                  className="flex items-center gap-2 rounded px-2 py-1 text-sm"
                  style={{
                    color: c === "general" ? "white" : "rgba(255,255,255,0.6)",
                    backgroundColor:
                      c === "general" ? "#1164A3" : "transparent",
                  }}
                >
                  <span className="text-white/40">#</span>
                  <span>{c}</span>
                </div>
              ))}
            </div>
            <div className="px-3 py-2">
              <div className="mb-1 px-2 text-xs font-semibold tracking-wide text-white/40 uppercase">
                Direct messages
              </div>
              {dms.map((dm) => (
                <div
                  key={dm}
                  className="flex items-center gap-2 rounded px-2 py-1 text-sm text-white/60"
                >
                  <span className="relative flex h-2 w-2">
                    <span className="absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75" />
                    <span className="relative inline-flex h-2 w-2 rounded-full bg-green-400" />
                  </span>
                  <span>{dm}</span>
                  {dm === "hivy" && (
                    <span className="ml-auto rounded bg-white/10 px-1.5 py-0 text-[10px] text-white/60">
                      APP
                    </span>
                  )}
                </div>
              ))}
            </div>
          </div>

          <div className="flex flex-1 flex-col bg-white">
            <div className="flex items-center border-b border-gray-200 px-5 py-3">
              <div className="flex items-center gap-2">
                <span className="text-lg font-bold text-gray-400">#</span>
                <span className="text-base font-bold text-gray-900">
                  general
                </span>
              </div>
              <div className="ml-4 flex items-center gap-1 text-sm text-gray-400">
                <svg
                  className="h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                  />
                </svg>
                <span>3</span>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-5 py-4">
              {messages.map((msg, i) => (
                <div
                  key={i}
                  className="group flex items-start gap-3 py-2 hover:bg-gray-50"
                >
                  <div
                    className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-sm font-bold text-white"
                    style={{
                      backgroundColor:
                        msg.sender === "hivy" ? "var(--pill-from)" : "#E01E5A",
                    }}
                  >
                    {msg.sender === "hivy" ? (
                      <svg
                        viewBox="0 0 640 640"
                        className="h-5 w-5"
                        fill="currentColor"
                      >
                        <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" />
                        <ellipse
                          cx="318.5"
                          cy="282"
                          rx="45.5"
                          ry="101"
                          fill="white"
                        />
                        <ellipse
                          cx="457.5"
                          cy="282"
                          rx="45.5"
                          ry="101"
                          fill="white"
                        />
                      </svg>
                    ) : (
                      msg.name[0]
                    )}
                  </div>
                  <div className="flex flex-col">
                    <div className="flex items-baseline gap-2">
                      <span
                        className="text-sm font-bold"
                        style={{
                          color:
                            msg.sender === "hivy"
                              ? "var(--pill-from)"
                              : "#1D1C1D",
                        }}
                      >
                        {msg.name}
                      </span>
                      {msg.sender === "hivy" && (
                        <span className="rounded bg-gray-100 px-1 py-0 text-[10px] font-semibold text-gray-500">
                          APP
                        </span>
                      )}
                      <span className="text-xs text-gray-400">{msg.time}</span>
                    </div>
                    <p className="text-sm leading-relaxed text-gray-700">
                      {msg.text}
                    </p>
                  </div>
                </div>
              ))}
            </div>
            <div className="border-t border-gray-200 px-5 py-3">
              <div className="flex items-center gap-2 rounded-lg border border-gray-300 px-4 py-2.5">
                <svg
                  className="h-5 w-5 text-gray-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 4v16m8-8H4"
                  />
                </svg>
                <span className="text-sm text-gray-400">Message #general</span>
                <div className="ml-auto flex items-center gap-1 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">
                  <svg
                    className="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13"
                    />
                  </svg>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

/* ─────────────────────────── Page ─────────────────────────── */

export default function HomePage() {
  return (
    <>
      <style>{`
        @keyframes fade-in-down {
          from { opacity: 0; transform: translateY(-8px); }
          to { opacity: 1; transform: translateY(0); }
        }
        @keyframes fade-in-up {
          from { opacity: 0; transform: translateY(16px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .animate-fade-in-down {
          animation: fade-in-down 0.6s ease-out both;
        }
        .animate-fade-in-up {
          animation: fade-in-up 0.7s ease-out 0.15s both;
        }
        @keyframes ghost-float {
          0%, 100% { transform: translateY(0px); }
          50% { transform: translateY(-8px); }
        }
        .animate-ghost-float {
          animation: ghost-float 3s ease-in-out infinite;
        }
        @keyframes hero-icon-drift {
          0%, 100% { transform: translateY(0); }
          50% { transform: translateY(calc(var(--hero-icon-drift) * -1)); }
        }
        @keyframes pulse-ring {
          0% { transform: scale(0.5); opacity: 0.3; }
          100% { transform: scale(2.5); opacity: 0; }
        }
        @keyframes node-fade-in {
          0% { opacity: 0; transform: scale(0.5); }
          100% { opacity: 1; transform: scale(1); }
        }
        .animate-node-fade-in {
          animation: node-fade-in 0.8s ease-out both;
        }
        .ghost-eye {
          transition: transform 0.25s ease;
        }
        .ghost-container:hover .ghost-eye {
          transform: translateX(-14px);
        }
        .ghost-tail {
          transform-origin: 80px 550px;
        }
        .ghost-container:hover .ghost-tail {
          animation: tail-wiggle 0.12s linear infinite;
        }
        @keyframes tail-wiggle {
          0% { transform: rotate(-6deg); }
          50% { transform: rotate(6deg); }
          100% { transform: rotate(-6deg); }
        }
      `}</style>
      <main className="relative flex min-h-screen flex-col items-center bg-background font-display text-foreground">
        <div className="pointer-events-none absolute inset-0 overflow-hidden">
          <div className="absolute -top-52 -left-28 h-[28rem] w-[28rem] rounded-full bg-[var(--glow-left)] opacity-55 blur-[140px]" />
          <div className="absolute -top-40 left-1/2 h-[28rem] w-[28rem] -translate-x-1/2 rounded-full bg-[var(--glow-center)] opacity-50 blur-[140px]" />
          <div className="absolute -top-52 -right-28 h-[28rem] w-[28rem] rounded-full bg-[var(--glow-right)] opacity-50 blur-[140px]" />
        </div>

        <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center justify-between px-4 md:px-0">
          <Link
            href="/"
            className="font-heading text-xl font-bold tracking-tight text-foreground"
          >
            hivy
          </Link>
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
            <Navbar />
          </div>
          <div className="flex items-center gap-2 sm:gap-3">
            <div className="hidden sm:block">
              <Button variant="ghost" size="sm" asChild>
                <a href="#">Talk to Sales</a>
              </Button>
            </div>
            <Button size="sm" asChild>
              <a href="#">Hire hivy</a>
            </Button>
          </div>
        </div>

        <div className="relative flex min-h-screen w-full flex-col items-center px-4 pt-36 sm:pt-44 lg:pt-52">
          <div className="animate-fade-in-up flex flex-1 flex-col items-center justify-center pb-10 sm:pb-14">
            <HeroContent />
            <div className="mt-36 sm:mt-44">
              <TrustedLogos />
            </div>
          </div>
        </div>

        <FeaturesBento />
        <SlackChatSection />
        <MarketingFooter />
      </main>
    </>
  )
}
