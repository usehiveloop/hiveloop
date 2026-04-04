# ZiraLoop — Brand Guide (New Direction)

> "The secure access layer for your AI agents."

---

## Competitive Brand Landscape

Before defining our brand, here's what the funded competitors look like — and the gaps we can exploit.

### Companies Studied

| Company | Funding | Category | Brand Aesthetic |
|---|---|---|---|
| **Composio** | $29M (Series A, Lightspeed) | AI agent tools | Dark, monochrome, monospace-heavy, ASCII art decorations, utilitarian |
| **StackOne** | $24M (Series A, GV/Google Ventures) | AI agent integrations | Purple + dark, enterprise-polished, clean sans-serif, connector grid |
| **Portkey** | $18M (Series A, Elevation Capital) | LLM gateway/observability | Dark navy, blue accent, data-heavy, SaaS dashboard aesthetic |
| **Nango** | $7.5M (Seed, Gradient + YC) | Integration infrastructure | Light/dark toggle, code-first, developer-docs feel, functional |
| **One (withone.ai)** | Undisclosed | Agent infrastructure | Near-black, cyan/orange accents, CLI-forward, hacker aesthetic |

### Pattern Analysis

**What they all do:**
- Dark mode as default (every single one)
- Monospace typography for "technical credibility"
- Dark cards with subtle borders
- Code snippets as hero visuals
- Social proof via logo grids
- "Get Started" + "Book Demo" dual CTAs

**What nobody does:**
- Warm, confident color palette (everyone is cold/dark/techy)
- Typography with personality (everyone uses safe geometric sans-serifs)
- Illustration or visual storytelling (everyone shows code or dashboards)
- A brand that feels approachable AND enterprise-ready simultaneously
- Light mode as the primary experience

**The opportunity:** Every competitor looks like they were designed by the same person. Dark background, monospace font, code snippet hero, blue/purple accent. They all signal "developer tool" and none signal "the company I trust with my credentials." We can stand out by being the brand that feels **premium, warm, and trustworthy** — like a fintech, not a devtool.

---

## Brand Positioning

**ZiraLoop is not another developer tool. It's the security layer.**

The brand should feel like:
- **Stripe** — premium, clean, trustworthy, confident
- **Linear** — opinionated, refined, fast
- **Vercel** — modern, minimal, authoritative

Not like:
- A hacker tool (dark mode, green text, terminal aesthetics)
- A generic SaaS dashboard (blue gradients, stock illustrations)
- An enterprise product (boring, gray, committee-designed)

---

## Color System

### Primary Palette

The core palette is built around **deep teal** — a color that communicates security, trust, and calm authority. It stands out in a market drowning in purple and electric blue.

| Role | Color | Hex | Usage |
|---|---|---|---|
| **Brand** | Deep Teal | `#0D9488` | Primary buttons, links, active states, brand identity |
| **Brand Dark** | Dark Teal | `#0F766E` | Hover states, pressed states |
| **Brand Light** | Light Teal | `#14B8A6` | Highlights, badges, accents on dark surfaces |
| **Brand Subtle** | Pale Teal | `#CCFBF1` | Light backgrounds, subtle highlights, tags |

### Neutral Palette

| Role | Color | Hex | Usage |
|---|---|---|---|
| **Foreground** | Ink | `#0F172A` | Primary text, headings |
| **Body** | Slate | `#334155` | Body text, descriptions |
| **Muted** | Cool Gray | `#64748B` | Secondary text, labels, placeholders |
| **Subtle** | Light Gray | `#94A3B8` | Disabled text, hints |
| **Border** | Border | `#E2E8F0` | Card borders, dividers |
| **Surface** | Surface | `#F8FAFC` | Card backgrounds, elevated surfaces |
| **Background** | White | `#FFFFFF` | Page background |

### Dark Mode Palette

Dark mode is supported but **not the default**. Light mode is the primary experience — this alone differentiates us from every competitor.

| Role | Light | Dark |
|---|---|---|
| **Background** | `#FFFFFF` | `#0B1120` |
| **Surface** | `#F8FAFC` | `#131B2E` |
| **Border** | `#E2E8F0` | `#1E293B` |
| **Foreground** | `#0F172A` | `#F1F5F9` |
| **Body** | `#334155` | `#CBD5E1` |
| **Muted** | `#64748B` | `#64748B` |
| **Brand** | `#0D9488` | `#2DD4BF` (brighter for dark bg) |

### Semantic Colors

| Role | Color | Hex |
|---|---|---|
| **Success** | Emerald | `#10B981` |
| **Warning** | Amber | `#F59E0B` |
| **Danger** | Rose | `#F43F5E` |
| **Info** | Sky | `#0EA5E9` |

### Why Teal, Not Purple

- **Purple is the default** for AI/dev tools (Composio, StackOne, current ZiraLoop, dozens of others). It's invisible.
- **Teal communicates trust and security** — it's the color of fintech (Stripe's blue-green), healthcare compliance, and banking UIs. It says "your credentials are safe here."
- **Teal + white reads as premium** — clean, confident, not trying too hard.
- **It pairs beautifully with warm neutrals** — slate grays, cream accents — creating a palette that feels human, not robotic.

---

## Typography

### Font Stack

| Role | Font | Weight | Usage |
|---|---|---|---|
| **Display** | **Inter** | 700 (Bold) | Hero headlines, page titles, marketing headers |
| **Heading** | **Inter** | 600 (Semibold) | Section headings, card titles, nav items |
| **Body** | **Inter** | 400 (Regular) | Paragraphs, descriptions, form labels |
| **Code** | **JetBrains Mono** | 400/500 | Code snippets, API endpoints, terminal output, monospace data |

### Why Inter

- The most refined variable sans-serif available. Used by Linear, Vercel, Resend, and every premium dev tool that takes typography seriously.
- Excellent readability at small sizes (critical for data-heavy dashboards).
- Variable font — one file, infinite weights, smaller bundle.
- Has a `font-feature-settings: "cv01", "cv02", "cv03", "cv04"` set that gives it a distinctive, slightly geometric character.

### Why JetBrains Mono (Not IBM Plex Mono)

- Designed specifically for code/data readability in UI contexts.
- Ligatures for common code patterns (`!=`, `=>`, `>=`).
- Taller x-height than IBM Plex Mono — reads better at small sizes in dashboards.
- Used by the JetBrains ecosystem — signals "professional developer tools."

### Type Scale

| Level | Size | Weight | Line Height | Letter Spacing | Usage |
|---|---|---|---|---|---|
| **Display XL** | 56px / 3.5rem | 700 | 1.1 | -0.025em | Landing page hero |
| **Display** | 40px / 2.5rem | 700 | 1.15 | -0.02em | Feature page heroes |
| **H1** | 32px / 2rem | 600 | 1.2 | -0.015em | Page titles |
| **H2** | 24px / 1.5rem | 600 | 1.3 | -0.01em | Section headings |
| **H3** | 20px / 1.25rem | 600 | 1.35 | 0 | Card titles, subsections |
| **H4** | 16px / 1rem | 600 | 1.4 | 0 | Small headings |
| **Body L** | 18px / 1.125rem | 400 | 1.6 | 0 | Hero descriptions, intro text |
| **Body** | 15px / 0.9375rem | 400 | 1.6 | 0 | Standard body text |
| **Body S** | 13px / 0.8125rem | 400 | 1.5 | 0 | Captions, secondary text |
| **Code** | 14px / 0.875rem | 400 | 1.6 | 0 | Code blocks, API refs |
| **Code S** | 12px / 0.75rem | 400 | 1.5 | 0 | Inline code, table data |
| **Label** | 12px / 0.75rem | 500 | 1 | 0.05em | Badges, tags, uppercase labels |

---

## Layout System

### Grid

- **Max content width:** 1200px
- **Columns:** 12-column grid with 24px gutters
- **Page padding:** 24px mobile, 40px tablet, 80px desktop
- **Section spacing:** 80px (mobile: 56px) between major sections

### Spacing Scale

Base unit: 4px. Use Tailwind's default scale:

| Token | Value | Usage |
|---|---|---|
| `space-1` | 4px | Tight padding, icon gaps |
| `space-2` | 8px | Inline spacing, small gaps |
| `space-3` | 12px | Form field gaps, compact padding |
| `space-4` | 16px | Standard card padding, list gaps |
| `space-6` | 24px | Section padding, card body |
| `space-8` | 32px | Section headings to content |
| `space-12` | 48px | Between content blocks |
| `space-16` | 64px | Between major sections |
| `space-20` | 80px | Hero padding, page sections |

### Border Radius

| Token | Value | Usage |
|---|---|---|
| `radius-sm` | 6px | Badges, tags, small elements |
| `radius-md` | 8px | Buttons, inputs, inline cards |
| `radius-lg` | 12px | Cards, containers, modals |
| `radius-xl` | 16px | Hero cards, feature blocks, Connect Widget |
| `radius-full` | 9999px | Avatars, status dots, pills |

**Philosophy:** Slightly rounded, not sharp, not pillowy. The 12px card radius is the signature — it communicates "modern and refined" without being soft.

---

## Component Design

### Buttons

**Primary:**
- Background: `#0D9488` (brand teal)
- Text: `#FFFFFF` white
- Padding: 12px 20px
- Border radius: 8px
- Font: Inter 15px, weight 500
- Hover: `#0F766E` (brand dark)
- Shadow: `0 1px 2px rgba(0, 0, 0, 0.05)` (subtle)

**Secondary:**
- Background: `#FFFFFF`
- Border: 1px solid `#E2E8F0`
- Text: `#334155`
- Hover: background `#F8FAFC`

**Ghost:**
- Background: transparent
- Text: `#64748B`
- Hover: background `#F8FAFC`

**Danger:**
- Background: `#F43F5E`
- Text: `#FFFFFF`

### Cards

- Background: `#FFFFFF` (dark: `#131B2E`)
- Border: 1px solid `#E2E8F0` (dark: `#1E293B`)
- Border radius: 12px
- Padding: 24px
- Shadow: `0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.02)` (very subtle)
- Hover (interactive cards): shadow increases to `0 4px 12px rgba(0, 0, 0, 0.06)`

### Inputs

- Height: 40px
- Background: `#FFFFFF` (dark: `#131B2E`)
- Border: 1px solid `#E2E8F0`
- Border radius: 8px
- Padding: 0 12px
- Focus: border `#0D9488`, ring `0 0 0 3px rgba(13, 148, 136, 0.1)`
- Font: Inter 15px

### Tables

- Header: `#F8FAFC` background, Inter 12px/500, uppercase, `#64748B` text, letter-spacing 0.05em
- Rows: white background, 1px bottom border `#E2E8F0`
- Hover: `#F8FAFC` background
- Monospace for data values (JetBrains Mono 14px)
- Numeric alignment: tabular-nums, right-aligned

### Badges / Tags

- Padding: 2px 8px
- Border radius: 6px
- Font: Inter 12px/500
- Variants:
  - Default: `#F8FAFC` bg, `#334155` text
  - Brand: `#CCFBF1` bg, `#0F766E` text
  - Success: `#D1FAE5` bg, `#065F46` text
  - Warning: `#FEF3C7` bg, `#92400E` text
  - Danger: `#FFE4E6` bg, `#9F1239` text

---

## Iconography

### Icon Library

**Lucide React** — continue using, but with refined sizing:

| Context | Size | Stroke Width |
|---|---|---|
| Navigation | 20px | 1.5px |
| Inline (buttons, badges) | 16px | 1.5px |
| Feature cards | 24px | 1.5px |
| Hero illustrations | 32px | 1.5px |

### Icon Color Rules

- Navigation (inactive): `#64748B` (muted)
- Navigation (active): `#0D9488` (brand)
- In-context: inherit text color
- On brand background: `#FFFFFF`

---

## Logo

### Mark

The lock icon remains the brand mark. It communicates security, trust, and custody. Refinements:

- Slightly increase the stroke weight for better visibility at small sizes
- Use brand teal `#0D9488` as the mark color (not purple)
- The mark should work at 16px (favicon), 24px (nav), 32px (footer), 48px (hero)

### Wordmark

- Font: **Inter**, weight 600 (semibold)
- Casing: "ZiraLoop" (capital Z, capital L) or "ziraloop" (all lowercase for informal contexts)
- The wordmark should stand alone without the lock mark in space-constrained contexts

### Logo Lockup

```
[Lock Mark]  ZiraLoop
```

- 8px gap between mark and wordmark
- Vertically centered
- Minimum clear space: 16px on all sides

### Color Variants

| Variant | Mark Color | Text Color | Background |
|---|---|---|---|
| **Default** | `#0D9488` | `#0F172A` | White/Light |
| **Reversed** | `#2DD4BF` | `#F1F5F9` | Dark |
| **Monochrome** | `#0F172A` | `#0F172A` | White |
| **Mono Reversed** | `#FFFFFF` | `#FFFFFF` | Dark |

---

## Website Layout

### Navigation

- **Style:** Clean horizontal nav, not sticky (scrolls away). Reappears on scroll-up.
- **Background:** White with 1px bottom border (`#E2E8F0`). Blurs to frosted glass on scroll.
- **Logo:** Left-aligned, lock mark + wordmark
- **Links:** Center-aligned. Inter 15px/500. Color `#334155`, hover `#0D9488`.
- **CTA:** Right-aligned. Primary button "Get Started" + ghost button "Docs"
- **Mobile:** Hamburger menu, slide-in from right

### Hero Pattern

Unlike competitors who lead with a code snippet, we lead with a **clear statement + visual**:

```
[Nav]

    The secure access layer
    for your AI agents.

    Connect agents to 250+ apps with enterprise-grade
    credential custody, scoped permissions, and managed auth.

    [Get Started — Free]   [Read the Docs]

    [Hero Visual: Stylized diagram showing the three flows —
     Connect → Vault → Proxy — with app logos flowing through]

    Trusted by teams at [logo] [logo] [logo] [logo]
```

- Hero headline: Display XL (56px/700), color `#0F172A`
- Subheadline: Body L (18px/400), color `#64748B`
- Max width on text: 680px, centered
- Hero visual: custom illustration (not a screenshot, not a code block)

### Section Pattern

Alternating sections with generous whitespace:

```
[White background section — 80px top/bottom padding]
  H2 heading, centered
  Body text, centered, max-width 560px
  Content (cards, features, etc.)

[Surface background section (#F8FAFC) — 80px padding]
  H2 heading, centered
  Body text, centered
  Content
```

### Feature Cards

- 3-column grid on desktop, 1-column mobile
- Card with 12px radius, subtle border, 24px padding
- Top: 24px icon in brand teal
- H3 title: 20px/600
- Body: 15px/400, `#64748B`
- Bottom: text link with arrow → in brand teal

### Code Blocks

- Background: `#0B1120` (dark regardless of page theme)
- Border: 1px solid `#1E293B`
- Border radius: 12px
- Font: JetBrains Mono 14px
- Header bar: `#131B2E` with filename/language label
- Syntax highlighting: standard dark theme with teal accents for strings
- Copy button: top-right, ghost style

---

## Photography & Illustration

### No Stock Photos

Zero stock photography. Zero human faces. The brand is abstract, technical, and confident.

### Illustration Style

- **Flat, geometric, diagrammatic** — not 3D, not isometric, not cartoon
- **Colors:** Brand teal + slate neutrals. Occasional emerald/amber for semantic meaning
- **Lines:** 1.5-2px stroke, consistent with icon stroke weight
- **App logos:** Used in their real brand colors within diagrams (Slack purple, GitHub black, etc.)
- **Flow diagrams:** Show data/credential flows as clean horizontal or vertical paths
- **Background patterns:** Subtle dot grid in `#E2E8F0` at 30% opacity on white sections; `#1E293B` at 15% on dark sections

---

## Voice & Language

### Tone

- **Direct, not clever.** Say what things do. No "supercharge your workflow" or "unleash the power of."
- **Confident, not aggressive.** State facts, show architecture, let the product speak.
- **Technical, not jargon-heavy.** Use precise terms (envelope encryption, SSE streaming, OAuth 2.0) but explain them in context.
- **Concise.** Every word earns its place.

### Examples

| Instead of | Write |
|---|---|
| "Supercharge your AI agents with enterprise-grade security" | "Your agents' credentials deserve a vault, not an environment variable." |
| "Seamlessly connect to hundreds of apps" | "Connect to 250+ apps. OAuth, API keys, webhooks — handled." |
| "Lightning-fast performance" | "Sub-5ms proxy overhead. Your users won't notice it's there." |
| "Robust permission management system" | "Agents get exactly the permissions they need. Nothing more." |
| "Cutting-edge encryption technology" | "AES-256-GCM envelope encryption. KMS-wrapped keys. Sealed memory." |

### Headlines Pattern

Lead with the outcome, not the feature. Use periods, not exclamation marks.

```
Good:  "See everything your agents do."
Bad:   "Powerful observability for AI agents!"

Good:  "Ship BYOK in days, not months."
Bad:   "The ultimate Bring Your Own Key solution"

Good:  "One connect flow. Every credential."
Bad:   "Seamless multi-provider authentication widget"
```

---

## What Makes This Brand Different

| Competitor Pattern | ZiraLoop Choice | Why |
|---|---|---|
| Dark mode default | **Light mode default** | Every competitor is dark. Light reads as confident, premium, and trustworthy. |
| Purple/blue accent | **Deep teal accent** | Teal communicates trust and security (fintech association). Purple is invisible in this market. |
| Monospace display fonts | **Inter display** | Clean, refined, professional. Monospace reserved for actual code. |
| Code snippet hero | **Statement + visual** | The hero says what we do, not how to install us. Code lives in docs. |
| Sharp/no border radius | **12px signature radius** | Modern and refined without being soft. |
| Dense, data-heavy layouts | **Generous whitespace** | Breathing room signals confidence. We don't need to cram everything above the fold. |
| Terminal/hacker aesthetic | **Premium fintech aesthetic** | We handle credentials. We should look like a company you trust with credentials. |

---

## Sources — Competitor Reference

- [Composio](https://composio.dev) — $29M raised, Lightspeed. Dark monochrome, ASCII art, utilitarian.
- [StackOne](https://www.stackone.com) — $24M raised, GV. Purple + dark, enterprise connector grid.
- [Portkey](https://portkey.ai) — $18M raised, Elevation Capital. Dark navy, blue accent, data-heavy.
- [Nango](https://nango.dev) — $7.5M raised, Gradient + YC. Light/dark toggle, code-first, developer-docs feel.
- [One (withone.ai)](https://www.withone.ai) — Undisclosed. Near-black, cyan/orange accents, CLI-forward.
