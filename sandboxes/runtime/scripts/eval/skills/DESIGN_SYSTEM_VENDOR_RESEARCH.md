# Design System Vendor Research

Research date: 2026-05-28

Goal: find high-quality public skills or skill-like design-system sources that can improve the Rails no-build builder without introducing Tailwind, React, npm build tools, Figma-only flows, or human-in-the-loop workflows. The target users are non-technical Lovable-style users, so the design agent must infer a usable brand direction from plain-language requests and write Rails-native CSS/design documentation autonomously.

## Recommended to vendor next

### Anthropic `frontend-design`

- Source: `anthropics/claude-plugins-official/plugins/frontend-design/skills/frontend-design`
- Commit inspected: `e9b54375b8e6b83a718cebb2033caca8c01976a8`
- License: Apache-2.0
- Popularity signal: `anthropics/claude-plugins-official` had 28,381 stars and 3,022 forks when inspected.
- Why it is useful: strongest trusted upstream guidance for avoiding generic AI-looking UI. It explicitly pushes distinctive aesthetic direction, stronger typography choices, cohesive color themes, spatial composition, motion, and contextual visual details.
- Required adaptation: remove framework-open wording such as React/Vue/Motion. Keep the aesthetic direction, typography, color, composition, and anti-generic rules, but constrain implementation to Rails ERB, plain CSS, Turbo, Stimulus, and Propshaft.
- Suggested local skill name: `anthropic-frontend-design`

### Naksha Studio `brand-kit`

- Source: `Adityaraj0421/naksha-studio/commands/brand-kit.md`
- Commit inspected: `ca9d793e161dd933977c685b3d7972b6d77bfb58`
- License: MIT
- Popularity signal: 285 stars and 22 forks when inspected.
- Why it is useful: best concrete protocol found for turning minimal user input into a brand kit. It covers primary/secondary palette generation, mood-based secondary color derivation, brand-tinted neutrals, semantic colors, type scale, spacing, and component tokens.
- Required adaptation: remove Tailwind config output, Figma style creation, preview MCP flow, and any assumptions that the user will provide technical inputs. Add Rails write targets: `app/assets/stylesheets/application.css`, optional local font assets under `app/assets/fonts`, and `README.md` brand guide.
- Suggested local skill name: `naksha-brand-kit`

### Naksha Studio `design-system`

- Source: `Adityaraj0421/naksha-studio/commands/design-system.md`
- Commit inspected: `ca9d793e161dd933977c685b3d7972b6d77bfb58`
- License: MIT
- Popularity signal: same as Naksha Studio above.
- Why it is useful: strongest compact token architecture protocol found. It formalizes primitive, semantic, and component tokens, plus generation-from-scratch, extraction-from-code, and component-pattern modes.
- Required adaptation: remove Context7, Tailwind, Style Dictionary, Figma, Stitch, and MCP branches. Make CSS custom properties the only output. Make extraction scan Rails CSS/ERB/Stimulus files and update the Rails brand guide.
- Suggested local skill name: `naksha-design-system`

### Naksha Studio `design-token-extractor`

- Source: `Adityaraj0421/naksha-studio/agents/design-token-extractor.md`
- Commit inspected: `ca9d793e161dd933977c685b3d7972b6d77bfb58`
- License: MIT
- Popularity signal: same as Naksha Studio above.
- Why it is useful: useful companion for "reuse the existing brand if this app already has one." It can scan CSS and hardcoded values, consolidate them into tokens, and produce a migration plan.
- Required adaptation: no Tailwind or Style Dictionary output. It should only read Rails project files and produce CSS custom properties plus a concise README brand guide update.
- Suggested local skill name: `naksha-design-token-extractor`

## Useful as reference material, not primary skills

### VoltAgent `awesome-design-md`

- Source: `VoltAgent/awesome-design-md`
- Commit inspected: `4a8c23122c04929fc6df13545b6d1525a473fdfd`
- License: MIT
- Popularity signal: 85,260 stars and 10,249 forks when inspected.
- Why it is useful: large corpus of DESIGN.md files with concrete color roles, typography rules, component styling, layout principles, and anti-patterns. The Lovable, Linear, Stripe, Notion, Supabase, Vercel, and Apple entries are especially useful as examples of precise design language.
- Why not vendor wholesale: this is a large inspiration corpus, not an autonomous skill. Some entries refer to proprietary fonts or brand-specific patterns that should not be copied into random user apps.
- Recommended use: copy only a very small set of representative DESIGN.md examples into `references/` if we need examples for the design-system agent. Prefer using them as pattern references, not as default themes.

### Anthropic `theme-factory`

- Source: `anthropics/skills/skills/theme-factory`
- Commit inspected: `690f15c` for the skills repo clone already reviewed.
- License: Apache-2.0 in the skill directory.
- Popularity signal: `anthropics/skills` had 142,662 stars and 16,844 forks when inspected, though the repository license metadata was not set at the repo root.
- Why it is useful: contains curated palette/font themes and a theme application workflow.
- Why not vendor now: it is artifact/deck-oriented and asks the user to inspect a showcase and choose a theme. That conflicts with our autonomous builder requirement. The font pairing guidance is not as web-app-specific as Naksha plus Anthropic frontend-design.

### OpenDesign `create-design-system`

- Source: `manalkaff/opendesign/skills/create-design-system/SKILL.md`
- Commit inspected: `71d6207df329197718aadc7b063ee00e250211bc`
- License: MIT
- Popularity signal: 140 stars and 13 forks when inspected.
- Why it is useful: good DESIGN.md-era structure for persistent design systems, including `SKILL.md`, tokens, assets, brand voice, visual foundations, iconography, and UI kits.
- Why not vendor now: it stops to ask when key resources are inaccessible and is organized around `./opendesign/design-systems/<name>/`, JSX UI kits, and existing-brand extraction. That is not the Rails eval's desired output path.

## Not recommended

### `nextlevelbuilder/ui-ux-pro-max-skill`

- License: MIT
- Popularity signal: 84,191 stars and 8,687 forks when inspected.
- Reason to skip: broad and impressive, but heavily tied to its own CLI/data stack, Tailwind/shadcn references, canvas/font assets, and multi-purpose presentation/design workflows. It is too large to vendor cleanly and would require more pruning than direct benefit.

### `natdexterra/work-with-design-systems`

- License: MIT
- Popularity signal: 44 stars and 4 forks when inspected.
- Reason to skip: high-quality Figma design-system skill, but requires Figma MCP and intentionally pauses between inspect/build phases. That is the opposite of the autonomous Rails local-app workflow.

### `zephyrwang6/brand-design-md`

- License: none detected by GitHub.
- Popularity signal: 85 stars and 8 forks when inspected.
- Reason to skip: depends on `npx getdesign@latest` at runtime and fetches brand specs dynamically. This violates the no-build/no-runtime-download direction and license posture is unclear.

### `wweggplant/web-design-system-extract`

- License: none detected by GitHub.
- Popularity signal: 7 stars and 0 forks when inspected.
- Reason to skip: too small/low-signal and license is unclear.

### `rohunvora/my-claude-skills` `build-design-system`

- License: none detected by GitHub.
- Popularity signal: 5 stars and 0 forks when inspected.
- Reason to skip: low-signal and license is unclear.

## Proposed design-system skill set

Replace the current single local `design-system-rails` skill with a small vendor-backed group:

- `anthropic-frontend-design`: aesthetic direction, typography taste, composition, anti-generic UI guidance.
- `naksha-brand-kit`: autonomous brand generation from app concept, mood, audience, and optional colors.
- `naksha-design-system`: primitive/semantic/component token protocol and CSS custom property generation.
- `naksha-design-token-extractor`: reuse existing Rails brand tokens before generating a new one.
- `rails-design-system-adapter`: minimal local adapter only for Rails no-build file paths, README brand guide format, no Tailwind/React/npm constraints, and autonomous behavior.

The design-system subagent should run before frontend implementation. Its output contract should be:

- Reuse existing brand if `README.md`, `DESIGN.md`, `app/assets/stylesheets/application.css`, or other Rails CSS already defines coherent tokens.
- Otherwise generate a brand direction from the user's app request, including audience, mood, font pairing, palette, spacing density, radius/shadow language, and component tokens.
- Write CSS custom properties and foundational classes into Rails CSS.
- Write or update a README Brand Guide with the brand rationale, token map, typography rules, component usage, and anti-patterns.
- Never require user choice during eval runs.
