# Vendored Skill Sources

This eval vendors a small subset of public skills rather than entire repositories.

Design-system vendor research is tracked in `DESIGN_SYSTEM_VENDOR_RESEARCH.md` before additional design skills are copied in.

## Upstream skills

- `rails-ai-models`, `rails-ai-controllers`, `rails-ai-views`, `rails-ai-hotwire`, `rails-ai-testing`, `rails-ai-security`
  - Source: `zerobearing2/rails-ai`
  - Commit inspected: `d6832c1`
  - License: MIT
  - Notes: Rails-AI is the closest Rails 8+ domain skill set found. Skills were copied individually and adapted only for this runtime: names changed from `rails-ai:*` to valid local skill names, Superpowers dependencies removed, and Tailwind/DaisyUI cross-references removed because this eval is strict Rails no-build/plain CSS. `rails-ai-testing` was rewritten toward integration-first Minitest coverage that proves real business behavior through Rails request/response workflows.

- `rails-audit-thoughtbot`
  - Source: `thoughtbot/rails-audit-thoughtbot`
  - Commit inspected: `c50f924`
  - License: MIT
  - Notes: Copied with references. Adapted to run autonomously for non-technical users by removing the ask-before-metrics flow and aligning wording with Rails default Minitest instead of RSpec. This audit bundle is assigned to the `code-review` subagent instead of the main app-builder agent.

- `agent-browser`
  - Source: Vercel Labs agent-browser skill data already vendored in this eval folder.
  - Notes: Used only by QA for browser screenshots, recordings, and interaction evidence.

## Local adapter skills

- `rails-no-build`
  - Local Hivy guardrail skill. Needed because public Rails skills commonly allow Tailwind, RSpec, Sidekiq, or plugin-specific workflows that conflict with the product stack.

- `design-system-rails`
  - Local Hivy design-system adapter. Needed because the Rails UI skills found online assume Tailwind, DaisyUI, or component gems. This eval requires plain CSS, CSS custom properties, and Rails-native assets.

- `rails-qa-evidence`
  - Local Hivy QA adapter. Needed to standardize evidence paths, Rails server startup, and `agent-browser` usage for autonomous eval runs.

## Omitted after review

- `Jeffallan/claude-skills` `rails-expert`: popular, but assumes RSpec and Sidekiq.
- `maquina-app/rails-claude-code` UI standards: Rails-specific, but assumes `maquina_components` and Tailwind CSS 4.
- `addyosmani/agent-skills`: very popular, but selected planning/browser skills include human-review or DevTools-MCP assumptions and many JS-centric examples.
- `Shoebtamboli/rails_claude_skills`: generator for skills/agents, not a mature Rails implementation skill.
