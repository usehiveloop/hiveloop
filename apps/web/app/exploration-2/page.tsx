"use client"

import { useState, useRef, useEffect } from "react"
import { AnimatePresence, motion } from "motion/react"
import {
  Theme,
  ROSE_THEME,
  Button,
  Switch,
  Card,
  Badge,
  Ghost,
} from "./_components/design-system"

function NavDropdown({
  label,
  items,
  theme,
}: {
  label: string
  items: { label: string; href: string }[]
  theme: Theme
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    return () => document.removeEventListener("mousedown", handleClickOutside)
  }, [])

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="group inline-flex items-center gap-0.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors hover:bg-black/[0.03]"
        style={{ color: open ? theme.text : theme.muted }}
      >
        {label}
        <svg
          width="12"
          height="12"
          viewBox="0 0 12 12"
          fill="none"
          className="mt-0.5 transition-transform"
          style={{
            color: "#9CA3AF",
            transform: open ? "rotate(180deg)" : "rotate(0deg)",
          }}
        >
          <path
            d="M3 4.5L6 7.5L9 4.5"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </button>
      {open && (
        <div
          className="absolute left-1/2 top-full z-50 mt-2 w-44 -translate-x-1/2 rounded-xl border p-1.5 shadow-lg backdrop-blur-lg"
          style={{
            backgroundColor: theme.navBg,
            borderColor: theme.navBorder,
          }}
        >
          {items.map((item) => (
            <a
              key={item.label}
              href={item.href}
              className="block rounded-lg px-3 py-2 text-xs font-medium transition-colors hover:bg-black/[0.03]"
              style={{ color: theme.muted }}
              onMouseEnter={(e) => (e.currentTarget.style.color = theme.text)}
              onMouseLeave={(e) => (e.currentTarget.style.color = theme.muted)}
            >
              {item.label}
            </a>
          ))}
        </div>
      )}
    </div>
  )
}

function Navbar({ theme }: { theme: Theme }) {
  return (
    <nav
      className="flex h-11 items-center rounded-full px-2 backdrop-blur-lg"
      style={{
        backgroundColor: theme.navBg,
        border: `1px solid ${theme.navBorder}`,
      }}
    >
      <div className="hidden items-center md:flex">
        <a
          href="#"
          className="rounded-lg px-3 py-2 text-xs font-medium transition-colors hover:bg-black/[0.03]"
          style={{ color: theme.muted }}
          onMouseEnter={(e) => (e.currentTarget.style.color = theme.text)}
          onMouseLeave={(e) => (e.currentTarget.style.color = theme.muted)}
        >
          Product
        </a>
        <NavDropdown
          label="Resources"
          items={[
            { label: "Blog", href: "#" },
            { label: "Changelog", href: "#" },
            { label: "Docs", href: "#" },
          ]}
          theme={theme}
        />
        <a
          href="#"
          className="rounded-lg px-3 py-2 text-xs font-medium transition-colors hover:bg-black/[0.03]"
          style={{ color: theme.muted }}
          onMouseEnter={(e) => (e.currentTarget.style.color = theme.text)}
          onMouseLeave={(e) => (e.currentTarget.style.color = theme.muted)}
        >
          Pricing
        </a>
        <NavDropdown
          label="Solutions"
          items={[
            { label: "Use cases", href: "#" },
            { label: "Integrations", href: "#" },
          ]}
          theme={theme}
        />
        <a
          href="https://github.com/usehivy/hivy"
          target="_blank"
          rel="noopener noreferrer"
          className="ml-1 flex items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors hover:bg-black/[0.03]"
          style={{ color: theme.muted }}
          onMouseEnter={(e) => (e.currentTarget.style.color = theme.text)}
          onMouseLeave={(e) => (e.currentTarget.style.color = theme.muted)}
        >
          <svg viewBox="0 0 1024 1024" className="h-4 w-4"><path fill="currentColor" fillRule="evenodd" d="M512 0C229.12 0 0 229.12 0 512c0 226.56 146.56 417.92 350.08 485.76 25.6 4.48 35.2-10.88 35.2-24.32 0-12.16-.64-52.48-.64-95.36-128.64 23.68-161.92-31.36-172.16-60.16-5.76-14.72-30.72-60.16-52.48-72.32-17.92-9.6-43.52-33.28-.64-33.92 40.32-.64 69.12 37.12 78.72 52.48 46.08 77.44 119.68 55.68 149.12 42.24 4.48-33.28 17.92-55.68 32.64-68.48-113.92-12.8-232.96-56.96-232.96-252.8 0-55.68 19.84-101.76 52.48-137.6-5.12-12.8-23.04-65.28 5.12-135.68 0 0 42.88-13.44 140.8 52.48 40.96-11.52 84.48-17.28 128-17.28s87.04 5.76 128 17.28c97.92-66.56 140.8-52.48 140.8-52.48 28.16 70.4 10.24 122.88 5.12 135.68 32.64 35.84 52.48 81.28 52.48 137.6 0 196.48-119.68 240-233.6 252.8 18.56 16 34.56 46.72 34.56 94.72 0 68.48-.64 123.52-.64 140.8 0 13.44 9.6 29.44 35.2 24.32C877.44 929.92 1024 737.92 1024 512 1024 229.12 794.88 0 512 0" clipRule="evenodd"/></svg>
          <span>2.4k</span>
        </a>
      </div>
    </nav>
  )
}

function AnnouncementPill({ theme }: { theme: Theme }) {
  return (
    <Badge theme={theme} variant="dot">
      Meet Hivy — your AI coworker for busy teams
    </Badge>
  )
}

function ToolIcon({ name }: { name: string }) {
  const icons: Record<string, React.ReactNode> = {
    github: (
      <svg viewBox="0 0 1024 1024" className="h-5 w-5"><path fill="currentColor" fillRule="evenodd" d="M512 0C229.12 0 0 229.12 0 512c0 226.56 146.56 417.92 350.08 485.76 25.6 4.48 35.2-10.88 35.2-24.32 0-12.16-.64-52.48-.64-95.36-128.64 23.68-161.92-31.36-172.16-60.16-5.76-14.72-30.72-60.16-52.48-72.32-17.92-9.6-43.52-33.28-.64-33.92 40.32-.64 69.12 37.12 78.72 52.48 46.08 77.44 119.68 55.68 149.12 42.24 4.48-33.28 17.92-55.68 32.64-68.48-113.92-12.8-232.96-56.96-232.96-252.8 0-55.68 19.84-101.76 52.48-137.6-5.12-12.8-23.04-65.28 5.12-135.68 0 0 42.88-13.44 140.8 52.48 40.96-11.52 84.48-17.28 128-17.28s87.04 5.76 128 17.28c97.92-66.56 140.8-52.48 140.8-52.48 28.16 70.4 10.24 122.88 5.12 135.68 32.64 35.84 52.48 81.28 52.48 137.6 0 196.48-119.68 240-233.6 252.8 18.56 16 34.56 46.72 34.56 94.72 0 68.48-.64 123.52-.64 140.8 0 13.44 9.6 29.44 35.2 24.32C877.44 929.92 1024 737.92 1024 512 1024 229.12 794.88 0 512 0" clipRule="evenodd"/></svg>
    ),
    postgres: (
      <svg viewBox="0 0 432 445" className="h-5 w-5"><g fillRule="nonzero" clipRule="nonzero"><path d="M323 324c3-24 2-27 20-23l4 1c14 1 31-2 42-7 22-10 36-28 14-23-50 10-54-7-54-7 53-79 75-179 56-203-52-67-143-35-144-34l-1 1c-10-2-21-3-33-3-23 0-40 6-53 16 0 0-161-66-154 84 2 32 46 242 99 178 19-23 38-43 38-43 9 6 20 9 32 8l1-1c0 3 0 6 0 9-14 15-10 18-37 23-27 6-11 16 0 19 13 3 42 8 62-20l-1 3c5 4 5 31 6 49 1 19 2 36 6 47 4 10 8 37 44 29 30-6 53-16 55-101" fill="#336791"/><ellipse cx="173" cy="142" rx="9" ry="16" fill="white"/></g></svg>
    ),
    aws: (
      <svg viewBox="0 0 304 182" className="h-5 w-5"><path fill="#252f3e" d="m86 66 2 9c0 3 1 5 3 8v2l-1 3-7 4-2 1-3-1-4-5-3-6c-8 9-18 14-29 14-9 0-16-3-20-8-5-4-8-11-8-19s3-15 9-20c6-6 14-8 25-8a79 79 0 0 1 22 3v-7c0-8-2-13-5-16-3-4-8-5-16-5l-11 1a80 80 0 0 0-14 5h-2c-1 0-2-1-2-3v-5l1-3c0-1 1-2 3-2l12-5 16-2c12 0 20 3 26 8 5 6 8 14 8 25v32zM46 82l10-2c4-1 7-4 10-7l3-6 1-9v-4a84 84 0 0 0-19-2c-6 0-11 1-15 4-3 2-4 6-4 11s1 8 3 11c3 2 6 4 11 4zm80 10-4-1-2-3-23-78-1-4 2-2h10l4 1 2 4 17 66 15-66 2-4 4-1h8l4 1 2 4 16 67 17-67 2-4 4-1h9c2 0 3 1 3 2v2l-1 2-24 78-2 4-4 1h-9l-4-1-1-4-16-65-15 64-2 4-4 1h-9zm129 3a66 66 0 0 1-27-6l-3-3-1-2v-5c0-2 1-3 2-3h2l3 1a54 54 0 0 0 23 5c6 0 11-2 14-4 4-2 5-5 5-9l-2-7-10-5-15-5c-7-2-13-6-16-10a24 24 0 0 1 5-34l10-5a44 44 0 0 1 20-2 110 110 0 0 1 12 3l4 2 3 2 1 4v4c0 3-1 4-2 4l-4-2c-6-2-12-3-19-3-6 0-11 0-14 2s-4 5-4 9c0 3 1 5 3 7s5 4 11 6l14 4c7 3 12 6 15 10s5 9 5 14l-3 12-7 8c-3 3-7 5-11 6l-14 2z"/><path d="M274 144A220 220 0 0 1 4 124c-4-3-1-6 2-4a300 300 0 0 0 263 16c5-2 10 4 5 8z" fill="#f90"/><path d="M287 128c-4-5-28-3-38-1-4 0-4-3-1-5 19-13 50-9 53-5 4 5-1 36-18 51-3 2-6 1-5-2 5-10 13-33 9-38z" fill="#f90"/></svg>
    ),
    sentry: (
      <svg viewBox="0 0 256 227" className="h-5 w-5"><path fill="#362D59" d="M148 12a24 24 0 0 0-41 0L74 70c52 26 87 78 91 137h-24c-4-50-34-94-79-116l-31 54a82 82 0 0 1 47 62h-54a4 4 0 0 1-3-6l15-26a55 55 0 0 0-17-10L3 191a23 23 0 0 0 20 35h74a99 99 0 0 0-41-89l12-20c36 24 56 66 53 109h63c3-65-29-128-84-163l24-41a4 4 0 0 1 5-1c3 1 104 178 106 180a4 4 0 0 1-3 6h-24c0 7 0 13 0 20h24A24 24 0 0 0 256 203a23 23 0 0 0-3-12L148 12Z"/></svg>
    ),
    "google-cloud": (
      <svg viewBox="0 0 256 256" className="h-5 w-5"><path fill="#EA4335" d="M170 32h22l1-9C153-12 89-8 52 34 42 45 35 60 31 75l8-1 44-7 3-4c20-22 53-24 76-6l8 1Z"/><path fill="#4285F4" d="M224 74a100 100 0 0 0-30-49L163 56a56 56 0 0 1 20 44v6c15 0 28 12 28 28s-12 27-28 27h-56l-5 6v33l5 5h56c40 0 72-32 72-71a72 72 0 0 0-32-60"/><path fill="#34A853" d="M72 206h56v-45H72a27 27 0 0 1-11-2l-8 2-22 23-2 7c13 10 28 15 44 15"/><path fill="#FBBC05" d="M72 61C32 62 0 94 0 134a72 72 0 0 0 28 57l32-32c-14-6-20-23-14-37s23-20 37-14a28 28 0 0 1 14 14l32-32A72 72 0 0 0 72 61"/></svg>
    ),
    "google-chrome": (
      <svg viewBox="0 0 191 191" className="h-5 w-5"><path fill="#fff" d="M95 143a48 48 0 1 0 0-95 48 48 0 0 0 0 95z"/><path fill="#229342" d="m54 119-41-71a95 95 0 0 0 0 95 95 95 0 0 0 82 48l41-71v-1a48 48 0 0 1-17 17 48 48 0 0 1-48 1 48 48 0 0 1-17-18z"/><path fill="#fbc116" d="m136 119-41 71a95 95 0 0 0 83-48A95 95 0 0 0 191 95a95 95 0 0 0-13-48H95l-1 1a48 48 0 0 1 24 6 48 48 0 0 1 17 17 48 48 0 0 1 0 48z"/><path fill="#1a73e8" d="M95 133a37 37 0 1 0 0-75 37 37 0 0 0 0 75z"/><path fill="#e33b2e" d="M95 48h82A95 95 0 0 0 143 13 95 95 0 0 0 95 0a95 95 0 0 0-48 13 95 95 0 0 0-35 35l41 71 1 1a48 48 0 0 1 0-48 48 48 0 0 1 41-24z"/></svg>
    ),
    slack: (
      <svg viewBox="0 0 2448 2453" className="h-5 w-5"><path fill="#36c5f0" d="m897 0c-135 0-245 110-245 245 0 135 110 245 245 245h245V245C1142 110 1032 0 897 0m0 654H244C109 654-1 764-1 899c-1 135 109 245 245 245h653c135 0 245-110 245-245 0-135-110-245-245-245z"/><path fill="#2eb67d" d="M2448 899c0-135-110-245-245-245s-245 110-245 245v245h245c135 0 245-110 245-245m-653 0V245c1-135-109-245-245-245S1255 110 1255 245v654c-1 135 109 245 245 245 135 0 245-110 245-245z"/><path fill="#ecb22e" d="M1550 2453c135 0 245-110 245-245 0-135-110-245-245-245h-245v245c0 135 110 245 245 245m0-654h653c135 0 245-110 245-245 0-135-110-245-245-245h-653c-135 0-245 110-245 245 0 135 110 245 245 245z"/><path fill="#e01e5a" d="M0 1553c0 135 110 245 245 245s245-110 245-245v-245H245C110 1308 0 1418 0 1553m654 0v654c-1 135 109 245 245 245s245-110 245-245v-654c1-135-109-245-245-245-135 0-245 110-245 245z"/></svg>
    ),
    figma: (
      <svg viewBox="0 0 54 80" className="h-5 w-5"><path d="M13 80a13 13 0 0 0 14-13V53H13A13 13 0 0 0 0 66c0 8 6 14 13 14Z" fill="#0ACF83"/><path d="M0 40c0-7 6-13 13-13h14v27H13A13 13 0 0 1 0 40Z" fill="#A259FF"/><path d="M0 13C0 6 6 0 13 0h14v27H13C6 27 0 21 0 13Z" fill="#F24E1E"/><path d="M27 0h13c7 0 13 6 13 13s-6 13-13 13H27V0Z" fill="#FF7262"/><path d="M53 40a13 13 0 1 1-26 0 13 13 0 0 1 26 0Z" fill="#1ABCFE"/></svg>
    ),
    trello: (
      <svg viewBox="0 0 63 63" className="h-5 w-5"><path d="M56 0H8a8 8 0 0 0-8 8v47a8 8 0 0 0 8 8h48a8 8 0 0 0 8-8V8a8 8 0 0 0-8-8zM28 45a3 3 0 0 1-3 3H15a3 3 0 0 1-3-3V15a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3v30zm24-14a3 3 0 0 1-3 3H39a3 3 0 0 1-3-3V15a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3v16z" fill="#2684ff"/></svg>
    ),
    "google-excel": (
      <svg viewBox="0 0 74 100" className="h-5 w-5"><path d="M45 1H8a7 7 0 0 0-7 7v84a7 7 0 0 0 7 7h57a7 7 0 0 0 7-7V28L45 1z" fill="#0F9D58"/><path d="M19 49v32h34V49H19zm15 28H23v-5h11v5zm0-9H23v-5h11v5zm0-9H23v-6h11v6zm15 18H38v-5h11v5zm0-9H38v-5h11v5zm0-9H38v-6h11v6z" fill="white"/></svg>
    ),
    "google-drive": (
      <svg viewBox="0 0 87 78" className="h-5 w-5"><path fill="#0066da" d="m7 67 4 7a8 8 0 0 0 3 3h15L27 53H0a8 8 0 0 0 7 14z"/><path fill="#00ac47" d="M44 25 29 1a8 8 0 0 0-3 3L1 48a8 8 0 0 0 7 5h27z"/><path fill="#ea4335" d="M74 77a8 8 0 0 0 3-3l2-3 7-13a8 8 0 0 0 1-5H60l5 11z"/><path fill="#00832d" d="M44 25 57 1a8 8 0 0 0-4-1H34a8 8 0 0 0-5 1z"/><path fill="#2684fc" d="M60 53H27L13 77a8 8 0 0 0 5 1h51a8 8 0 0 0 4-1z"/><path fill="#ffba00" d="m73 27-13-22a8 8 0 0 0-3-3L44 25 60 53h28a8 8 0 0 0-1-5z"/></svg>
    ),
    "google-analytics": (
      <svg viewBox="0 0 2200 2430" className="h-5 w-5"><path fill="#E37400" d="M2196 2127a303 303 0 0 1-338 302 304 304 0 0 1-301-316V316a303 303 0 0 1 301-316 304 304 0 0 1 338 302v1825zM301 1829a301 301 0 1 0 0 602 301 301 0 0 0 0-602zm792-913a302 302 0 0 0-293 317v809c0 220 97 353 238 381 163 33 322-72 355-236 4-20 6-40 6-60v-907a302 302 0 0 0-306-304z"/></svg>
    ),
    clickup: (
      <svg viewBox="0 0 64 64" className="h-5 w-5"><path fill="#7B68EE" d="M32 8L8 32l10 10 14-14 14 14 10-10L32 8z"/><path fill="#FF4081" d="M32 24L18 38l14 14 14-14-14-14z"/></svg>
    ),
    instagram: (
      <svg viewBox="0 0 24 24" className="h-5 w-5"><path fill="url(#ig1)" d="M12 2.2c3.2 0 3.6 0 4.9.1 1.2.1 1.8.2 2.2.4.6.2 1 .5 1.4.9.4.4.7.8.9 1.4.2.4.3 1 .4 2.2.1 1.3.1 1.7.1 4.9s0 3.6-.1 4.9c-.1 1.2-.2 1.8-.4 2.2-.2.6-.5 1-.9 1.4-.4.4-.8.7-1.4.9-.4.2-1 .3-2.2.4-1.3.1-1.7.1-4.9.1s-3.6 0-4.9-.1c-1.2-.1-1.8-.2-2.2-.4-.6-.2-1-.5-1.4-.9-.4-.4-.7-.8-.9-1.4-.2-.4-.3-1-.4-2.2-.1-1.3-.1-1.7-.1-4.9s0-3.6.1-4.9c.1-1.2.2-1.8.4-2.2.2-.6.5-1 .9-1.4.4-.4.8-.7 1.4-.9.4-.2 1-.3 2.2-.4 1.3-.1 1.7-.1 4.9-.1M12 0C8.7 0 8.3 0 7 .1 5.7.1 4.8.3 4 .6c-.9.3-1.6.7-2.3 1.4C1 2.7.6 3.4.3 4.3.1 5.1 0 6 0 7.3c0 1.3-.1 1.7-.1 5s0 3.7.1 5c.1 1.3.3 2.2.6 3 .3.9.7 1.6 1.4 2.3.7.7 1.4 1.1 2.3 1.4.8.3 1.7.5 3 .6 1.3.1 1.7.1 5 .1s3.7 0 5-.1c1.3-.1 2.2-.3 3-.6.9-.3 1.6-.7 2.3-1.4.7-.7 1.1-1.4 1.4-2.3.3-.8.5-1.7.6-3 .1-1.3.1-1.7.1-5s0-3.7-.1-5c-.1-1.3-.3-2.2-.6-3-.3-.9-.7-1.6-1.4-2.3C21.6 1 21 0.6 20 .3c-.8-.3-1.7-.5-3-.6C15.7.1 15.3 0 12 0z"/><path fill="url(#ig1)" d="M12 5.8a6.2 6.2 0 1 0 0 12.4 6.2 6.2 0 0 0 0-12.4zM12 16a4 4 0 1 1 0-8 4 4 0 0 1 0 8z"/><circle cx="18.4" cy="5.6" r="1.4" fill="url(#ig1)"/><defs><radialGradient id="ig1" cx="0" cy="0" r="1"><stop offset="0%" stop-color="#f09433"/><stop offset="25%" stop-color="#e6683c"/><stop offset="50%" stop-color="#dc2743"/><stop offset="75%" stop-color="#cc2366"/><stop offset="100%" stop-color="#bc1888"/></radialGradient></defs></svg>
    ),
    posthog: (
      <svg viewBox="0 0 50 30" className="h-5 w-5"><path fill="#F54E00" d="M0 3.4c0-.89 1.08-1.34 1.71-.71l4.58 4.58c.63.63.18 1.71-.71 1.71H1a1 1 0 0 1-1-1V3.4z"/><path fill="#000" d="m42.5 23.5-9.4-9.4c-.63-.63-1.71-.18-1.71.71v13.2a1 1 0 0 0 1 1h14.6a1 1 0 0 0 1-1v-1.2a1 1 0 0 0-1-1h-4.5z"/><path fill="#1D4AFF" d="M10.9 17.2a1 1 0 0 1-1.79 0l-.88-1.76a1 1 0 0 1 0-.9l.88-1.76a1 1 0 0 1 1.79 0l.88 1.76a1 1 0 0 1 0 .9l-.88 1.76z"/></svg>
    ),
    stripe: (
      <svg viewBox="0 0 32 32" className="h-5 w-5"><path fill="#635BFF" d="M30 16c0-7.7-6.3-14-14-14S2 8.3 2 16s6.3 14 14 14 14-6.3 14-14z"/><path fill="#fff" d="M13.2 12.5c0-.8.7-1.1 1.8-1.1 1.6 0 3.6.5 5.2 1.4V8.4c-1.7-.7-3.4-1-5.2-1-4.3 0-7.2 2.2-7.2 5.9 0 5.8 8 4.9 8 7.4 0 .9-.8 1.2-1.9 1.2-1.7 0-3.9-.7-5.6-1.7v4.5c1.9.8 3.9 1.2 5.6 1.2 4.4 0 7.5-2.2 7.5-5.9-.1-6.2-8.1-5.1-8.1-7.5z"/></svg>
    ),
    tiktok: (
      <svg viewBox="0 0 24 24" className="h-5 w-5"><path fill="#000" d="M19.59 6.69a4.83 4.83 0 0 1-3.77-4.25V2h-3.45v13.67a2.89 2.89 0 0 1-5.2 1.74 2.89 2.89 0 0 1 2.31-4.64 2.93 2.93 0 0 1 .88.13V9.4a6.84 6.84 0 0 0-1-.05A6.33 6.33 0 0 0 5 20.1a6.34 6.34 0 0 0 10.86-4.43v-7a8.16 8.16 0 0 0 4.77 1.52v-3.4a4.85 4.85 0 0 1-1-.1z"/></svg>
    ),
    vercel: (
      <svg viewBox="0 0 76 65" className="h-5 w-5"><path fill="#000" d="M37.5274 0L75.0548 65H0L37.5274 0Z"/></svg>
    ),
    "google-calendar": (
      <svg viewBox="0 0 24 24" className="h-5 w-5"><path fill="#4285F4" d="M19 4h-1V2h-2v2H8V2H6v2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2z"/><path fill="#fff" d="M19 20H5V9h14v11z"/><path fill="#EA4335" d="M12 11h5v5h-5z"/></svg>
    ),
    "google-slides": (
      <svg viewBox="0 0 24 24" className="h-5 w-5"><path fill="#F9AB00" d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2z"/><path fill="#fff" d="M7 7h10v2H7zm0 4h10v2H7zm0 4h7v2H7z"/></svg>
    ),
    photoshop: (
      <svg viewBox="0 0 32 32" className="h-5 w-5"><path fill="#001E36" d="M4 2h24a2 2 0 0 1 2 2v24a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z"/><path fill="#31A8FF" d="M8.5 10h2.5c2 0 3.5.5 3.5 2.5S12.5 15 10.5 15H9v5H6.5V10H8.5zm.5 3.5h1.5c1 0 1.5-.5 1.5-1s-.5-1-1.5-1H9v2zm7 1.5c0-1.5 1-2.5 3-2.5s3 1 3 2.5v5h-2.5v-4.5c0-.5-.5-1-1-1s-1 .5-1 1v4.5h-2.5v-5.5z"/></svg>
    ),
  }
  return <div className="flex h-8 w-8 items-center justify-center rounded-lg border border-black/[0.04] bg-white/70 shadow-sm backdrop-blur-sm">{icons[name] || null}</div>
}

function ScatteredIcons() {
  const icons: { name: string; top: string; left?: string; right?: string; delay: string; size?: number }[] = [
    // Upper area — above the title
    { name: "github", top: "-90px", left: "-180px", delay: "0s" },
    { name: "slack", top: "-70px", right: "-160px", delay: "0.7s" },
    { name: "aws", top: "-110px", left: "60px", delay: "1.3s" },
    { name: "figma", top: "-100px", right: "40px", delay: "0.4s" },
    { name: "vercel", top: "-130px", left: "-40px", delay: "2.1s" },
    { name: "stripe", top: "-60px", right: "-80px", delay: "1.1s" },
    { name: "postgres", top: "-140px", right: "-200px", delay: "0.2s" },
    { name: "google-cloud", top: "-80px", left: "-100px", delay: "1.8s" },
    // Around title area
    { name: "sentry", top: "20px", left: "-220px", delay: "0.5s" },
    { name: "trello", top: "10px", right: "-240px", delay: "1.5s" },
    { name: "google-chrome", top: "80px", left: "-140px", delay: "0.9s" },
    { name: "tiktok", top: "70px", right: "-180px", delay: "2.3s" },
    { name: "clickup", top: "130px", left: "-260px", delay: "0.3s" },
    { name: "instagram", top: "120px", right: "-140px", delay: "1.7s" },
    // Around description
    { name: "google-excel", top: "180px", left: "-100px", delay: "2.5s" },
    { name: "google-drive", top: "190px", right: "-220px", delay: "0.6s" },
    { name: "posthog", top: "220px", left: "-200px", delay: "1.2s" },
    { name: "google-analytics", top: "210px", right: "-100px", delay: "2.7s" },
    // Around CTAs
    { name: "google-calendar", top: "290px", left: "-160px", delay: "0.8s" },
    { name: "photoshop", top: "300px", right: "-160px", delay: "2.0s" },
    { name: "google-slides", top: "320px", left: "-280px", delay: "1.4s" },
    { name: "aws", top: "340px", right: "-260px", delay: "0.1s" },
  ]

  return (
    <>
      {icons.map((icon) => (
        <div
          key={`${icon.name}-${icon.top}-${icon.left ?? icon.right}`}
          className="pointer-events-none absolute hidden lg:block"
          style={{
            top: icon.top,
            ...(icon.left ? { left: icon.left } : { right: icon.right }),
            animation: `gentle-float 4s ease-in-out infinite`,
            animationDelay: icon.delay,
            opacity: 0.8,
          }}
        >
          <ToolIcon name={icon.name} />
        </div>
      ))}
    </>
  )
}

function HeroContent({ theme }: { theme: Theme }) {
  return (
    <div className="relative flex flex-col items-center text-center">
      {/* Ghost */}
      <div className="ghost-container absolute -top-24 left-1/2 -translate-x-1/2 animate-ghost-float cursor-pointer">
        <svg viewBox="0 0 640 640" width="64" height="64" style={{ color: theme.muted }} fill="currentColor" className="drop-shadow-[0_0_16px_rgba(139,140,246,0.35)]">
          <g>
            <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" fill="currentColor" />
            <g className="ghost-eye">
              <ellipse cx="318.5" cy="282" rx="45.5" ry="101" fill={theme.bg} />
            </g>
            <g className="ghost-eye">
              <ellipse cx="457.5" cy="282" rx="45.5" ry="101" fill={theme.bg} />
            </g>
            <path className="ghost-tail" d="M 80 550 C 40 600, 0 620, -60 650 C -120 680, -140 720, -180 750 C -220 780, -240 820, -260 850 C -280 880, -300 920, -340 950" fill="none" stroke="currentColor" strokeWidth="54" strokeLinecap="round" />
          </g>
        </svg>
      </div>

      {/* Scattered tool icons */}
      <ScatteredIcons />

      <div className="mb-6">
        <AnnouncementPill theme={theme} />
      </div>
      <h1
        className="font-recoleta max-w-3xl text-5xl md:text-6xl lg:text-7xl font-normal leading-none tracking-[-0.02em]"
        style={{ color: theme.text }}
      >
        Your AI coworker
        <br />
        that gets work done
      </h1>
      <p className="mt-5 max-w-lg text-base leading-[1.6] sm:text-lg" style={{ color: theme.muted }}>
        Hivy connects to your tools, understands your work, and completes tasks across your team — from follow-ups and reports to pull requests and project updates.
      </p>
      <div className="mt-7 flex flex-col items-center gap-3 sm:flex-row">
        <Button variant="primary" size="md" theme={theme} href="#">
          Hire hivy
        </Button>
        <Button variant="secondary" size="md" theme={theme} href="#">
          Talk to Sales
        </Button>
      </div>
    </div>
  )
}

function TrustedLogos({ theme }: { theme: Theme }) {
  const logos = ["stripe", "notion", "linear", "vercel", "figma", "slack"]
  return (
    <div className="w-full max-w-5xl px-6 pt-2 pb-8">
      <div className="flex flex-wrap items-center justify-center gap-x-10 gap-y-5 sm:gap-x-14 lg:gap-x-20">
        {logos.map((name) => (
          <span
            key={name}
            className="select-none font-recoleta text-2xl font-normal tracking-tight opacity-40 transition-opacity duration-300 hover:opacity-80 sm:text-2xl"
            style={{ color: theme.text }}
          >
            {name}
          </span>
        ))}
      </div>
    </div>
  )
}



function WorksWhereYouWorkVisual({ theme }: { theme: Theme }) {
  const orbit = [
    {
      name: "slack",
      angle: -90,
      svg: (
        <svg viewBox="0 0 2448 2453" className="h-4 w-4"><path fill="#36c5f0" d="m897 0c-135 0-245 110-245 245 0 135 110 245 245 245h245V245C1142 110 1032 0 897 0m0 654H244C109 654-1 764-1 899c-1 135 109 245 245 245h653c135 0 245-110 245-245 0-135-110-245-245-245z"/><path fill="#2eb67d" d="M2448 899c0-135-110-245-245-245s-245 110-245 245v245h245c135 0 245-110 245-245m-653 0V245c1-135-109-245-245-245S1255 110 1255 245v654c-1 135 109 245 245 245 135 0 245-110 245-245z"/><path fill="#ecb22e" d="M1550 2453c135 0 245-110 245-245 0-135-110-245-245-245h-245v245c0 135 110 245 245 245m0-654h653c135 0 245-110 245-245 0-135-110-245-245-245h-653c-135 0-245 110-245 245 0 135 110 245 245 245z"/><path fill="#e01e5a" d="M0 1553c0 135 110 245 245 245s245-110 245-245v-245H245C110 1308 0 1418 0 1553m654 0v654c-1 135 109 245 245 245s245-110 245-245v-654c1-135-109-245-245-245-135 0-245 110-245 245z"/></svg>
      ),
    },
    {
      name: "figma",
      angle: -45,
      svg: (
        <svg viewBox="0 0 54 80" className="h-4 w-4"><path d="M13 80a13 13 0 0 0 14-13V53H13A13 13 0 0 0 0 66c0 8 6 14 13 14Z" fill="#0ACF83"/><path d="M0 40c0-7 6-13 13-13h14v27H13A13 13 0 0 1 0 40Z" fill="#A259FF"/><path d="M0 13C0 6 6 0 13 0h14v27H13C6 27 0 21 0 13Z" fill="#F24E1E"/><path d="M27 0h13c7 0 13 6 13 13s-6 13-13 13H27V0Z" fill="#FF7262"/><path d="M53 40a13 13 0 1 1-26 0 13 13 0 0 1 26 0Z" fill="#1ABCFE"/></svg>
      ),
    },
    {
      name: "github",
      angle: 0,
      svg: (
        <svg viewBox="0 0 1024 1024" className="h-4 w-4"><path fill="currentColor" fillRule="evenodd" d="M512 0C229.12 0 0 229.12 0 512c0 226.56 146.56 417.92 350.08 485.76 25.6 4.48 35.2-10.88 35.2-24.32 0-12.16-.64-52.48-.64-95.36-128.64 23.68-161.92-31.36-172.16-60.16-5.76-14.72-30.72-60.16-52.48-72.32-17.92-9.6-43.52-33.28-.64-33.92 40.32-.64 69.12 37.12 78.72 52.48 46.08 77.44 119.68 55.68 149.12 42.24 4.48-33.28 17.92-55.68 32.64-68.48-113.92-12.8-232.96-56.96-232.96-252.8 0-55.68 19.84-101.76 52.48-137.6-5.12-12.8-23.04-65.28 5.12-135.68 0 0 42.88-13.44 140.8 52.48 40.96-11.52 84.48-17.28 128-17.28s87.04 5.76 128 17.28c97.92-66.56 140.8-52.48 140.8-52.48 28.16 70.4 10.24 122.88 5.12 135.68 32.64 35.84 52.48 81.28 52.48 137.6 0 196.48-119.68 240-233.6 252.8 18.56 16 34.56 46.72 34.56 94.72 0 68.48-.64 123.52-.64 140.8 0 13.44 9.6 29.44 35.2 24.32C877.44 929.92 1024 737.92 1024 512 1024 229.12 794.88 0 512 0" clipRule="evenodd"/></svg>
      ),
    },
    {
      name: "google-drive",
      angle: 45,
      svg: (
        <svg viewBox="0 0 87 78" className="h-4 w-4"><path fill="#0066da" d="m7 67 4 7a8 8 0 0 0 3 3h15L27 53H0a8 8 0 0 0 7 14z"/><path fill="#00ac47" d="M44 25 29 1a8 8 0 0 0-3 3L1 48a8 8 0 0 0 7 5h27z"/><path fill="#ea4335" d="M74 77a8 8 0 0 0 3-3l2-3 7-13a8 8 0 0 0 1-5H60l5 11z"/><path fill="#00832d" d="M44 25 57 1a8 8 0 0 0-4-1H34a8 8 0 0 0-5 1z"/><path fill="#2684fc" d="M60 53H27L13 77a8 8 0 0 0 5 1h51a8 8 0 0 0 4-1z"/><path fill="#ffba00" d="m73 27-13-22a8 8 0 0 0-3-3L44 25 60 53h28a8 8 0 0 0-1-5z"/></svg>
      ),
    },
    {
      name: "sheets",
      angle: 90,
      svg: (
        <svg viewBox="0 0 74 100" className="h-4 w-4"><path d="M45 1H8a7 7 0 0 0-7 7v84a7 7 0 0 0 7 7h57a7 7 0 0 0 7-7V28L45 1z" fill="#0F9D58"/><path d="M19 49v32h34V49H19zm15 28H23v-5h11v5zm0-9H23v-5h11v5zm0-9H23v-6h11v6zm15 18H38v-5h11v5zm0-9H38v-5h11v5zm0-9H38v-6h11v6z" fill="white"/></svg>
      ),
    },
    {
      name: "trello",
      angle: 135,
      svg: (
        <svg viewBox="0 0 63 63" className="h-4 w-4"><path d="M56 0H8a8 8 0 0 0-8 8v47a8 8 0 0 0 8 8h48a8 8 0 0 0 8-8V8a8 8 0 0 0-8-8zM28 45a3 3 0 0 1-3 3H15a3 3 0 0 1-3-3V15a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3v30zm24-14a3 3 0 0 1-3 3H39a3 3 0 0 1-3-3V15a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3v16z" fill="#2684ff"/></svg>
      ),
    },
    {
      name: "sentry",
      angle: 180,
      svg: (
        <svg viewBox="0 0 256 227" className="h-4 w-4"><path fill="#362D59" d="M148 12a24 24 0 0 0-41 0L74 70c52 26 87 78 91 137h-24c-4-50-34-94-79-116l-31 54a82 82 0 0 1 47 62h-54a4 4 0 0 1-3-6l15-26a55 55 0 0 0-17-10L3 191a23 23 0 0 0 20 35h74a99 99 0 0 0-41-89l12-20c36 24 56 66 53 109h63c3-65-29-128-84-163l24-41a4 4 0 0 1 5-1c3 1 104 178 106 180a4 4 0 0 1-3 6h-24c0 7 0 13 0 20h24A24 24 0 0 0 256 203a23 23 0 0 0-3-12L148 12Z"/></svg>
      ),
    },
    {
      name: "vercel",
      angle: -135,
      svg: (
        <svg viewBox="0 0 76 65" className="h-4 w-4"><path fill="#000" d="M37.5274 0L75.0548 65H0L37.5274 0Z"/></svg>
      ),
    },
  ]

  return (
    <div className="relative h-full w-full overflow-hidden">
      {/* Subtle radial glow */}
      <div
        className="absolute inset-0 opacity-[0.05]"
        style={{ background: `radial-gradient(circle at 50% 50%, ${theme.pillFrom}, transparent 70%)` }}
      />

      {/* Pulse rings — share exact same centered origin as ghost */}
      {[0, 1, 2].map((delay) => (
        <div
          key={delay}
          className="absolute left-1/2 top-1/2 h-20 w-20 -translate-x-1/2 -translate-y-1/2"
        >
          <div
            className="h-full w-full rounded-full border opacity-0"
            style={{
              borderColor: theme.pillFrom,
              animation: `pulse-ring 3s ease-out ${delay}s infinite`,
            }}
          />
        </div>
      ))}

      {/* Ghost in center */}
      <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2">
        <Ghost color={theme.muted} bgColor={theme.secondary} size={40} />
      </div>

      {/* Orbiting logos */}
      {orbit.map((item) => (
        <div
          key={item.name}
          className="absolute left-1/2 top-1/2"
          style={{
            transform: `translate(-50%, -50%) rotate(${item.angle}deg) translateX(120px) rotate(${-item.angle}deg)`,
          }}
        >
          <div className="flex h-8 w-8 items-center justify-center rounded-lg border border-black/[0.04] bg-white/80 shadow-sm backdrop-blur-sm">
            {item.svg}
          </div>
        </div>
      ))}
    </div>
  )
}

function RemembersEverythingVisual({ theme }: { theme: Theme }) {
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
        style={{ background: `radial-gradient(circle at 85% 50%, ${theme.pillFrom}, transparent 70%)` }}
      />
      <svg viewBox="0 0 200 50" className="h-full w-full">
        {/* Curvy connection paths from left nodes to brain on right */}
        {nodes.map((node, i) => (
          <g key={`line-${node.id}`}>
            <path
              d={curvePaths[i]}
              fill="none"
              stroke={theme.secondaryBorder}
              strokeWidth="0.75"
              opacity="0.5"
            />
            {/* Traveling dot along the curve toward brain */}
            <circle r="1.5" fill={theme.pillFrom} opacity="0.8">
              <animateMotion
                dur={`${2 + ((node.id * 0.4) % 1.5)}s`}
                repeatCount="indefinite"
                path={curvePaths[i]}
              />
            </circle>
          </g>
        ))}

        {/* Left-side nodes */}
        {nodes.map((node) => (
          <g key={node.id} className="animate-node-fade-in" style={{ animationDelay: node.delay }}>
            {node.type === "person" && (
              <>
                <circle cx={node.x} cy={node.y} r="5" fill={theme.secondary} stroke={theme.pillFrom} strokeWidth="1" />
                <circle cx={node.x} cy={node.y - 1.2} r="1.5" fill={theme.muted} opacity="0.4" />
                <path
                  d={`M${node.x - 2},${node.y + 2.5} Q${node.x},${node.y + 0.8} ${node.x + 2},${node.y + 2.5}`}
                  stroke={theme.muted}
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
                  fill={theme.secondary}
                  stroke={theme.pillTo}
                  strokeWidth="0.75"
                />
                <line x1={node.x - 2} y1={node.y - 2.5} x2={node.x + 2} y2={node.y - 2.5} stroke={theme.muted} strokeWidth="0.5" opacity="0.3" />
                <line x1={node.x - 2} y1={node.y} x2={node.x + 0.5} y2={node.y} stroke={theme.muted} strokeWidth="0.5" opacity="0.3" />
              </>
            )}
            {node.type === "generic" && (
              <>
                <circle cx={node.x} cy={node.y} r="3" fill={theme.secondary} stroke={theme.pillVia} strokeWidth="0.75" />
                <circle cx={node.x} cy={node.y} r="1.2" fill={theme.pillFrom} opacity="0.5" />
              </>
            )}
          </g>
        ))}

        {/* Brain on the right */}
        <g className="animate-node-fade-in">
          <circle cx={brainX} cy={brainY} r="9" fill={theme.secondary} stroke={theme.pillFrom} strokeWidth="1" opacity="0.9" />
          <g transform={`translate(${brainX}, ${brainY}) scale(0.32) translate(-12, -12)`} style={{ color: theme.pillFrom }}>
            <path
              d="M12 19V5C12 3.34315 10.6569 2 9 2C7.34315 2 6 3.34315 6 5C6 5.55228 5.55228 6 5 6C3.34315 6 2 7.34315 2 9C2 10.6569 3.34315 12 5 12C3.34315 12 2 13.3431 2 15C2 16.6569 3.34315 18 5 18C5.55228 18 6 18.4477 6 19C6 20.6569 7.34315 22 9 22C10.6569 22 12 20.6569 12 19Z"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinejoin="round"
            />
            <path
              d="M12 19V5C12 3.34315 13.3431 2 15 2C16.6569 2 18 3.34315 18 5C18 5.55228 18.4477 6 19 6C20.6569 6 22 7.34315 22 9C22 10.6569 20.6569 12 19 12C20.6569 12 22 13.3431 22 15C22 16.6569 20.6569 18 19 18C18.4477 18 18 18.4477 18 19C18 20.6569 16.6569 22 15 22C13.3431 22 12 20.6569 12 19Z"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinejoin="round"
            />
          </g>
        </g>
      </svg>
    </div>
  )
}

function SecureByDefaultVisual({ theme }: { theme: Theme }) {
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
      icon: (
        <svg viewBox="0 0 74 100" className="h-5 w-5"><path d="M45 1H8a7 7 0 0 0-7 7v84a7 7 0 0 0 7 7h57a7 7 0 0 0 7-7V28L45 1z" fill="#0F9D58"/><path d="M19 49v32h34V49H19zm15 28H23v-5h11v5zm0-9H23v-5h11v5zm0-9H23v-6h11v6zm15 18H38v-5h11v5zm0-9H38v-5h11v5zm0-9H38v-6h11v6z" fill="white"/></svg>
      ),
    },
    {
      id: "slack",
      name: "Slack",
      icon: (
        <svg viewBox="0 0 2448 2453" className="h-5 w-5"><path fill="#36c5f0" d="m897 0c-135 0-245 110-245 245 0 135 110 245 245 245h245V245C1142 110 1032 0 897 0m0 654H244C109 654-1 764-1 899c-1 135 109 245 245 245h653c135 0 245-110 245-245 0-135-110-245-245-245z"/><path fill="#2eb67d" d="M2448 899c0-135-110-245-245-245s-245 110-245 245v245h245c135 0 245-110 245-245m-653 0V245c1-135-109-245-245-245S1255 110 1255 245v654c-1 135 109 245 245 245 135 0 245-110 245-245z"/><path fill="#ecb22e" d="M1550 2453c135 0 245-110 245-245 0-135-110-245-245-245h-245v245c0 135 110 245 245 245m0-654h653c135 0 245-110 245-245 0-135-110-245-245-245h-653c-135 0-245 110-245 245 0 135 110 245 245 245z"/><path fill="#e01e5a" d="M0 1553c0 135 110 245 245 245s245-110 245-245v-245H245C110 1308 0 1418 0 1553m654 0v654c-1 135 109 245 245 245s245-110 245-245v-654c1-135-109-245-245-245-135 0-245 110-245 245z"/></svg>
      ),
    },
    {
      id: "github",
      name: "GitHub",
      icon: (
        <svg viewBox="0 0 1024 1024" className="h-5 w-5"><path fill="currentColor" fillRule="evenodd" d="M512 0C229.12 0 0 229.12 0 512c0 226.56 146.56 417.92 350.08 485.76 25.6 4.48 35.2-10.88 35.2-24.32 0-12.16-.64-52.48-.64-95.36-128.64 23.68-161.92-31.36-172.16-60.16-5.76-14.72-30.72-60.16-52.48-72.32-17.92-9.6-43.52-33.28-.64-33.92 40.32-.64 69.12 37.12 78.72 52.48 46.08 77.44 119.68 55.68 149.12 42.24 4.48-33.28 17.92-55.68 32.64-68.48-113.92-12.8-232.96-56.96-232.96-252.8 0-55.68 19.84-101.76 52.48-137.6-5.12-12.8-23.04-65.28 5.12-135.68 0 0 42.88-13.44 140.8 52.48 40.96-11.52 84.48-17.28 128-17.28s87.04 5.76 128 17.28c97.92-66.56 140.8-52.48 140.8-52.48 28.16 70.4 10.24 122.88 5.12 135.68 32.64 35.84 52.48 81.28 52.48 137.6 0 196.48-119.68 240-233.6 252.8 18.56 16 34.56 46.72 34.56 94.72 0 68.48-.64 123.52-.64 140.8 0 13.44 9.6 29.44 35.2 24.32C877.44 929.92 1024 737.92 1024 512 1024 229.12 794.88 0 512 0" clipRule="evenodd"/></svg>
      ),
    },
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
        className="flex items-center justify-between rounded-lg border px-3 py-2"
        style={{
          backgroundColor: theme.bg,
          borderColor: on ? theme.secondaryBorder : `${theme.secondaryBorder}99`,
        }}
      >
        <span className="text-sm font-medium" style={{ color: on ? theme.text : theme.muted }}>
          {key}
        </span>
        <Switch
          checked={on}
          onChange={() => toggle(key)}
          theme={theme}
        />
      </div>
    )
  }

  return (
    <div className="flex h-full w-full flex-col px-6 pb-5 pt-4">
      {/* Connection selector row */}
      <div className="mb-5 flex gap-3">
        {connections.map((c) => (
          <button
            key={c.id}
            type="button"
            onClick={() => setSelected(c.id)}
            className="flex items-center gap-2.5 rounded-xl px-3.5 py-2.5 text-sm font-medium transition-all"
            style={{
              backgroundColor: selected === c.id ? theme.pillFrom + "12" : theme.secondary,
              border: `1px solid ${selected === c.id ? theme.pillFrom + "40" : theme.secondaryBorder}`,
              color: selected === c.id ? theme.text : theme.muted,
              boxShadow: selected === c.id ? `0 0 0 3px ${theme.pillFrom}15` : "none",
            }}
          >
            {c.icon}
            <span>{c.name}</span>
          </button>
        ))}
      </div>

      {/* Permissions grid */}
      <AnimatePresence mode="wait">
        <motion.div
          key={selected}
          className="grid flex-1 grid-cols-3 gap-4"
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -8 }}
          transition={{ duration: 0.2, ease: "easeOut" }}
        >
          {/* Read */}
          <div className="flex flex-col gap-2">
            <div className="mb-1 flex items-center gap-2">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: "#22c55e" }} />
              <span className="text-sm font-semibold" style={{ color: theme.text }}>Read</span>
            </div>
            {readKeys.map(renderRow)}
          </div>

          {/* Write */}
          <div className="flex flex-col gap-2">
            <div className="mb-1 flex items-center gap-2">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: theme.pillFrom }} />
              <span className="text-sm font-semibold" style={{ color: theme.text }}>Write</span>
            </div>
            {writeKeys.map(renderRow)}
          </div>

          {/* Admin */}
          <div className="flex flex-col gap-2">
            <div className="mb-1 flex items-center gap-2">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: "#ef4444" }} />
              <span className="text-sm font-semibold" style={{ color: theme.text }}>Admin</span>
            </div>
            {adminKeys.map(renderRow)}
          </div>
        </motion.div>
      </AnimatePresence>
    </div>
  )
}

function FeaturesBento({ theme }: { theme: Theme }) {
  const features = [
    {
      title: "Works where you work",
      description: "Slack, Google Sheets, GitHub, Google Meet — Hivy lives inside the tools your team already uses. No new tabs, no context switching.",
      span: "md:row-span-2",
      height: "min-h-72 md:min-h-0",
      textPosition: "bottom" as const,
    },
    {
      title: "Remembers everything",
      description: "Hivy builds a living memory of your business, your teammates, and your decisions — so nothing important is ever lost.",
      span: "",
      height: "min-h-64",
      textPosition: "top" as const,
    },
    {
      title: "Automates the boring stuff",
      description: "Set unlimited recurring tasks on any schedule. Reports, follow-ups, reminders — Hivy handles it all while you focus on what matters.",
      span: "",
      height: "min-h-64",
      textPosition: "overlay" as const,
    },
    {
      title: "Secure by default",
      description: "Granular permissions let you control exactly what Hivy can and cannot do. Your data stays yours, always.",
      span: "md:col-span-2",
      height: "min-h-56",
      textPosition: "top" as const,
    },
  ]

  return (
    <section className="relative z-10 -mt-24 w-full max-w-5xl px-4 py-8 sm:py-8 md:px-0">
      <div className="mb-12 text-center sm:mb-16">
        <h2
          className="font-recoleta text-3xl md:text-4xl font-normal leading-[1.1] tracking-[-0.02em]"
          style={{ color: theme.text }}
        >
          Built for the way you work
        </h2>
        <p className="mx-auto mt-4 max-w-lg text-base leading-[1.6]" style={{ color: theme.muted }}>
          Four reasons teams choose Hivy as their AI coworker.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 md:gap-5">
        {features.map((f, i) => (
          <Card
            key={i}
            theme={theme}
            padding="none"
            className={`relative flex flex-col overflow-hidden ${f.span} ${f.height}`}
          >
            {f.textPosition === "top" && (
              <div className="px-6 pt-6 pb-2">
                <h3
                  className="font-recoleta text-lg font-medium leading-snug tracking-tight sm:text-xl"
                  style={{ color: theme.text }}
                >
                  {f.title}
                </h3>
                <p className="mt-1 text-sm leading-relaxed" style={{ color: theme.muted }}>
                  {f.description}
                </p>
              </div>
            )}

            {/* Visual area */}
            <div className={`relative overflow-hidden ${i === 0 || i === 1 || f.textPosition === "overlay" ? "flex-1" : i === 3 ? "h-[412px]" : "h-44"}`}>
              {i === 0 ? (
                <WorksWhereYouWorkVisual theme={theme} />
              ) : i === 1 ? (
                <RemembersEverythingVisual theme={theme} />
              ) : i === 3 ? (
                <SecureByDefaultVisual theme={theme} />
              ) : (
                <>
                  <div
                    className="absolute inset-0 opacity-[0.06]"
                    style={{
                      background: `linear-gradient(135deg, ${theme.pillFrom}, ${theme.pillTo})`,
                    }}
                  />
                  <div className="flex h-full w-full items-center justify-center">
                    <span className="text-xs font-medium opacity-40" style={{ color: theme.muted }}>
                      Graphic placeholder
                    </span>
                  </div>
                </>
              )}
            </div>

            {f.textPosition === "bottom" && (
              <div className="px-6 pb-6 pt-4">
                <h3
                  className="font-recoleta text-lg font-medium leading-snug tracking-tight sm:text-xl"
                  style={{ color: theme.text }}
                >
                  {f.title}
                </h3>
                <p className="mt-2 text-sm leading-relaxed" style={{ color: theme.muted }}>
                  {f.description}
                </p>
              </div>
            )}

            {f.textPosition === "overlay" && (
              <div className="absolute bottom-0 left-0 right-0 px-6 pb-5 pt-8 backdrop-blur-sm">
                <h3
                  className="font-recoleta text-lg font-medium leading-snug tracking-tight sm:text-xl"
                  style={{ color: theme.text }}
                >
                  {f.title}
                </h3>
                <p className="mt-1 text-sm leading-relaxed" style={{ color: theme.muted }}>
                  {f.description}
                </p>
              </div>
            )}
          </Card>
        ))}
      </div>
    </section>
  )
}

function SlackChatSection({ theme }: { theme: Theme }) {
  const messages = [
    { sender: "user", name: "Alex", text: "Hey team, can someone pull the Q3 numbers from the Sheets?", time: "9:41 AM" },
    { sender: "hivy", name: "hivy", text: "On it — pulling Q3 revenue, expenses, and growth metrics from the master sheet now.", time: "9:41 AM" },
    { sender: "user", name: "Alex", text: "Also draft a follow-up email to the investors?", time: "9:42 AM" },
    { sender: "hivy", name: "hivy", text: "Draft ready in your Gmail drafts. I used last quarter's template and plugged in the new numbers.", time: "9:42 AM" },
    { sender: "user", name: "Alex", text: "You're a lifesaver 🙏", time: "9:43 AM" },
    { sender: "hivy", name: "hivy", text: "Anytime. I'll also create a GitHub issue to track the follow-up tasks.", time: "9:43 AM" },
  ]

  const channels = ["general", "engineering", "design", "announcements"]
  const dms = ["Sarah", "hivy", "Mike"]

  return (
    <section className="relative z-10 w-full max-w-5xl px-4 py-16 sm:py-24 md:px-0">
      <div className="mb-10 text-center sm:mb-14">
        <h2
          className="font-recoleta text-3xl md:text-4xl font-normal leading-[1.1] tracking-[-0.02em]"
          style={{ color: theme.text }}
        >
          Say hello to hivy, your new coworker
        </h2>
        <p className="mx-auto mt-4 max-w-lg text-base leading-[1.6]" style={{ color: theme.muted }}>
          Hivy lives in your Slack, hears your requests, and gets things done — just like a teammate who never sleeps.
        </p>
      </div>

      {/* Gradient card container */}
      <div
        className="rounded-3xl p-8 sm:p-12"
        style={{
          background: `linear-gradient(135deg, ${theme.pillFrom}30, ${theme.pillVia}20, ${theme.pillTo}30)`,
        }}
      >
        {/* Slack app mockup */}
        <div className="flex overflow-hidden rounded-xl shadow-2xl" style={{ height: 600 }}>
          {/* Slack left sidebar */}
          <div className="flex w-52 shrink-0 flex-col" style={{ backgroundColor: "#3F0E40" }}>
            {/* Workspace header */}
            <div className="flex items-center gap-2 px-4 py-3">
              <div className="flex h-6 w-6 items-center justify-center rounded bg-white/10">
                <svg viewBox="0 0 127 127" className="h-3.5 w-3.5"><path d="M27.2 80.0c0 7.3-5.9 13.2-13.2 13.2S.8 87.3.8 80c0-7.3 5.9-13.2 13.2-13.2h13.2v13.2zm6.6 0c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2v33c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V80z" fill="#E01E5A"/><path d="M47.0 27.0c-7.3 0-13.2-5.9-13.2-13.2S39.7.6 47.0.6s13.2 5.9 13.2 13.2v13.2H47.0zm0 6.7c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H13.5C6.2 60.1.3 54.2.3 46.9s5.9-13.2 13.2-13.2h33.5z" fill="#36C5F0"/><path d="M99.9 46.9c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2-5.9 13.2-13.2 13.2H99.9V46.9zm-6.6 0c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V13.5C66.9 6.2 72.8.3 80.1.3s13.2 5.9 13.2 13.2v33.4z" fill="#2EB67D"/><path d="M80.1 99.8c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2-13.2-5.9-13.2-13.2V99.8h13.2zm0-6.6c-7.3 0-13.2-5.9-13.2-13.2s5.9-13.2 13.2-13.2h33.5c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H80.1z" fill="#ECB22E"/></svg>
              </div>
              <div>
                <div className="text-sm font-bold text-white">Acme Inc</div>
                <div className="text-xs text-white/50">hivy workspace</div>
              </div>
            </div>

            {/* Section: Channels */}
            <div className="px-3 py-2">
              <div className="mb-1 px-2 text-xs font-semibold uppercase tracking-wide text-white/40">Channels</div>
              {channels.map((c) => (
                <div
                  key={c}
                  className="flex items-center gap-2 rounded px-2 py-1 text-sm"
                  style={{ color: c === "general" ? "white" : "rgba(255,255,255,0.6)", backgroundColor: c === "general" ? "#1164A3" : "transparent" }}
                >
                  <span className="text-white/40">#</span>
                  <span>{c}</span>
                </div>
              ))}
            </div>

            {/* Section: Direct messages */}
            <div className="px-3 py-2">
              <div className="mb-1 px-2 text-xs font-semibold uppercase tracking-wide text-white/40">Direct messages</div>
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
                  {dm === "hivy" && <span className="ml-auto rounded bg-white/10 px-1.5 py-0 text-[10px] text-white/60">APP</span>}
                </div>
              ))}
            </div>
          </div>

          {/* Main chat area */}
          <div className="flex flex-1 flex-col bg-white">
            {/* Channel header */}
            <div className="flex items-center border-b border-gray-200 px-5 py-3">
              <div className="flex items-center gap-2">
                <span className="text-lg font-bold text-gray-400">#</span>
                <span className="text-base font-bold text-gray-900">general</span>
              </div>
              <div className="ml-4 flex items-center gap-1 text-sm text-gray-400">
                <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                <span>3</span>
              </div>
            </div>

            {/* Messages */}
            <div className="flex-1 overflow-y-auto px-5 py-4">
              {messages.map((msg, i) => (
                <div key={i} className="group flex items-start gap-3 py-2 hover:bg-gray-50">
                  {/* Avatar */}
                  <div
                    className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-sm font-bold text-white"
                    style={{
                      backgroundColor: msg.sender === "hivy" ? theme.pillFrom : "#E01E5A",
                    }}
                  >
                    {msg.sender === "hivy" ? (
                      <svg viewBox="0 0 640 640" className="h-5 w-5" fill="currentColor"><path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" /><ellipse cx="318.5" cy="282" rx="45.5" ry="101" fill="white" /><ellipse cx="457.5" cy="282" rx="45.5" ry="101" fill="white" /></svg>
                    ) : msg.name[0]}
                  </div>

                  {/* Message content */}
                  <div className="flex flex-col">
                    <div className="flex items-baseline gap-2">
                      <span className="text-sm font-bold" style={{ color: msg.sender === "hivy" ? theme.pillFrom : "#1D1C1D" }}>
                        {msg.name}
                      </span>
                      {msg.sender === "hivy" && (
                        <span className="rounded bg-gray-100 px-1 py-0 text-[10px] font-semibold text-gray-500">APP</span>
                      )}
                      <span className="text-xs text-gray-400">{msg.time}</span>
                    </div>
                    <p className="text-sm leading-relaxed text-gray-700">{msg.text}</p>
                  </div>
                </div>
              ))}
            </div>

            {/* Input bar */}
            <div className="border-t border-gray-200 px-5 py-3">
              <div className="flex items-center gap-2 rounded-lg border border-gray-300 px-4 py-2.5">
                <svg className="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" /></svg>
                <span className="text-sm text-gray-400">Message #general</span>
                <div className="ml-auto flex items-center gap-1 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">
                  <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13" /></svg>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

function Footer({ theme }: { theme: Theme }) {
  const linkGroups = [
    {
      title: "Product",
      links: ["Features", "Pricing", "Security", "Changelog"],
    },
    {
      title: "Company",
      links: ["About", "Blog", "Careers", "Contact"],
    },
    {
      title: "Resources",
      links: ["Documentation", "API Reference", "Community", "Status"],
    },
    {
      title: "Legal",
      links: ["Privacy", "Terms", "Cookies"],
    },
  ]

  return (
    <footer className="relative z-10 w-full">
      {/* Link columns */}
      <div className="mx-auto max-w-5xl px-4 py-16 sm:px-0">
        <div className="grid grid-cols-2 gap-8 sm:grid-cols-4">
          {linkGroups.map((group) => (
            <div key={group.title}>
              <div className="mb-4 text-xs font-semibold uppercase tracking-wide" style={{ color: theme.muted }}>
                {group.title}
              </div>
              <div className="flex flex-col gap-3">
                {group.links.map((link) => (
                  <a
                    key={link}
                    href="#"
                    className="text-sm transition-colors hover:opacity-70"
                    style={{ color: theme.text }}
                  >
                    {link}
                  </a>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Giant logo text */}
      <div className="overflow-hidden">
        <div
          className="flex justify-center items-start pt-8 sm:pt-12"
          style={{ height: "clamp(100px, 15vw, 680px)" }}
        >
          <span
            className="font-recoleta font-bold tracking-tighter"
            style={{ color: theme.text, fontSize: "clamp(120px, 18vw, 800px)", lineHeight: 1 }}
          >
            hire hivy
          </span>
        </div>
      </div>
    </footer>
  )
}

export default function ExplorationTwoPage() {
  const theme = ROSE_THEME

  return (
    <>
      <style>{`
        @font-face {
          font-family: 'Recoleta';
          src: url('/fonts/recoleta/Recoleta-Regular.woff2') format('woff2');
          font-weight: 400;
          font-style: normal;
          font-display: swap;
        }
        @font-face {
          font-family: 'Recoleta';
          src: url('/fonts/recoleta/Recoleta-Medium.woff2') format('woff2');
          font-weight: 500;
          font-style: normal;
          font-display: swap;
        }
        @font-face {
          font-family: 'Recoleta';
          src: url('/fonts/recoleta/Recoleta-SemiBold.woff2') format('woff2');
          font-weight: 600;
          font-style: normal;
          font-display: swap;
        }
        @font-face {
          font-family: 'Recoleta';
          src: url('/fonts/recoleta/Recoleta-Bold.woff2') format('woff2');
          font-weight: 700;
          font-style: normal;
          font-display: swap;
        }
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
        @font-face {
          font-family: 'Sohne';
          src: url('/fonts/sohne/Sohne-Buch.otf') format('opentype');
          font-weight: 400;
          font-style: normal;
          font-display: swap;
        }
        @font-face {
          font-family: 'Sohne';
          src: url('/fonts/sohne/Sohne-Kraftig.otf') format('opentype');
          font-weight: 500;
          font-style: normal;
          font-display: swap;
        }
        @font-face {
          font-family: 'Sohne';
          src: url('/fonts/sohne/Sohne-Halbfett.otf') format('opentype');
          font-weight: 600;
          font-style: normal;
          font-display: swap;
        }
        @font-face {
          font-family: 'Sohne';
          src: url('/fonts/sohne/Sohne-Dreiviertelfett.otf') format('opentype');
          font-weight: 700;
          font-style: normal;
          font-display: swap;
        }
        .font-recoleta {
          font-family: 'Recoleta', Georgia, serif;
        }
        .font-sohne {
          font-family: 'Sohne', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        }
        @keyframes ghost-float {
          0%, 100% { transform: translateY(0px); }
          50% { transform: translateY(-8px); }
        }
        .animate-ghost-float {
          animation: ghost-float 3s ease-in-out infinite;
        }
        @keyframes gentle-float {
          0%, 100% { transform: translateY(0px); }
          50% { transform: translateY(-6px); }
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
      `}</style>
      <main
        className="font-sohne relative flex min-h-screen flex-col items-center"
        style={{ backgroundColor: theme.bg, color: theme.text }}
      >
        {/* Soft pastel glow background */}
        <div className="pointer-events-none absolute inset-0 overflow-hidden">
          <div className="absolute -top-44 -left-20 h-96 w-96 rounded-full opacity-30 blur-[120px]" style={{ backgroundColor: theme.glowLeft }} />
          <div className="absolute -top-36 left-1/2 h-96 w-96 -translate-x-1/2 rounded-full opacity-25 blur-[120px]" style={{ backgroundColor: theme.glowCenter }} />
          <div className="absolute -top-44 -right-20 h-96 w-96 rounded-full opacity-25 blur-[120px]" style={{ backgroundColor: theme.glowRight }} />
        </div>

        {/* Floating header */}
        <div className="fixed top-5 left-0 right-0 z-50 mx-auto flex max-w-5xl items-center justify-between px-4 md:px-0 animate-fade-in-down">
          <a href="#" className="font-recoleta text-xl font-bold tracking-tight" style={{ color: theme.text }}>
            hivy
          </a>
          <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2">
            <Navbar theme={theme} />
          </div>
          <div className="flex items-center gap-2 sm:gap-3">
            <div className="hidden sm:block">
              <Button variant="ghost" size="sm" theme={theme} href="#">
                Talk to Sales
              </Button>
            </div>
            <Button variant="primary" size="sm" theme={theme} href="#">
              Hire hivy
            </Button>
             <Button
               variant="ghost"
               size="sm"
               theme={theme}
               className="md:hidden h-9 w-9 px-0"
               aria-label="Open menu"
             >
               <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                 <path d="M3 5h14M3 10h14M3 15h14" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
               </svg>
             </Button>
          </div>
        </div>

        {/* Hero content */}
        <div className="relative flex min-h-screen w-full flex-col items-center px-4 pt-36 sm:pt-44 lg:pt-52">
          <div className="flex flex-1 flex-col items-center justify-center pb-10 sm:pb-14 animate-fade-in-up">
            <HeroContent theme={theme} />
            <div className="mt-36 sm:mt-44">
              <TrustedLogos theme={theme} />
            </div>
          </div>
        </div>

        {/* Features bento */}
        <FeaturesBento theme={theme} />

        {/* Slack chat section */}
        <SlackChatSection theme={theme} />

        <Footer theme={theme} />
      </main>
    </>
  )
}
